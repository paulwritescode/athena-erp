package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/gen/athena/v1/athenav1connect"
	"athena-erp/internal/webui"
	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	_ "github.com/go-sql-driver/mysql"
)

const (
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 10 * time.Second
)

// Serve starts the single HTTP server for the embedded UI and ConnectRPC API.
func Serve(ctx context.Context, addr string) error {
	databaseStatus := "Not configured"
	var db *sql.DB
	if dsn := os.Getenv("ATHENA_DATABASE_URL"); dsn != "" {
		var err error
		db, err = sql.Open("mysql", dsn)
		if err != nil {
			return fmt.Errorf("open MariaDB connection: %w", err)
		}
		defer db.Close()
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = db.PingContext(pingCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("connect to MariaDB: %w", err)
		}
		if err := migrate(db); err != nil {
			return fmt.Errorf("initialize MariaDB metadata tables: %w", err)
		}
		databaseStatus = "Connected"
		log.Printf("MariaDB connected; metadata tables are ready")
	}

	mux := http.NewServeMux()

	rpcServer := connect.NewServer()
	athenav1connect.RegisterSystemServiceHandler(rpcServer, systemService{})
	connecthttp.Mount(mux, rpcServer)
	mux.Handle("/api/records", recordHandler(db))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"ok","database":%q}`, databaseStatus)
	})
	mux.Handle("/", webui.Handler())

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("Athena ERP listening at http://localhost%s", addr)
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
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

type systemService struct{}

func (systemService) GetSystemInfo(context.Context, *athenav1.GetSystemInfoRequest) (*athenav1.GetSystemInfoResponse, error) {
	return &athenav1.GetSystemInfoResponse{
		ProductName: "Athena ERP",
		Version:     "0.1.0-dev",
	}, nil
}
