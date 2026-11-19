package server

import (
	"context"
	"database/sql"
	"fmt"

	"athena-erp/gen/athena/v1/athenav1connect"
	"athena-erp/internal/lifecycle"
	"athena-erp/internal/store"

	"github.com/go-sql-driver/mysql"
)

// Kernel gives the CLI the same Record Type and Record services the
// ConnectRPC handlers use, over its own database connection. It adds no
// endpoint.
type Kernel struct {
	RecordTypes athenav1connect.RecordTypeServiceHandler
	Records     athenav1connect.RecordServiceHandler
	System      athenav1connect.SystemServiceHandler
	// Hooks takes Go hooks for the lifecycle events (spec 5.2).
	Hooks *lifecycle.Registry
	db    *sql.DB
}

// OpenKernel connects to the Workspace database and applies pending system
// migrations, as Serve does at start-up. When workspace is set, the database
// must belong to it. Close the Kernel when done.
func OpenKernel(ctx context.Context, cfg *mysql.Config, workspace *Workspace) (*Kernel, error) {
	db, err := openMigrationDB(ctx, cfg, workspace)
	if err != nil {
		return nil, err
	}
	if _, err := store.Up(ctx, db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply system migrations: %w", err)
	}
	if err := resumeOperations(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("resume interrupted Record Type saves: %w", err)
	}
	hooks := lifecycle.NewRegistry()
	return &Kernel{RecordTypes: recordTypeService{db: db}, Records: recordService{db: db, hooks: hooks}, System: systemService{db: db}, Hooks: hooks, db: db}, nil
}

// Close releases the database connection.
func (k *Kernel) Close() error { return k.db.Close() }
