// Package project manages Athena workspaces and the projects inside them.
//
// A workspace is a directory holding athena-workspace.yaml, the database
// server login in .env.local, and one sibling directory per project. Each
// project has its own athena.yaml and its own database, named from the
// project slug when it is created.
package project

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// WorkspaceFile marks a workspace root.
	WorkspaceFile = "athena-workspace.yaml"
	// ManifestFile marks a project root.
	ManifestFile = "athena.yaml"
	// ServerURLEnv names the variable holding the database server DSN,
	// without a database name.
	ServerURLEnv = "ATHENA_SERVER_URL"
	// SchemaFormatVersion is the project file layout this build writes.
	SchemaFormatVersion = 1
)

// Workspace is athena-workspace.yaml.
type Workspace struct {
	FormatVersion int               `yaml:"format_version"`
	Database      WorkspaceDatabase `yaml:"database"`
}

// WorkspaceDatabase names the database server every project in the workspace uses.
type WorkspaceDatabase struct {
	Driver string `yaml:"driver"`
	// CredentialsEnv names the environment variable with the server DSN.
	CredentialsEnv string `yaml:"credentials_env"`
}

// Manifest is a project's athena.yaml.
type Manifest struct {
	ID                  string          `yaml:"id"`
	Name                string          `yaml:"name"`
	Slug                string          `yaml:"slug"`
	SchemaFormatVersion int             `yaml:"schema_format_version"`
	CoreVersion         string          `yaml:"core_version"`
	Database            ProjectDatabase `yaml:"database"`
}

// ProjectDatabase is the project's own database on the workspace server.
type ProjectDatabase struct {
	Name string `yaml:"name"`
}

// Project is a resolved project: its manifest, root and workspace.
type Project struct {
	Manifest  Manifest
	Root      string
	Workspace string
}

// slugPattern keeps slugs valid as directory names, database names and
// MariaDB identifiers with room for prefixes.
var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,47}$`)

// reservedSlugs are MariaDB system schemas and names that would confuse tooling.
var reservedSlugs = map[string]bool{"mysql": true, "information_schema": true, "performance_schema": true, "sys": true, "test": true, "athena": true}

// Slugify derives a lowercase snake_case slug from a display name, for
// example "Acme Trading" becomes acme_trading.
func Slugify(name string) string {
	var b strings.Builder
	lastUnderscore := true
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		case !lastUnderscore:
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

// ValidateSlug reports why a slug cannot name a project, or nil.
func ValidateSlug(slug string) error {
	if !slugPattern.MatchString(slug) {
		return fmt.Errorf("project slug %q must start with a letter and use up to 48 lowercase letters, digits or underscores", slug)
	}
	if reservedSlugs[slug] {
		return fmt.Errorf("project slug %q is reserved", slug)
	}
	return nil
}

// LoadWorkspace reads athena-workspace.yaml from dir.
func LoadWorkspace(dir string) (Workspace, error) {
	var ws Workspace
	if err := readYAML(filepath.Join(dir, WorkspaceFile), &ws); err != nil {
		return ws, err
	}
	if ws.Database.Driver != "mariadb" {
		return ws, fmt.Errorf("%s: database driver %q is not supported; use mariadb", WorkspaceFile, ws.Database.Driver)
	}
	if ws.Database.CredentialsEnv == "" {
		ws.Database.CredentialsEnv = ServerURLEnv
	}
	return ws, nil
}

// LoadManifest reads and validates a project's athena.yaml.
func LoadManifest(root string) (Manifest, error) {
	var m Manifest
	if err := readYAML(filepath.Join(root, ManifestFile), &m); err != nil {
		return m, err
	}
	if m.ID == "" || m.Name == "" {
		return m, fmt.Errorf("%s: id and name are required", filepath.Join(root, ManifestFile))
	}
	if err := ValidateSlug(m.Slug); err != nil {
		return m, err
	}
	if m.Database.Name != m.Slug {
		return m, fmt.Errorf("%s: database.name must equal slug %q", filepath.Join(root, ManifestFile), m.Slug)
	}
	if m.SchemaFormatVersion > SchemaFormatVersion {
		return m, fmt.Errorf("project %q uses schema format %d; this Athena supports up to %d", m.Slug, m.SchemaFormatVersion, SchemaFormatVersion)
	}
	return m, nil
}

// FindWorkspace walks up from dir to the nearest workspace root.
func FindWorkspace(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if exists(filepath.Join(dir, WorkspaceFile)) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no Athena workspace found here or in any parent directory; run athena init first")
		}
		dir = parent
	}
}

// Resolve finds the project to run. With a slug it selects that project in
// the workspace. Without one it walks up from dir to the nearest athena.yaml,
// stopping at the workspace root, which must then be given a slug.
func Resolve(dir, slug string) (Project, error) {
	workspace, err := FindWorkspace(dir)
	if err != nil {
		return Project{}, err
	}
	root := ""
	if slug != "" {
		if err := ValidateSlug(slug); err != nil {
			return Project{}, err
		}
		root = filepath.Join(workspace, slug)
	} else {
		current, err := filepath.Abs(dir)
		if err != nil {
			return Project{}, err
		}
		for current != workspace {
			if exists(filepath.Join(current, ManifestFile)) {
				root = current
				break
			}
			current = filepath.Dir(current)
		}
		if root == "" {
			return Project{}, errors.New("this is the workspace, not a project; run from a project directory or pass --project <slug>")
		}
	}
	// A project must be a direct child of its workspace, and not a symlink out of it.
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return Project{}, fmt.Errorf("project %q not found in %s", filepath.Base(root), workspace)
	}
	resolvedWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return Project{}, err
	}
	if filepath.Dir(resolved) != resolvedWorkspace {
		return Project{}, fmt.Errorf("project %s is not directly inside workspace %s", root, workspace)
	}
	manifest, err := LoadManifest(root)
	if err != nil {
		return Project{}, err
	}
	if manifest.Slug != filepath.Base(root) {
		return Project{}, fmt.Errorf("%s: slug %q does not match its directory %q", filepath.Join(root, ManifestFile), manifest.Slug, filepath.Base(root))
	}
	return Project{Manifest: manifest, Root: root, Workspace: workspace}, nil
}

// LoadEnv reads KEY=value lines from the given .env files, earlier files
// taking precedence. Variables already set in the process environment win
// over every file. Missing files are skipped.
func LoadEnv(files ...string) (map[string]string, error) {
	values := map[string]string{}
	for i := len(files) - 1; i >= 0; i-- {
		f, err := os.Open(files[i])
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			value = strings.TrimSpace(value)
			if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') && value[len(value)-1] == value[0] {
				value = value[1 : len(value)-1]
			}
			values[strings.TrimSpace(key)] = value
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}
	for key := range values {
		if v, ok := os.LookupEnv(key); ok {
			values[key] = v
		}
	}
	return values, nil
}

func readYAML(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
