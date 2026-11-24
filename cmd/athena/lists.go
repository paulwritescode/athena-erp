package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	athenav1 "athena-erp/gen/athena/v1"
)

const listsUsage = `usage: athena lists export <id> --pdf [--out path] [--max-rows N] [--workspace slug]
  export <id> --pdf          render the saved list to PDF bytes (header with
                             Workspace name, logo, title, filter and time;
                             --out writes a file, default prints the filename
                             and writes it beside it)
                             [--max-rows N] caps the rendered rows (1-500,
                             default 200)`

func listsCommand(args []string, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, listsUsage)
		return nil
	}
	if len(args) == 0 || args[0] != "export" {
		got := ""
		if len(args) > 0 {
			got = args[0]
		}
		return fmt.Errorf("unknown lists command %q; use export", got)
	}
	positional, rest, err := leadingArgs(args[1:], 1)
	if err != nil {
		return fmt.Errorf("lists export: %w", err)
	}
	flags := flag.NewFlagSet("lists export", flag.ContinueOnError)
	slug := flags.String("workspace", "", "workspace slug, when running from the home directory")
	pdf := flags.Bool("pdf", false, "render PDF bytes (the only format)")
	outFile := flags.String("out", "", "write the PDF here instead of <list>.pdf")
	maxRows := flags.Int("max-rows", 0, "rows rendered, 1-500")
	if ok, err := parse(flags, rest); !ok {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if !*pdf {
		return fmt.Errorf("lists export needs --pdf")
	}
	kernel, ctx, closeKernel, err := openKernel(*slug)
	if err != nil {
		return err
	}
	defer closeKernel()
	res, err := kernel.Lists.ExportListPdf(ctx, &athenav1.ExportListPdfRequest{Id: positional[0], MaxRows: int32(*maxRows)})
	if err != nil {
		return rpcError(err)
	}
	target := *outFile
	if target == "" {
		target = res.GetFilename()
	}
	if err := os.WriteFile(target, res.GetPdf(), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "Wrote %d bytes to %s\n", len(res.GetPdf()), target)
	return nil
}
