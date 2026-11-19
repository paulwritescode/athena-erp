package server

import (
	"context"
	"database/sql"
	"fmt"

	"athena-erp/gen/athena/v1/athenav1connect"
	"athena-erp/internal/auth"
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
	// Auth is the AuthService handler. The CLI acts as the operator: it has
	// no session, so it uses the user, Role and Access Log methods only.
	Auth athenav1connect.AuthServiceHandler
	// Users is the auth store, for what only the operator may do, such as
	// creating an instance owner.
	Users *auth.Service
	// Files is the FileService handler over the Workspace directory. It
	// refuses every call when the Kernel has no Workspace.
	Files athenav1connect.FileServiceHandler
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
	files := newFileService(workspace, db)
	hooks := lifecycle.NewRegistry()
	users := &auth.Service{DB: db}
	return &Kernel{RecordTypes: recordTypeService{db: db}, Records: recordService{db: db, hooks: hooks}, System: systemService{db: db}, Auth: authService{auth: users}, Users: users, Files: files, Hooks: hooks, db: db}, nil
}

// Close releases the database connection.
func (k *Kernel) Close() error {
	if f, ok := k.Files.(fileService); ok && f.files != nil {
		_ = f.files.Close()
	}
	return k.db.Close()
}
