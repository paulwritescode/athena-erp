package project

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// InitWorkspace writes athena-workspace.yaml and the server login into dir.
// It refuses to overwrite an existing workspace.
func InitWorkspace(dir, serverURL string) error {
	if exists(filepath.Join(dir, WorkspaceFile)) {
		return fmt.Errorf("%s already contains a workspace", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	ws := Workspace{FormatVersion: 1, Database: WorkspaceDatabase{Driver: "mariadb", CredentialsEnv: ServerURLEnv}}
	data, err := yaml.Marshal(ws)
	if err != nil {
		return err
	}
	header := "# Athena workspace. Each project is a sibling directory with its own database.\n# The server login is in .env.local, which must stay private.\n"
	if err := os.WriteFile(filepath.Join(dir, WorkspaceFile), append([]byte(header), data...), 0o644); err != nil {
		return err
	}
	env := fmt.Sprintf("# Database server login for every project in this workspace. Keep private.\n%s='%s'\n", ServerURLEnv, serverURL)
	if err := os.WriteFile(filepath.Join(dir, ".env.local"), []byte(env), 0o600); err != nil {
		return err
	}
	ignore := "# The server login and any other private settings.\n.env.local\n.env.*.local\n"
	if !exists(filepath.Join(dir, ".gitignore")) {
		return os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(ignore), 0o644)
	}
	return nil
}

// ServerConfig parses the workspace server DSN and returns the driver config
// for the given database name. Pass "" for a server-level connection.
func ServerConfig(serverURL, database string) (*mysql.Config, error) {
	if serverURL == "" {
		return nil, fmt.Errorf("%s is not set; run athena init or set it in the workspace .env.local", ServerURLEnv)
	}
	cfg, err := mysql.ParseDSN(serverURL)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", ServerURLEnv, err)
	}
	cfg.DBName = database
	return cfg, nil
}

// CreateResult describes a created project.
type CreateResult struct {
	Root     string
	Manifest Manifest
	// GitInitialized is false when git is not installed.
	GitInitialized bool
}

// Create makes a new project in the workspace: it stages the project files
// beside their final directory, creates the project database and records the
// project ID in it, then moves the files into place and initializes Git.
//
// Each step can be retried. A staging directory left by an interrupted run is
// reused with its ID and any edited files. An existing project directory, or a
// database that holds tables but no matching project ID, is never adopted.
func Create(ctx context.Context, workspace, name, serverURL, coreVersion string) (CreateResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return CreateResult{}, errors.New("a project name is required")
	}
	slug := Slugify(name)
	if err := ValidateSlug(slug); err != nil {
		return CreateResult{}, fmt.Errorf("%w (from name %q)", err, name)
	}
	if _, err := LoadWorkspace(workspace); err != nil {
		return CreateResult{}, err
	}
	final := filepath.Join(workspace, slug)
	if _, err := os.Lstat(final); err == nil {
		return CreateResult{}, fmt.Errorf("%s already exists; choose another name", final)
	}
	staging := filepath.Join(workspace, "."+slug+".creating")

	manifest, err := stageFiles(staging, name, slug, coreVersion)
	if err != nil {
		return CreateResult{}, fmt.Errorf("stage project files: %w", err)
	}
	if err := createDatabase(ctx, serverURL, manifest); err != nil {
		return CreateResult{}, err
	}
	if err := os.Rename(staging, final); err != nil {
		return CreateResult{}, fmt.Errorf("move project into place: %w", err)
	}
	result := CreateResult{Root: final, Manifest: manifest}
	if _, err := exec.LookPath("git"); err == nil && !exists(filepath.Join(final, ".git")) {
		cmd := exec.CommandContext(ctx, "git", "init", "--quiet", "--initial-branch=main")
		cmd.Dir = final
		if out, err := cmd.CombinedOutput(); err != nil {
			return result, fmt.Errorf("project created, but git init failed: %v: %s", err, out)
		}
		result.GitInitialized = true
	}
	return result, nil
}

// stageFiles writes any project files missing from the staging directory and
// returns its manifest, reusing the ID from an earlier interrupted run.
func stageFiles(staging, name, slug, coreVersion string) (Manifest, error) {
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return Manifest{}, err
	}
	manifest := Manifest{Name: name, Slug: slug, SchemaFormatVersion: SchemaFormatVersion, CoreVersion: coreVersion, Database: ProjectDatabase{Name: slug}}
	if exists(filepath.Join(staging, ManifestFile)) {
		previous, err := LoadManifest(staging)
		if err != nil {
			return Manifest{}, err
		}
		manifest = previous
	} else {
		id, err := uuid.NewV7()
		if err != nil {
			return Manifest{}, err
		}
		manifest.ID = id.String()
		data, err := yaml.Marshal(manifest)
		if err != nil {
			return Manifest{}, err
		}
		header := "# Athena project identity. The id, slug and database name are fixed at creation;\n# renaming the project changes only name.\n"
		if err := os.WriteFile(filepath.Join(staging, ManifestFile), append([]byte(header), data...), 0o644); err != nil {
			return Manifest{}, err
		}
	}
	readme := strings.NewReplacer("{{name}}", manifest.Name, "{{slug}}", manifest.Slug).Replace(readmeTemplate)
	files := map[string]string{
		"AGENTS.md":             agentsTemplate,
		".gitignore":            gitignoreTemplate,
		".env.example":          envExampleTemplate,
		"README.md":             readme,
		"records/.gitkeep":      "",
		"storage/uploads/.keep": "",
	}
	for rel, content := range files {
		path := filepath.Join(staging, rel)
		if exists(path) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return Manifest{}, err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return Manifest{}, err
		}
	}
	return manifest, nil
}

// createDatabase creates the project database and its athena_project row.
// An existing database is accepted only if it already carries this project's
// ID, or is empty (left by an interrupted run of this same creation).
func createDatabase(ctx context.Context, serverURL string, manifest Manifest) error {
	cfg, err := ServerConfig(serverURL, "")
	if err != nil {
		return err
	}
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return err
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect to the database server: %w", err)
	}
	name := manifest.Database.Name
	var existing int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.schemata WHERE LOWER(schema_name) = ?`, name).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 {
		var tables int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ?`, name).Scan(&tables); err != nil {
			return err
		}
		if tables > 0 {
			id, err := ProjectID(ctx, db, name)
			if err != nil || id != manifest.ID {
				return fmt.Errorf("database %q already exists and does not belong to this new project; choose another name", name)
			}
			return nil
		}
	} else {
		// name passed ValidateSlug, so it is safe inside backticks.
		if _, err := db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", name)); err != nil {
			return fmt.Errorf("create database %q: %w", name, err)
		}
	}
	statements := []string{
		fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s`.athena_project (id CHAR(36) NOT NULL PRIMARY KEY, slug VARCHAR(48) NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci", name),
		fmt.Sprintf("INSERT IGNORE INTO `%s`.athena_project (id, slug) VALUES (?, ?)", name),
	}
	if _, err := db.ExecContext(ctx, statements[0]); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, statements[1], manifest.ID, manifest.Slug)
	return err
}

// ProjectID returns the project ID recorded in a project database.
func ProjectID(ctx context.Context, db *sql.DB, database string) (string, error) {
	var id string
	err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT id FROM `%s`.athena_project LIMIT 1", database)).Scan(&id)
	return id, err
}
