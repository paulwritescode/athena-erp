package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"athena-erp/internal/secrets"
)

// keyDirEnv names the environment variable holding the key directory
// when --key-dir is absent. The directory stays outside the Workspace
// and its backups: key material is owner-held and never stored in the
// database (ADR-0033, Z4).
const keyDirEnv = "ATHENA_SECRETS_DIR"

const secretsUsage = `usage: athena secrets save|rotate [--workspace slug] [--json]
  save --key name --ref locator [--provider file]
                               register a secret reference (never a value)
  rotate --key name --from path [--key-dir dir]
                               store new 32-byte key material and move the
                               key to a new active version, retiring the old
                               one; a first rotation bootstraps version 1.
                               The key directory defaults to $ATHENA_SECRETS_DIR
                               and must stay outside the Workspace`

func secretsCommand(args []string, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, secretsUsage)
		return nil
	}
	sub, rest := args[0], args[1:]
	if sub != "save" && sub != "rotate" {
		return fmt.Errorf("unknown secrets command %q; use save or rotate", sub)
	}
	flags := flag.NewFlagSet("secrets "+sub, flag.ContinueOnError)
	slug := flags.String("workspace", "", "workspace slug, when running from the home directory")
	asJSON := flags.Bool("json", false, "print JSON")
	key := flags.String("key", "", "secret reference name (save, rotate)")
	provider := flags.String("provider", "file", "provider name (save)")
	locator := flags.String("ref", "", "provider-side locator (save)")
	from := flags.String("from", "", "path holding the new 32-byte key material (rotate)")
	keyDir := flags.String("key-dir", "", "key directory (rotate, defaults to $ATHENA_SECRETS_DIR)")
	if ok, err := parse(flags, rest); !ok {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	kernel, ctx, closeKernel, err := openKernel(*slug)
	if err != nil {
		return err
	}
	defer closeKernel()
	switch sub {
	case "save":
		if *key == "" || *locator == "" {
			return fmt.Errorf("secrets save needs --key and --ref")
		}
		if *provider != "file" {
			return fmt.Errorf("secrets save: provider %q is not available yet; only file ships in beta", *provider)
		}
		ref, err := secrets.SaveRef(ctx, kernel.Users.DB, *key, *provider, *locator)
		if err != nil {
			return err
		}
		if *asJSON {
			return writeJSON(out, map[string]any{"name": ref.Name, "provider": ref.Provider, "ref": ref.ProviderRef})
		}
		fmt.Fprintf(out, "saved reference %s (%s:%s)\n", ref.Name, ref.Provider, ref.ProviderRef)
		return nil
	case "rotate":
		if *key == "" || *from == "" {
			return fmt.Errorf("secrets rotate needs --key and --from")
		}
		dir := *keyDir
		if dir == "" {
			dir = os.Getenv(keyDirEnv)
		}
		if dir == "" {
			return fmt.Errorf("secrets rotate needs --key-dir or $%s pointing outside the Workspace", keyDirEnv)
		}
		material, err := os.ReadFile(*from)
		if err != nil {
			return fmt.Errorf("secrets rotate: read %s: %w", *from, err)
		}
		tx, err := kernel.Users.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		res, err := secrets.Rotate(ctx, tx, secrets.NewFileProvider(dir), *key, material)
		if err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		if *asJSON {
			return writeJSON(out, map[string]any{"key": *key, "version": res.Version, "retired": res.Retired})
		}
		if res.Retired == 0 {
			fmt.Fprintf(out, "rotated %s to version %d (first version)\n", *key, res.Version)
		} else {
			fmt.Fprintf(out, "rotated %s to version %d (version %d retired)\n", *key, res.Version, res.Retired)
		}
		return nil
	}
	return fmt.Errorf("unknown secrets command %q; use save or rotate", sub)
}
