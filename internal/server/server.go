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
	// Database is the driver config for the project database. Nil serves the
	// desk without a database.
	Database *mysql.Config
	// Project, when set, must match the athena_project row in Database.
	Project *Project
	Version string
}

// Project is the identity a server process is bound to for its lifetime.
type Project struct {
	ID   string
	Name string
	Slug string
}

// Serve starts the single HTTP server for the embedded UI and ConnectRPC API.
func Serve(ctx context.Context, config Config) error {
	addr := config.Addr
	var attrs []attribute.KeyValue
	if config.Project != nil {
		attrs = append(attrs, attribute.String("athena.project.slug", config.Project.Slug), attribute.String("athena.project.id", config.Project.ID))
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
		// otelsql records statement text but never parameter values, so entry
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
		if config.Project != nil {
			if err := checkProject(ctx, db, *config.Project); err != nil {
				return err
			}
		}
		if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
			return fmt.Errorf("apply system migrations: %w", err)
		}
		if err := resumeOperations(ctx, db); err != nil {
			return fmt.Errorf("resume interrupted Record saves: %w", err)
		}
		databaseStatus = "Connected"
		log.Printf("MariaDB connected; metadata tables are ready")
	}

	mux := http.NewServeMux()

	var interceptors []connect.ServerInterceptor
	if tel.Enabled() {
		// Trust the browser's traceparent so a desk click and its server work
		// share one trace. Safe while the server is loopback-only and
		// unauthenticated; revisit when remote access is enabled.
		interceptor, err := otelconnect.NewServerInterceptor(otelconnect.WithTrustRemote())
		if err != nil {
			return fmt.Errorf("tracing interceptor: %w", err)
		}
		interceptors = append(interceptors, interceptor)
	}
	rpcServer := connect.NewServer(interceptors...)
	athenav1connect.RegisterSystemServiceHandler(rpcServer, systemService{config: config, telemetry: tel.Enabled()})
	athenav1connect.RegisterRecordServiceHandler(rpcServer, recordService{db: db})
	athenav1connect.RegisterEntryServiceHandler(rpcServer, entryService{db: db})
	connecthttp.Mount(mux, rpcServer)

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"ok","database":%q}`, databaseStatus)
	})
	mux.Handle("/otlp/v1/traces", tel.BrowserTracesHandler())
	mux.Handle("/", webui.Handler())

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
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

// utcConfig returns a copy of cfg with the session and driver pinned to UTC.
// TIMESTAMP columns convert through the session time zone, so this keeps entry
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
}

func (s systemService) GetSystemInfo(context.Context, *athenav1.GetSystemInfoRequest) (*athenav1.GetSystemInfoResponse, error) {
	res := &athenav1.GetSystemInfoResponse{ProductName: "Athena ERP", Version: s.config.Version, TelemetryEnabled: s.telemetry}
	if s.config.Project != nil {
		res.ProjectName = s.config.Project.Name
		res.ProjectSlug = s.config.Project.Slug
	}
	return res, nil
}

// checkProject refuses to serve a database that belongs to another project,
// which guards against a wrong database name or a restored backup mix-up.
func checkProject(ctx context.Context, db *sql.DB, project Project) error {
	var id string
	err := db.QueryRowContext(ctx, `SELECT id FROM athena_project LIMIT 1`).Scan(&id)
	if err != nil {
		return fmt.Errorf("database for project %q has no project identity (athena_project): %w", project.Slug, err)
	}
	if id != project.ID {
		return fmt.Errorf("database belongs to project %s, but athena.yaml for %q has id %s; check the database name and backups", id, project.Slug, project.ID)
	}
	return nil
}
