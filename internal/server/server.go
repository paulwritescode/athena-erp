package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/gen/athena/v1/athenav1connect"
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
		cfg := config.Database.Clone()
		// TIMESTAMP columns convert through the session time zone. Pinning the
		// session and the driver to UTC keeps entry timestamps correct whatever
		// the server's system zone is.
		cfg.ParseTime = true
		cfg.Loc = time.UTC
		if cfg.Params == nil {
			cfg.Params = map[string]string{}
		}
		cfg.Params["time_zone"] = "'+00:00'"
		connector, err := mysql.NewConnector(cfg)
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
		if err := migrate(db); err != nil {
			return fmt.Errorf("initialize MariaDB metadata tables: %w", err)
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

func migrate(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS athena_records (
			name VARCHAR(140) NOT NULL PRIMARY KEY,
			label VARCHAR(255) NOT NULL,
			definition JSON NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS athena_modules (
			name VARCHAR(140) NOT NULL PRIMARY KEY,
			label VARCHAR(255) NOT NULL,
			definition JSON NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS athena_rules (
			name VARCHAR(140) NOT NULL PRIMARY KEY,
			label VARCHAR(255) NOT NULL,
			module_name VARCHAR(140) NOT NULL DEFAULT 'Core',
			definition JSON NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	}
	statements = append(statements,
		`ALTER TABLE athena_records ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1 AFTER label`,
		// Journal of Record saves. A pending row is resumed at startup.
		`CREATE TABLE IF NOT EXISTS athena_record_operations (
			id CHAR(36) NOT NULL PRIMARY KEY,
			record_name VARCHAR(140) NOT NULL,
			expected_revision BIGINT NOT NULL,
			target_revision BIGINT NULL,
			definition JSON NOT NULL,
			status VARCHAR(20) NOT NULL,
			last_step VARCHAR(40) NOT NULL,
			error TEXT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			KEY idx_record_operations_status (status)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`ALTER TABLE athena_record_operations ADD COLUMN IF NOT EXISTS action VARCHAR(20) NOT NULL DEFAULT 'save' AFTER record_name`,
		`ALTER TABLE athena_record_operations ADD COLUMN IF NOT EXISTS options JSON NULL AFTER definition`,
		// Audit history of entry changes, written in the same transaction as the change.
		`CREATE TABLE IF NOT EXISTS athena_entry_history (
			id CHAR(36) NOT NULL PRIMARY KEY,
			record_name VARCHAR(140) NOT NULL,
			entry_id CHAR(36) NOT NULL,
			revision BIGINT NOT NULL,
			action VARCHAR(20) NOT NULL,
			changed_fields JSON NOT NULL,
			actor VARCHAR(255) NOT NULL,
			created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
			KEY idx_entry_history_entry (record_name, entry_id, created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	)
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	return addEntryRevisions(db)
}

// legacyRecordKeyPattern accepts keys saved before keys were lowercase-only.
var legacyRecordKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)

// addEntryRevisions adds the revision column to Record tables saved before
// optimistic concurrency existed. Names come from saved definitions only.
func addEntryRevisions(db *sql.DB) error {
	rows, err := db.Query(`SELECT name FROM athena_records`)
	if err != nil {
		return err
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		names = append(names, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, name := range names {
		if !legacyRecordKeyPattern.MatchString(name) {
			return fmt.Errorf("saved Record name %q is not a valid identifier", name)
		}
		table := recordTableName(name)
		exists, err := dialect.TableExists(context.Background(), db, table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if _, err := db.Exec(dialect.AddRevisionColumn(table)); err != nil {
			return err
		}
	}
	return nil
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
