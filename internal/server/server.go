package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/gen/athena/v1/athenav1connect"
	"athena-erp/internal/audit"
	"athena-erp/internal/auth"
	"athena-erp/internal/lifecycle"
	"athena-erp/internal/store"
	"athena-erp/internal/telemetry"
	"athena-erp/internal/webui"
	"athena-erp/internal/workspace"
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
	// Database is the driver endpoint for the Workspace database. Nil serves
	// the desk without a database.
	Database *workspace.DBEndpoint
	// Workspace, when set, must match the athena_workspace row in Database.
	Workspace *Workspace
	// FilesRoot is the directory FileService manages when no Workspace is
	// set, as with athena serve. Ignored when Workspace has a Root.
	FilesRoot string
	Version   string
}

// Workspace is the identity a server process is bound to for its lifetime.
type Workspace struct {
	ID   string
	Name string
	Slug string
	// Root is the Workspace directory FileService manages. Empty when the
	// caller has no directory, which leaves FileService unavailable.
	Root string
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
		endpoint := config.Database
		dialect = dialectFor(endpoint.Driver)
		driverName := "MariaDB"
		dbAttrs := otelsql.WithAttributes(semconv.DBSystemNameMariaDB)
		if endpoint.Driver == workspace.DriverPostgres {
			driverName = "PostgreSQL"
			dbAttrs = otelsql.WithAttributes(semconv.DBSystemNamePostgreSQL)
		}
		connector, err := endpointConnector(endpoint)
		if err != nil {
			return err
		}
		// otelsql records statement text but never parameter values, so Record
		// data stays out of traces.
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
			return fmt.Errorf("connect to %s: %w", driverName, err)
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
		users := &auth.Service{DB: db}
		if err := users.EnsureSystemRoles(ctx); err != nil {
			return fmt.Errorf("seed system roles: %w", err)
		}
		if generated, err := users.EnsureRoot(ctx, endpoint.Password()); err != nil {
			return fmt.Errorf("seed root administrator: %w", err)
		} else if generated != "" {
			log.Printf("Administrator account created with a generated password, shown once: %s", generated)
		}
		databaseStatus = "Connected"
		log.Printf("%s connected; metadata tables are ready", driverName)
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

// filesWorkspace is the Workspace whose directory FileService manages: the
// bound Workspace, or for athena serve a Workspace that only carries
// FilesRoot.
func filesWorkspace(config Config) *Workspace {
	if config.Workspace != nil && config.Workspace.Root != "" {
		return config.Workspace
	}
	if config.FilesRoot != "" {
		return &Workspace{Root: config.FilesRoot}
	}
	return config.Workspace
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
	hooks := lifecycle.NewRegistry()
	registerRules(hooks, db)
	athenav1connect.RegisterRecordServiceHandler(rpcServer, recordService{db: db, hooks: hooks})
	athenav1connect.RegisterRuleServiceHandler(rpcServer, newRuleService(db))
	athenav1connect.RegisterFileServiceHandler(rpcServer, newFileService(filesWorkspace(config), db))
	athenav1connect.RegisterWorkspaceServiceHandler(rpcServer, workspaceService{db})
	athenav1connect.RegisterNotificationServiceHandler(rpcServer, notificationService{db})
	athenav1connect.RegisterLedgerServiceHandler(rpcServer, newLedgerService(db))
	connecthttp.Mount(mux, rpcServer)
}

// dialectFor returns the SQL dialect for an endpoint driver. Unknown drivers
// fall back to MariaDB; callers validate the driver before opening.
func dialectFor(driver string) store.Dialect {
	if driver == workspace.DriverPostgres {
		return store.PostgreSQL{}
	}
	return store.MariaDB{}
}

// endpointConnector returns a database/sql connector for the endpoint,
// pinning the MariaDB session to UTC. The PostgreSQL branch pins its session
// at parse time (timezone=UTC in the connection config).
func endpointConnector(endpoint *workspace.DBEndpoint) (driver.Connector, error) {
	if endpoint.Driver == workspace.DriverPostgres {
		connector, err := endpoint.Connector()
		if err != nil {
			return nil, fmt.Errorf("open PostgreSQL connection: %w", err)
		}
		return connector, nil
	}
	connector, err := mysql.NewConnector(utcConfig(endpoint.MySQL))
	if err != nil {
		return nil, fmt.Errorf("open MariaDB connection: %w", err)
	}
	return connector, nil
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
