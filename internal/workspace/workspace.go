// Package workspace manages Athena homes and the workspaces inside them.
//
// A home is a directory holding athena-home.yaml, the database
// server login in .env.local, and one sibling directory per workspace. Each
// workspace has its own athena.yaml and its own database, named from the
// workspace slug when it is created.
package workspace

import (
	"bufio"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gopkg.in/yaml.v3"
)

const (
	// HomeFile marks a home root.
	HomeFile = "athena-home.yaml"
	// ManifestFile marks a workspace root.
	ManifestFile = "athena.yaml"
	// ServerURLEnv names the variable holding the database server DSN,
	// without a database name.
	ServerURLEnv = "ATHENA_SERVER_URL"
	// SchemaFormatVersion is the workspace file layout this build writes.
	SchemaFormatVersion = 1
	// DriverMariaDB and DriverPostgres are the supported database drivers.
	// Only MariaDB is tested; the PostgreSQL adapter is implemented but
	// untested and claims no parity.
	DriverMariaDB  = "mariadb"
	DriverPostgres = "postgres"
)

// Home is athena-home.yaml.
type Home struct {
	FormatVersion int          `yaml:"format_version"`
	Database      HomeDatabase `yaml:"database"`
}

// HomeDatabase names the database server every workspace in the home uses.
type HomeDatabase struct {
	Driver string `yaml:"driver"`
	// CredentialsEnv names the environment variable with the server DSN.
	CredentialsEnv string `yaml:"credentials_env"`
}

// Manifest is a workspace's athena.yaml.
type Manifest struct {
	ID                  string            `yaml:"id"`
	Name                string            `yaml:"name"`
	Slug                string            `yaml:"slug"`
	SchemaFormatVersion int               `yaml:"schema_format_version"`
	CoreVersion         string            `yaml:"core_version"`
	Database            WorkspaceDatabase `yaml:"database"`
}

// WorkspaceDatabase is the workspace's own database on the home server.
type WorkspaceDatabase struct {
	Name string `yaml:"name"`
}

// Workspace is a resolved workspace: its manifest, root and home.
type Workspace struct {
	Manifest Manifest
	Root     string
	Home     string
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

// ValidateSlug reports why a slug cannot name a workspace, or nil.
func ValidateSlug(slug string) error {
	if !slugPattern.MatchString(slug) {
		return fmt.Errorf("workspace slug %q must start with a letter and use up to 48 lowercase letters, digits or underscores", slug)
	}
	if reservedSlugs[slug] {
		return fmt.Errorf("workspace slug %q is reserved", slug)
	}
	return nil
}

// LoadHome reads athena-home.yaml from dir.
func LoadHome(dir string) (Home, error) {
	var home Home
	if err := readYAML(filepath.Join(dir, HomeFile), &home); err != nil {
		return home, err
	}
	if home.Database.Driver != DriverMariaDB && home.Database.Driver != DriverPostgres {
		return home, fmt.Errorf("%s: database driver %q is not supported; use mariadb or postgres", HomeFile, home.Database.Driver)
	}
	if home.Database.CredentialsEnv == "" {
		home.Database.CredentialsEnv = ServerURLEnv
	}
	return home, nil
}

// LoadManifest reads and validates a workspace's athena.yaml.
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
		return m, fmt.Errorf("workspace %q uses schema format %d; this Athena supports up to %d", m.Slug, m.SchemaFormatVersion, SchemaFormatVersion)
	}
	return m, nil
}

// FindHome walks up from dir to the nearest home root.
func FindHome(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if upgraded, err := UpgradeLegacyHome(dir); err != nil {
			return "", err
		} else if upgraded {
			fmt.Fprintf(os.Stderr, "athena: renamed %s to %s in %s\n", legacyHomeFile, HomeFile, dir)
		}
		if exists(filepath.Join(dir, HomeFile)) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no Athena home found here or in any parent directory; run athena init first")
		}
		dir = parent
	}
}

// Resolve finds the workspace to run. With a slug it selects that workspace in
// the home. Without one it walks up from dir to the nearest athena.yaml,
// stopping at the home root, which must then be given a slug.
func Resolve(dir, slug string) (Workspace, error) {
	home, err := FindHome(dir)
	if err != nil {
		return Workspace{}, err
	}
	root := ""
	if slug != "" {
		if err := ValidateSlug(slug); err != nil {
			return Workspace{}, err
		}
		root = filepath.Join(home, slug)
	} else {
		current, err := filepath.Abs(dir)
		if err != nil {
			return Workspace{}, err
		}
		for current != home {
			if exists(filepath.Join(current, ManifestFile)) {
				root = current
				break
			}
			current = filepath.Dir(current)
		}
		if root == "" {
			return Workspace{}, errors.New("this is the home, not a workspace; run from a workspace directory or pass --workspace <slug>")
		}
	}
	// A workspace must be a direct child of its home, and not a symlink out of it.
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return Workspace{}, fmt.Errorf("workspace %q not found in %s", filepath.Base(root), home)
	}
	resolvedHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return Workspace{}, err
	}
	if filepath.Dir(resolved) != resolvedHome {
		return Workspace{}, fmt.Errorf("workspace %s is not directly inside home %s", root, home)
	}
	manifest, err := LoadManifest(root)
	if err != nil {
		return Workspace{}, err
	}
	if manifest.Slug != filepath.Base(root) {
		return Workspace{}, fmt.Errorf("%s: slug %q does not match its directory %q", filepath.Join(root, ManifestFile), manifest.Slug, filepath.Base(root))
	}
	return Workspace{Manifest: manifest, Root: root, Home: home}, nil
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

// DBEndpoint carries the driver plus its parsed connection config so server
// and CLI open paths switch on one value. Exactly one of MySQL or Postgres
// is set, matching Driver.
type DBEndpoint struct {
	Driver   string
	MySQL    *mysql.Config
	Postgres *pgx.ConnConfig
}

// ParseEndpoint parses a server URL plus database name into a DBEndpoint for
// the given driver. MariaDB URLs are classic DSNs ("user:pass@tcp(host:port)/"
// with no database); PostgreSQL URLs are postgres:// connection strings with
// no database path segment. The database name is set on the parsed config.
func ParseEndpoint(driver, serverURL, database string) (*DBEndpoint, error) {
	if serverURL == "" {
		return nil, fmt.Errorf("%s is not set; run athena init or set it in the home .env.local", ServerURLEnv)
	}
	switch driver {
	case DriverPostgres:
		pgcfg, err := pgx.ParseConfig(serverURL)
		if err != nil {
			return nil, fmt.Errorf("parse postgres server URL: %w", err)
		}
		if pgcfg.Database != "" {
			return nil, fmt.Errorf("postgres server URL must not include a database name; got %q", pgcfg.Database)
		}
		pgcfg.Database = database
		if pgcfg.RuntimeParams == nil {
			pgcfg.RuntimeParams = map[string]string{}
		}
		pgcfg.RuntimeParams["timezone"] = "UTC"
		pgcfg.RuntimeParams["datestyle"] = "ISO, MDY"
		return &DBEndpoint{Driver: DriverPostgres, Postgres: pgcfg}, nil
	case "", DriverMariaDB:
		mycfg, err := mysql.ParseDSN(serverURL)
		if err != nil {
			return nil, fmt.Errorf("parse MariaDB server URL: %w", err)
		}
		if mycfg.DBName != "" {
			return nil, fmt.Errorf("MariaDB server URL must not include a database name; got %q", mycfg.DBName)
		}
		mycfg.DBName = database
		return &DBEndpoint{Driver: DriverMariaDB, MySQL: mycfg}, nil
	default:
		return nil, fmt.Errorf("database driver %q is not supported; use mariadb or postgres", driver)
	}
}

// Password returns the server login password carried by the endpoint.
func (e *DBEndpoint) Password() string {
	if e.Driver == DriverPostgres {
		if e.Postgres == nil {
			return ""
		}
		return e.Postgres.Password
	}
	if e.MySQL == nil {
		return ""
	}
	return e.MySQL.Passwd
}

// DatabaseName returns the database name carried by the endpoint.
func (e *DBEndpoint) DatabaseName() string {
	if e.Driver == DriverPostgres {
		if e.Postgres == nil {
			return ""
		}
		return e.Postgres.Database
	}
	if e.MySQL == nil {
		return ""
	}
	return e.MySQL.DBName
}

// Connector returns a database/sql connector for the endpoint. The MariaDB
// connector keeps session defaults (multiStatements/parseTime); callers that
// need a UTC session must set Loc=UTC before calling.
func (e *DBEndpoint) Connector() (driver.Connector, error) {
	switch e.Driver {
	case DriverPostgres:
		return stdlib.GetConnector(*e.Postgres), nil
	case DriverMariaDB:
		return mysql.NewConnector(e.MySQL)
	default:
		return nil, fmt.Errorf("database driver %q is not supported; use mariadb or postgres", e.Driver)
	}
}

// ParseDatabaseURL parses a full database URL, server plus database name, for
// the serve and migrate fallbacks outside a home. A postgres:// or
// postgresql:// scheme selects PostgreSQL; anything else parses as a MariaDB
// DSN with its existing errors.
func ParseDatabaseURL(dsn string) (*DBEndpoint, error) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		pgcfg, err := pgx.ParseConfig(dsn)
		if err != nil {
			return nil, fmt.Errorf("invalid postgres URL: %w", err)
		}
		if pgcfg.Database == "" {
			return nil, errors.New("invalid postgres URL: no database name in the path")
		}
		if pgcfg.RuntimeParams == nil {
			pgcfg.RuntimeParams = map[string]string{}
		}
		pgcfg.RuntimeParams["timezone"] = "UTC"
		pgcfg.RuntimeParams["datestyle"] = "ISO, MDY"
		return &DBEndpoint{Driver: DriverPostgres, Postgres: pgcfg}, nil
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	return &DBEndpoint{Driver: DriverMariaDB, MySQL: cfg}, nil
}
