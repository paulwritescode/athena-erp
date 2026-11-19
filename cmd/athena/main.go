package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"athena-erp/internal/project"
	"athena-erp/internal/server"
	"github.com/go-sql-driver/mysql"
	"golang.org/x/term"
)

const (
	version     = "0.1.0-dev"
	defaultAddr = "127.0.0.1:3000"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Printf("athena: %v", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}
	switch args[0] {
	case "init":
		return initCommand(args[1:])
	case "new":
		return newCommand(args[1:])
	case "run":
		return runCommand(args[1:])
	case "serve":
		return serveCommand(args[1:])
	case "migrate":
		return migrateCommand(args[1:], os.Stdout)
	case "version", "--version", "-v":
		fmt.Println(version)
		return nil
	case "help", "--help", "-h":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage() {
	fmt.Printf(`Athena ERP %s

Usage:
  athena init [directory] [--host 127.0.0.1] [--port 3306] [--user name]
      Create a workspace and save its MariaDB server login. The password is
      read from ATHENA_SERVER_PASSWORD or prompted for.
  athena new [name]
      Create a project in the current workspace, with its own database.
  athena run [--project slug] [--addr %s]
      Run a project. Inside a project directory the project is found
      automatically; from the workspace pass --project.
  athena serve [--addr %s]
      Run against ATHENA_DATABASE_URL without a project (development).
  athena migrate status [--project slug] [--json]
      List the system migrations and whether the database has applied them.
      Writes nothing. Without a project it uses ATHENA_DATABASE_URL.
  athena migrate up [--project slug] [--to N] [--dry-run] [--json]
      Apply pending system migrations. The server also applies them at start.
      --to N stops after migration N; --dry-run lists them without applying.
  athena version
`, version, defaultAddr, defaultAddr)
}

func parse(flags *flag.FlagSet, args []string) (bool, error) {
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func initCommand(args []string) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	host := flags.String("host", "127.0.0.1", "MariaDB host")
	port := flags.Int("port", 3306, "MariaDB port")
	user := flags.String("user", "", "MariaDB user that can create databases")
	if ok, err := parse(flags, args); !ok {
		return err
	}
	dir := "."
	if flags.NArg() > 0 {
		// Accept flags after the directory as well as before it.
		dir = flags.Arg(0)
		if ok, err := parse(flags, flags.Args()[1:]); !ok {
			return err
		}
		if flags.NArg() > 0 {
			return fmt.Errorf("unexpected argument %q", flags.Arg(0))
		}
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	in := bufio.NewReader(os.Stdin)
	if *user == "" {
		if *user, err = prompt(in, "MariaDB user: "); err != nil {
			return err
		}
	}
	password, ok := os.LookupEnv("ATHENA_SERVER_PASSWORD")
	if !ok {
		if password, err = promptSecret("MariaDB password: "); err != nil {
			return err
		}
	}
	cfg := mysql.NewConfig()
	cfg.User = *user
	cfg.Passwd = password
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(*host, strconv.Itoa(*port))
	if err := ping(cfg); err != nil {
		return fmt.Errorf("cannot connect to MariaDB at %s as %s: %w", cfg.Addr, cfg.User, err)
	}
	if err := project.InitWorkspace(dir, cfg.FormatDSN()); err != nil {
		return err
	}
	fmt.Printf("Workspace ready at %s\nCreate a project with:\n  cd %s && athena new \"Project name\"\n", dir, dir)
	return nil
}

func newCommand(args []string) error {
	flags := flag.NewFlagSet("new", flag.ContinueOnError)
	if ok, err := parse(flags, args); !ok {
		return err
	}
	workspace, err := project.FindWorkspace(".")
	if err != nil {
		return err
	}
	name := strings.Join(flags.Args(), " ")
	if name == "" {
		if name, err = prompt(bufio.NewReader(os.Stdin), "Project name: "); err != nil {
			return err
		}
	}
	env, err := project.LoadEnv(filepath.Join(workspace, ".env.local"))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	result, err := project.Create(ctx, workspace, name, env[project.ServerURLEnv], version)
	if err != nil {
		return err
	}
	fmt.Printf("Created %s in %s\nDatabase: %s\n", result.Manifest.Name, result.Root, result.Manifest.Database.Name)
	if result.GitInitialized {
		fmt.Println("Git repository initialized on main. Commit and add a remote when ready.")
	} else {
		fmt.Println("Git is not installed; initialize a repository yourself if you want one.")
	}
	fmt.Printf("Run it with:\n  cd %s && athena run\n", result.Root)
	return nil
}

func runCommand(args []string) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	slug := flags.String("project", "", "project slug, when running from the workspace directory")
	addr := flags.String("addr", defaultAddr, "HTTP listen address (loopback by default)")
	if ok, err := parse(flags, args); !ok {
		return err
	}
	p, cfg, err := resolveProject(*slug)
	if err != nil {
		return err
	}
	lock, err := project.Acquire(project.Active{Project: p.Manifest.Name + " (" + p.Manifest.Slug + ")", Root: p.Root, Addr: *addr})
	if err != nil {
		return err
	}
	defer lock.Release()
	log.Printf("Running project %s from %s", p.Manifest.Name, p.Root)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.Serve(ctx, server.Config{
		Addr:     *addr,
		Database: cfg,
		Project:  &server.Project{ID: p.Manifest.ID, Name: p.Manifest.Name, Slug: p.Manifest.Slug},
		Version:  version,
	})
}

// resolveProject finds a project and the database config its server login and
// database name give.
func resolveProject(slug string) (project.Project, *mysql.Config, error) {
	p, err := project.Resolve(".", slug)
	if err != nil {
		return project.Project{}, nil, err
	}
	env, err := project.LoadEnv(filepath.Join(p.Root, ".env.local"), filepath.Join(p.Workspace, ".env.local"))
	if err != nil {
		return project.Project{}, nil, err
	}
	cfg, err := project.ServerConfig(env[project.ServerURLEnv], p.Manifest.Database.Name)
	if err != nil {
		return project.Project{}, nil, err
	}
	return p, cfg, nil
}

func serveCommand(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := flags.String("addr", defaultAddr, "HTTP listen address (loopback by default)")
	if ok, err := parse(flags, args); !ok {
		return err
	}
	config := server.Config{Addr: *addr, Version: version}
	label := "athena serve (no database)"
	if dsn := os.Getenv("ATHENA_DATABASE_URL"); dsn != "" {
		cfg, err := mysql.ParseDSN(dsn)
		if err != nil {
			return fmt.Errorf("parse ATHENA_DATABASE_URL: %w", err)
		}
		config.Database = cfg
		label = "athena serve (database " + cfg.DBName + ")"
	}
	lock, err := project.Acquire(project.Active{Project: label, Addr: *addr})
	if err != nil {
		return err
	}
	defer lock.Release()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.Serve(ctx, config)
}

func ping(cfg *mysql.Config) error {
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return err
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return db.PingContext(ctx)
}

func prompt(in *bufio.Reader, label string) (string, error) {
	fmt.Print(label)
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read %s: %w", strings.TrimSuffix(label, ": "), err)
	}
	return strings.TrimSpace(line), nil
}

func promptSecret(label string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("no terminal for the password prompt; set ATHENA_SERVER_PASSWORD")
	}
	fmt.Print(label)
	secret, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	return string(secret), err
}
