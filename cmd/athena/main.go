package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	v1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/brand"
	"athena-erp/internal/server"
	"athena-erp/internal/workspace"
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

// cliCommand is one athena command: its name, its one-line menu summary
// and its handler. The interactive menu lists this table, so a newly
// registered command appears there with no other change.
type cliCommand struct {
	name  string
	short string
	run   func(args []string, out io.Writer) error
}

var commands = []cliCommand{
	{name: "init", short: "Create a home and save its MariaDB server login.", run: func(args []string, _ io.Writer) error { return initCommand(args) }},
	{name: "new", short: "Create a workspace in the current home, with its own database.", run: func(args []string, out io.Writer) error { return newCommand(args, out) }},
	{name: "run", short: "Run a workspace.", run: func(args []string, _ io.Writer) error { return runCommand(args) }},
	{name: "serve", short: "Run against ATHENA_DATABASE_URL without a workspace (development).", run: func(args []string, _ io.Writer) error { return serveCommand(args) }},
	{name: "migrate", short: "Show or apply system migrations.", run: func(args []string, out io.Writer) error { return migrateCommand(args, out) }},
	{name: "audit", short: "Verify the Audit Log hash chain.", run: func(args []string, out io.Writer) error { return auditCommand(args, out) }},
	{name: "record-types", short: "Read and change Record Types.", run: func(args []string, out io.Writer) error { return recordTypesCommand(args, out) }},
	{name: "records", short: "List, create and delete Records; submit, cancel and amend documents.", run: func(args []string, out io.Writer) error { return recordsCommand(args, out) }},
	{name: "expressions", short: "Type-check and evaluate expressions.", run: func(args []string, out io.Writer) error { return expressionsCommand(args, out) }},
	{name: "security", short: "Show or change session and rate settings.", run: func(args []string, out io.Writer) error { return securityCommand(args, out) }},
	{name: "user", short: "Manage sign-in users.", run: func(args []string, out io.Writer) error { return userCommand(args, out) }},
	{name: "role", short: "Manage Roles and their Grants.", run: func(args []string, out io.Writer) error { return roleCommand(args, out) }},
	{name: "access-log", short: "Print sign-ins, failures, lockouts, denials and access changes.", run: func(args []string, out io.Writer) error { return accessLogCommand(args, out) }},
	{name: "files", short: "List, read, write, rename and delete workspace files.", run: func(args []string, out io.Writer) error { return filesCommand(args, os.Stdin, out) }},
	{name: "workspace", short: "Show or change Workspace identity and appearance.", run: func(args []string, out io.Writer) error { return workspaceCommand(args, out) }},
	{name: "notifications", short: "Read an inbox as the operator.", run: func(args []string, out io.Writer) error { return notificationsCommand(args, out) }},
	{name: "ledger", short: "Manage the kernel's ledger masters.", run: func(args []string, out io.Writer) error { return ledgerCommand(args, out) }},
	{name: "post", short: "Preview, submit and cancel voucher postings.", run: func(args []string, out io.Writer) error { return postCommand(args, out) }},
	{name: "jobs", short: "List jobs and schedule occurrences; run dead work again.", run: func(args []string, out io.Writer) error { return jobsCommand(args, out) }},
	{name: "events", short: "List events and outbox deliveries; emit dead work again.", run: func(args []string, out io.Writer) error { return eventsCommand(args, out) }},
	{name: "secrets", short: "Register secret references and rotate keys.", run: func(args []string, out io.Writer) error { return secretsCommand(args, out) }},
	{name: "rules", short: "Manage Starlark Rules.", run: func(args []string, out io.Writer) error { return rulesCommand(args, out) }},
	{name: "lists", short: "Run saved lists and export them (PDF).", run: func(args []string, out io.Writer) error { return listsCommand(args, out) }},
	{name: "doctor", short: "Check the database, migrations, disk, ports and UI.", run: func(args []string, out io.Writer) error { return doctorCommand(args, out) }},
	{name: "version", short: "Print the version.", run: func(_ []string, out io.Writer) error { _, err := fmt.Fprintln(out, versionBanner()); return err }},
	{name: "help", short: "Show every command with its flags.", run: func(_ []string, out io.Writer) error { printUsageTo(out); return nil }},
}

func lookupCommand(name string) *cliCommand {
	for i := range commands {
		if commands[i].name == name {
			return &commands[i]
		}
	}
	return nil
}

func run(args []string) error {
	if len(args) == 0 {
		if isInteractive(os.Stdin, os.Stdout) {
			return runMenu(os.Stdin, os.Stdout)
		}
		printUsage()
		return nil
	}
	name := args[0]
	switch name {
	case "--version", "-v":
		name = "version"
	case "--help", "-h":
		name = "help"
	}
	if cmd := lookupCommand(name); cmd != nil {
		return cmd.run(args[1:], os.Stdout)
	}
	printUsage()
	return fmt.Errorf("unknown command %q", args[0])
}

// asciiMark is the static Athena tree for the CLI banner: a copy of the
// light final frame in web/src/art/logo-frames.ts (docs/design/ascii-logo.md
// explains the sync; web/src/art/logo-sync.test.ts enforces it).
const asciiMark = `##   ##    # ##      # #      ## #
### #  ###       ##  ### ## ##  ##  #
 #######         ######  ##   ###    #
   #############           ###########
     ######
        ####
            #############
                 ####`

func usageBanner() string {
	return asciiMark + "\n\nAthena ERP " + version
}

// versionBanner keeps the banner above the version lines (F8 owns the
// lines below it).
func versionBanner() string {
	return asciiMark + "\n" + version
}

func printUsage() {
	printUsageTo(os.Stdout)
}

func printUsageTo(out io.Writer) {
	fmt.Fprint(out, usageBanner())
	fmt.Fprintf(out, `

Usage:
  athena init [directory] [--host 127.0.0.1] [--port 3306] [--user name]
      Create a home and save its MariaDB server login. The password is
      read from ATHENA_SERVER_PASSWORD or prompted for.
  athena new [name] [--driver mariadb|postgres] [--logo file]
      Create a workspace in the current home, with its own database.
      The flags work before the name too. --driver must match the home
      driver; postgres is untested. --logo sets the light-theme logo
      (PNG or SVG, at most 512 KB).
  athena run [--workspace slug] [--addr %s]
      Run a workspace. Inside a workspace directory the workspace is found
      automatically; from the home pass --workspace.
  athena serve [--addr %s]
      Run against ATHENA_DATABASE_URL without a workspace (development).
  athena migrate status [--workspace slug] [--json]
      List the system migrations and whether the database has applied them.
      Writes nothing. Without a workspace it uses ATHENA_DATABASE_URL.
  athena migrate up [--workspace slug] [--to N] [--dry-run] [--json]
      Apply pending system migrations. The server also applies them at start.
      --to N stops after migration N; --dry-run lists them without applying.
  athena record-types list|show|save|delete [--workspace slug] [--json]
      Read and change Record Types with the same kernel code as the desk:
      kind, class, parent and field attributes. "save --file" takes a JSON
      RecordType; --dry-run prints the plan, --confirm-data-loss allows
      steps that delete stored values. "delete <key> --yes" drops the table.
  athena records list|lookup|create|delete|submit|cancel|amend|versions|activity|history <record-type> [--workspace slug] [--json]
      List, create and delete Records. create takes --values '{"field":"text"}',
      for child Record Types --parent id, and --name for a manual document name;
      delete takes <id> --revision N. lookup finds Records by display label.
      submit, cancel and amend run the document lifecycle (<id> --revision N);
      versions, activity and history print a document's snapshots, timeline and
      Audit Log rows.
  athena expressions check|eval|preview <record-type> [expression] [--workspace slug] [--json]
      Type-check an expression against a Record Type, evaluate it for one
      Record (--record id), or calculate formulas and field states for values
      that are not saved (--values). records list also takes --filter.
  athena security show|set [key value] [--workspace slug]
  athena user invite|list|set|resend|reset|suspend|reactivate|sessions <username> [--workspace slug] [--json]
      Manage sign-in users. "invite <username> --owner" creates an instance
      owner, which the first start of the desk needs; owners hold the
      Instance Owner role and are created here only (create is the same
      command under its old name). Users choose their own passwords through
      one-time challenges. set takes --roles a,b, --disable, --enable,
      --unlock and --display-name.
  athena role list|show|save|delete|assign|revoke|explain [name] [--workspace slug] [--json]
      Manage Roles and their Grants. "save --file role.json" takes a JSON
      Role: system permissions (build, audit, files:read, files:write, rules:read, rules:write, ledger:read, ledger:write, settings:appearance, settings:workspace, settings:security, access:ownership, access:users, access:roles, access:assign, access:recover, access:sessions, access:explore) and Grants of actions
      on a Record Type with a permission level and an optional row condition.
  athena access-log [--event name] [--limit N] [--workspace slug] [--json]
      Print sign-ins, failures, lockouts, denials and access changes.
  athena files list|get|put|mv|rm|history [path] [--workspace slug] [--json]
      List, read, write, rename and delete the files of a workspace (record_types/,
      rules/, packs/, generated/) with the same checks as the desk: paths stay
      inside the folders, files are UTF-8 text up to 1 MiB, Record Type files are
      validated, and every change keeps a version and an Audit Log entry. "list"
      shows which file backs which Record Type, Rule or Report. "put" reads
      --file or standard input; --expected-hash, --new and "rm --yes" guard
      against overwriting. "history" lists versions and "get --revision N" reads one.
  athena notifications list|read|read-all [--user name] [--workspace slug]
      Read an inbox as the operator. Defaults to administrator.
  athena workspace appearance show|set [--workspace slug]
      Choose a preset or custom accent and chart colors.
  athena workspace identity show|set [--workspace slug]
      Show or set the display name and the light and dark logos (--name,
      --logo-light file, --logo-dark file).
  athena ledger company|period|currency|account|determination|setting ... [--workspace slug] [--json]
      Manage the kernel's ledger masters. company save|list, period
      save|list|close|reopen, currency save|list|rate-save|rate-get, account
      type-save|type-list|save|list, determination save|list|resolve, setting
      save|list|save-value|publish|discard|list-values|resolve|preview. Run
      "athena ledger" with no nouns for every flag.
  athena post preview|submit|cancel|status|log <record-type> <id> [--workspace slug] [--json]
      Preview the plans a submit would write, submit or cancel a voucher,
      and show its posting groups and attempts. submit takes --revision N
      and refuses when the plan changed since --preview-hash H.
  athena jobs list|show|retry|occurrences [--workspace slug] [--json]
      List jobs newest first, show one with its payload and result, run dead
      work again as a new job (the dead row stays), and list schedule
      occurrences in period order. list takes --status, --kind, --limit N
      and --page token; occurrences takes --schedule, --scope and --status.
  athena events list|show|outbox|requeue|subscriptions [--workspace slug] [--json]
      List committed events newest first, show one envelope, list outbox
      delivery rows, emit a dead delivery again as a fresh event (the dead
      row stays), and list the declared consumer subscriptions.
  athena secrets save|rotate [--workspace slug] [--json]
      Register a secret reference (never a value) and rotate a key to a
      new active version from a 32-byte file, retiring the old one.
      rotate takes --key, --from and --key-dir ($ATHENA_SECRETS_DIR).
  athena lists export <id> --pdf [--out path] [--max-rows N] [--workspace slug]
      Render a saved list to PDF bytes (Workspace name, logo, title,
      filter and time in a repeated header).
  athena rules list|show|save|rm|versions|simulate|runs [key] [--workspace slug] [--json]
      Manage Starlark Rules (validation, derivation, guard, transform, posting). Rules run
      at document events inside the save, through Go host functions only, under
      step and time limits, with no files, network, clock or floats. "save --file
      rule.star" creates a version; "simulate" is a rolled-back dry run with a diff;
      "runs" prints the Rule Run log. Memory-guarded, proposed (ADR-0016, ADR-0020).
  athena audit verify|anchor [--anchor seq:hash] [--workspace slug] [--json]
      verify checks the Audit Log hash chain from the first row to the last and
      prints the chain head. anchor verifies, then prints only "seq:hash" for a
      daily job to store outside the database. Both exit with an error when the
      chain is broken.
      These commands apply pending migrations first, as the server does.
  athena doctor [--workspace slug] [--addr 127.0.0.1:3000] [--json]
      Check that this machine can run Athena: MariaDB reachable, system
      migrations applied, local disk room, the HTTP address free and the
      UI built into the binary (node is only needed to build it). Exits 0
      when no check fails, 1 when any fails; the last line is a
      machine-readable doctor status= line.
  athena version
`, defaultAddr, defaultAddr)
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
	if err := workspace.InitHome(dir, cfg.FormatDSN()); err != nil {
		return err
	}
	fmt.Printf("Home ready at %s\nCreate a workspace with:\n  cd %s && athena new \"Workspace name\"\n", dir, dir)
	return nil
}

func newCommand(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("new", flag.ContinueOnError)
	driverFlag := flags.String("driver", "", "database driver the home must use (default: the home driver)")
	logoFlag := flags.String("logo", "", "light-theme logo file (PNG or SVG, at most 512 KB)")
	// Accept flags after the name as well as before it: the name is every
	// positional, wherever it sits.
	nameArgs := args
	flagArgs := []string{}
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") {
			nameArgs, flagArgs = args[:i], args[i:]
			break
		}
	}
	if ok, err := parse(flags, flagArgs); !ok {
		return err
	}
	home, err := workspace.FindHome(".")
	if err != nil {
		return err
	}
	loaded, err := workspace.LoadHome(home)
	if err != nil {
		return err
	}
	driver := loaded.Database.Driver
	if *driverFlag != "" && *driverFlag != driver {
		return fmt.Errorf("athena new: --driver %q does not match the home driver %q", *driverFlag, driver)
	}
	if driver == workspace.DriverPostgres {
		fmt.Fprintln(out, "warning: the PostgreSQL adapter is implemented but untested; MariaDB is the only tested engine")
	}
	// Read and validate the logo before asking for a name, so a bad path
	// or bad content fails fast instead of after an interactive prompt or
	// a created Workspace.
	var logo *v1.LogoUpload
	if *logoFlag != "" {
		var err error
		if logo, err = readLogo(*logoFlag); err != nil {
			return err
		}
		if _, err := brand.ValidateLogo(logo.GetFilename(), logo.GetContent()); err != nil {
			return err
		}
	}
	nameWords := append([]string{}, nameArgs...)
	nameWords = append(nameWords, flags.Args()...)
	name := strings.Join(nameWords, " ")
	if name == "" {
		if name, err = prompt(bufio.NewReader(os.Stdin), "Workspace name: "); err != nil {
			return err
		}
	}
	env, err := workspace.LoadEnv(filepath.Join(home, ".env.local"))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	result, err := workspace.Create(ctx, home, name, env[workspace.ServerURLEnv], version)
	if err != nil {
		return err
	}
	if logo != nil {
		if err := setNewLogo(ctx, logo, result, loaded.Database.Driver, env[workspace.ServerURLEnv]); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "Created %s in %s\nDatabase: %s\n", result.Manifest.Name, result.Root, result.Manifest.Database.Name)
	if result.GitInitialized {
		fmt.Fprintln(out, "Git repository initialized on main. Commit and add a remote when ready.")
	} else {
		fmt.Fprintln(out, "Git is not installed; initialize a repository yourself if you want one.")
	}
	fmt.Fprintf(out, "Run it with:\n  cd %s && athena run\n", result.Root)
	return nil
}

func runCommand(args []string) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	slug := flags.String("workspace", "", "workspace slug, when running from the home directory")
	addr := flags.String("addr", defaultAddr, "HTTP listen address (loopback by default)")
	if ok, err := parse(flags, args); !ok {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	p, cfg, err := resolveWorkspace(*slug)
	if err != nil {
		return err
	}
	lock, err := workspace.Acquire(workspace.Active{Workspace: p.Manifest.Name + " (" + p.Manifest.Slug + ")", Root: p.Root, Addr: *addr})
	if err != nil {
		return err
	}
	defer lock.Release()
	log.Printf("Running workspace %s from %s", p.Manifest.Name, p.Root)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.Serve(ctx, server.Config{
		Addr:      *addr,
		Database:  cfg,
		Workspace: &server.Workspace{ID: p.Manifest.ID, Name: p.Manifest.Name, Slug: p.Manifest.Slug, Root: p.Root},
		Version:   version,
	})
}

// resolveWorkspace finds a workspace and the database endpoint its home driver,
// server login and database name give.
func resolveWorkspace(slug string) (workspace.Workspace, *workspace.DBEndpoint, error) {
	p, err := workspace.Resolve(".", slug)
	if err != nil {
		return workspace.Workspace{}, nil, err
	}
	env, err := workspace.LoadEnv(filepath.Join(p.Root, ".env.local"), filepath.Join(p.Home, ".env.local"))
	if err != nil {
		return workspace.Workspace{}, nil, err
	}
	home, err := workspace.LoadHome(p.Home)
	if err != nil {
		return workspace.Workspace{}, nil, err
	}
	endpoint, err := workspace.ParseEndpoint(home.Database.Driver, env[workspace.ServerURLEnv], p.Manifest.Database.Name)
	if err != nil {
		return workspace.Workspace{}, nil, err
	}
	return p, endpoint, nil
}

func serveCommand(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := flags.String("addr", defaultAddr, "HTTP listen address (loopback by default)")
	filesDir := flags.String("files-dir", os.Getenv("ATHENA_FILES_DIR"), "directory the Files page manages (default $ATHENA_FILES_DIR; created if missing; empty disables Files)")
	if ok, err := parse(flags, args); !ok {
		return err
	}
	config := server.Config{Addr: *addr, Version: version}
	if *filesDir != "" {
		abs, err := filepath.Abs(*filesDir)
		if err != nil {
			return fmt.Errorf("resolve --files-dir: %w", err)
		}
		if err := os.MkdirAll(abs, 0o750); err != nil {
			return fmt.Errorf("create --files-dir: %w", err)
		}
		config.FilesRoot = abs
	}
	label := "athena serve (no database)"
	if dsn := os.Getenv("ATHENA_DATABASE_URL"); dsn != "" {
		endpoint, err := workspace.ParseDatabaseURL(dsn)
		if err != nil {
			return fmt.Errorf("parse ATHENA_DATABASE_URL: %w", err)
		}
		config.Database = endpoint
		label = "athena serve (database " + endpoint.DatabaseName() + ")"
	}
	lock, err := workspace.Acquire(workspace.Active{Workspace: label, Addr: *addr})
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
