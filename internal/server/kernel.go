package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"athena-erp/gen/athena/v1/athenav1connect"
	"athena-erp/internal/auth"
	"athena-erp/internal/lifecycle"
	"athena-erp/internal/store"
	"athena-erp/internal/workspace"
)

// Kernel gives the CLI the same Record Type and Record services the
// ConnectRPC handlers use, over its own database connection. It adds no
// endpoint.
type Kernel struct {
	Workspace   athenav1connect.WorkspaceServiceHandler
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
	// Rules is the RuleService handler (Starlark Rules, ADR-0016).
	Rules athenav1connect.RuleServiceHandler
	// Hooks takes Go hooks for the lifecycle events (spec 5.2).
	Hooks *lifecycle.Registry
	db    *sql.DB
}

// OpenKernel connects to the Workspace database and applies pending system
// migrations, as Serve does at start-up. When workspace is set, the database
// must belong to it. Close the Kernel when done.
func OpenKernel(ctx context.Context, endpoint *workspace.DBEndpoint, svc *Workspace) (*Kernel, error) {
	db, err := openMigrationDB(ctx, endpoint, svc)
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
	users := &auth.Service{DB: db}
	if err := users.EnsureSystemRoles(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("seed system roles: %w", err)
	}
	// The generated password, if any, goes to the process log (stderr), so
	// command output stays clean for scripts and --json.
	if generated, err := users.EnsureRoot(ctx, endpoint.Password()); err != nil {
		db.Close()
		return nil, fmt.Errorf("seed root administrator: %w", err)
	} else if generated != "" {
		log.Printf("Administrator account created with a generated password, shown once: %s", generated)
	}
	files := newFileService(svc, db)
	hooks := lifecycle.NewRegistry()
	registerRules(hooks, db)
	registerDurableEvents(hooks)
	var slug string
	if svc != nil {
		slug = svc.Slug
	}
	return &Kernel{Workspace: newWorkspaceService(svc, db), RecordTypes: recordTypeService{db: db}, Records: recordService{db: db, hooks: hooks, workspace: slug}, System: systemService{db: db}, Auth: authService{auth: users}, Users: users, Files: files, Rules: newRuleService(db), Hooks: hooks, db: db}, nil
}

// Close releases the database connection.
func (k *Kernel) Close() error {
	if f, ok := k.Files.(fileService); ok && f.files != nil {
		_ = f.files.Close()
	}
	if w, ok := k.Workspace.(workspaceService); ok && w.files != nil {
		_ = w.files.Close()
	}
	return k.db.Close()
}
