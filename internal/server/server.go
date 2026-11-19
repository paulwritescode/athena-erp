package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/gen/athena/v1/athenav1connect"
	"athena-erp/internal/audit"
	"athena-erp/internal/auth"
	"athena-erp/internal/store"
	"athena-erp/internal/telemetry"
	"athena-erp/internal/webui"
	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"connectrpc.com/otelconnect"
	"github.com/XSAM/otelsql"
	"github.com/go-sql-driver/mysql"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

const (
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 10 * time.Second
)

// Config selects what one server process serves.
type Config struct {
	Addr string
	// Database is the driver config for the Workspace database. Nil serves the
	// desk without a database.
	Database *mysql.Config
	// Workspace, when set, must match the athena_workspace row in Database.
	Workspace *Workspace
	Version   string
}

// Workspace is the identity a server process is bound to for its lifetime.
type Workspace struct {
	ID   string
	Name string
	Slug string
}

// Serve starts the single HTTP server for the embedded UI and ConnectRPC API.
func Serve(ctx context.Context, config Config) error {
	addr := config.Addr
	var attrs []attribute.KeyValue
	if config.Workspace != nil {
		attrs = append(attrs, attribute.String("athena.workspace.slug", config.Workspace.Slug), attribute.String("athena.workspace.id", config.Workspace.ID))
	}
	tel, err := telemetry.Setup(ctx, config.Version, attrs...)
	if err != nil {
		return err
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tel.Shutdown(flushCtx)
	}()
	databaseStatus := "Not configured"
	var db *sql.DB
	if config.Database != nil {
		connector, err := mysql.NewConnector(utcConfig(config.Database))
		if err != nil {
			return fmt.Errorf("open MariaDB connection: %w", err)
		}
		// otelsql records statement text but never parameter values, so Record
		// data stays out of traces.
		dbAttrs := otelsql.WithAttributes(semconv.DBSystemNameMariaDB)
		db = otelsql.OpenDB(connector, dbAttrs, otelsql.WithSpanOptions(otelsql.SpanOptions{OmitConnResetSession: true, OmitRows: true}))
		if tel.Enabled() {
			if _, err := otelsql.RegisterDBStatsMetrics(db, dbAttrs); err != nil {
				return fmt.Errorf("register database metrics: %w", err)
			}
		}
		defer db.Close()
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = db.PingContext(pingCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("connect to MariaDB: %w", err)
		}
		if config.Workspace != nil {
			if err := checkWorkspace(ctx, db, *config.Workspace); err != nil {
				return err
			}
		}
		if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
			return fmt.Errorf("apply system migrations: %w", err)
		}
		if err := resumeOperations(ctx, db); err != nil {
			return fmt.Errorf("resume interrupted Record Type saves: %w", err)
		}
		databaseStatus = "Connected"
		log.Printf("MariaDB connected; metadata tables are ready")
	}

	mux := http.NewServeMux()

	interceptors := []connect.ServerInterceptor{requestInterceptor()}
	if tel.Enabled() {
		// Trust the browser's traceparent so a desk click and its server work
		// share one trace. Safe while the server is loopback-only; revisit
		// when remote access is enabled.
		interceptor, err := otelconnect.NewServerInterceptor(otelconnect.WithTrustRemote())
		if err != nil {
			return fmt.Errorf("tracing interceptor: %w", err)
		}
		interceptors = append(interceptors, interceptor)
	}
	mountRPC(mux, config, db, tel.Enabled(), interceptors)

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"ok","database":%q}`, databaseStatus)
	})
	mux.Handle("/otlp/v1/traces", tel.BrowserTracesHandler())
	mux.Handle("/", webui.Handler())

	srv := &http.Server{
		Addr:              addr,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("Athena ERP listening at http://%s", addr)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down HTTP server: %w", err)
		}
		return nil
	}
}

// mountRPC registers every ConnectRPC service on mux behind interceptors and
// the permission gate. The gate runs last, so denials are traced and carry the
// request ID. Without a database it refuses every call.
func mountRPC(mux *http.ServeMux, config Config, db *sql.DB, telemetry bool, interceptors []connect.ServerInterceptor) {
	var authSvc *auth.Service
	if db != nil {
		authSvc = &auth.Service{DB: db}
	}
	interceptors = append(interceptors, gateInterceptor(authSvc))
	rpcServer := connect.NewServer(interceptors...)
	athenav1connect.RegisterSystemServiceHandler(rpcServer, systemService{config: config, telemetry: telemetry, db: db})
	athenav1connect.RegisterAuthServiceHandler(rpcServer, authService{auth: authSvc})
	athenav1connect.RegisterRecordTypeServiceHandler(rpcServer, recordTypeService{db: db})
	athenav1connect.RegisterRecordServiceHandler(rpcServer, recordService{db: db})
	connecthttp.Mount(mux, rpcServer)
}

// utcConfig returns a copy of cfg with the session and driver pinned to UTC.
// TIMESTAMP columns convert through the session time zone, so this keeps Record
// timestamps correct whatever the server's system zone is.
func utcConfig(base *mysql.Config) *mysql.Config {
	cfg := base.Clone()
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	cfg.Params["time_zone"] = "'+00:00'"
	return cfg
}

type systemService struct {
	config    Config
	telemetry bool
	db        *sql.DB
}

func (s systemService) VerifyAuditLog(ctx context.Context, req *athenav1.VerifyAuditLogRequest) (*athenav1.VerifyAuditLogResponse, error) {
	if s.db == nil {
		return nil, connect.NewError(connect.CodeUnavailable, "the database is not configured")
	}
	if _, err := requireSystem(ctx, auth.PermissionAudit); err != nil {
		return nil, err
	}
	var anchor *audit.Anchor
	if req.GetAnchor() != "" {
		var err error
		if anchor, err = audit.ParseAnchor(req.GetAnchor()); err != nil {
			return nil, invalid(err.Error())
		}
	}
	ctx, _, end := startKernelSpan(ctx, "verify_audit_log", "")
	result, err := audit.Verify(ctx, s.db, anchor)
	if err != nil {
		err = internalError("verify audit log", err)
	}
	end(err)
	if err != nil {
		return nil, err
	}
	return &athenav1.VerifyAuditLogResponse{Ok: result.OK, RowsChecked: result.Rows, HeadSeq: result.HeadSeq, HeadHash: result.HeadHash, BrokenSeq: result.BrokenSeq, Reason: result.Reason}, nil
}

func (s systemService) GetSystemInfo(context.Context, *athenav1.GetSystemInfoRequest) (*athenav1.GetSystemInfoResponse, error) {
	res := &athenav1.GetSystemInfoResponse{ProductName: "Athena ERP", Version: s.config.Version, TelemetryEnabled: s.telemetry}
	if s.config.Workspace != nil {
		res.WorkspaceName = s.config.Workspace.Name
		res.WorkspaceSlug = s.config.Workspace.Slug
	}
	return res, nil
}

// checkWorkspace refuses to serve a database that belongs to another
// Workspace, which guards against a wrong database name or a restored backup
// mix-up. A database from before the vocabulary rename has no athena_workspace
// table until migration 5 runs, so the identity is read from the legacy table.
func checkWorkspace(ctx context.Context, db *sql.DB, workspace Workspace) error {
	id, err := workspaceIdentity(ctx, db)
	if err != nil {
		return fmt.Errorf("database for workspace %q has no workspace identity (athena_workspace): %w", workspace.Slug, err)
	}
	if id != workspace.ID {
		return fmt.Errorf("database belongs to workspace %s, but athena.yaml for %q has id %s; check the database name and backups", id, workspace.Slug, workspace.ID)
	}
	return nil
}
