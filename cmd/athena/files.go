package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	athenav1 "athena-erp/gen/athena/v1"

	"connectrpc.com/connect/v2"
	"google.golang.org/protobuf/proto"
)

const filesUsage = `usage: athena files list|get|put|mv|rm|history [path] [--workspace slug] [--json]
  Manage the files of a Workspace (record_types/, rules/, packs/, generated/) with the same code as
  the desk. A path is relative to the Workspace directory, lowercase, at most 1 MiB, UTF-8 text.
  Every change is kept as a file version and written to the Audit Log (path, hash and actor, never
  content). These commands need a Workspace (--workspace, or run inside one); they start no server,
  so --addr does not apply.
  list                          list files with their size, revision, hash and what each backs (a
                                Record Type, Rule or Report), and Record Types with no file
  get <path>                    print the file; [--revision N] prints that recorded revision,
                                [--out file] writes it to a local file instead
  put <path>                    create or replace a file from [--file local] or standard input;
                                [--expected-hash h] fails when the file changed since you read it,
                                [--new] fails when the file already exists. Without either flag the
                                current file is replaced
  mv <from> <to>                rename a file; [--expected-hash h] guards against a changed file
  rm <path> --yes               delete a file from disk; its versions stay;
                                [--expected-hash h] guards against a changed file
  history <path>                list the file's versions, newest first, across renames`

func filesCommand(args []string, in io.Reader, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, filesUsage)
		return nil
	}
	sub, args := args[0], args[1:]
	want := map[string]int{"list": 0, "get": 1, "put": 1, "mv": 2, "rm": 1, "history": 1}
	n, ok := want[sub]
	if !ok {
		return fmt.Errorf("unknown files command %q; use list, get, put, mv, rm or history", sub)
	}
	positional, args, err := leadingArgs(args, n)
	if err != nil {
		return fmt.Errorf("files %s: %w", sub, err)
	}
	flags := flag.NewFlagSet("files "+sub, flag.ContinueOnError)
	slug := flags.String("workspace", "", "workspace slug, when running from the home directory")
	asJSON := flags.Bool("json", false, "print JSON")
	revision := flags.Int64("revision", 0, "read this recorded revision (get)")
	outFile := flags.String("out", "", "write the file to this local path (get)")
	local := flags.String("file", "", "read the content from this local file; default standard input (put)")
	expected := flags.String("expected-hash", "", "fail unless the file on disk has this SHA-256 (put, mv, rm)")
	isNew := flags.Bool("new", false, "fail when the file already exists (put)")
	yes := flags.Bool("yes", false, "confirm the deletion (rm)")
	if ok, err := parse(flags, args); !ok {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if sub == "rm" && !*yes {
		return errors.New("files rm deletes the file from disk; pass --yes to confirm (its versions stay)")
	}
	if *isNew && *expected != "" {
		return errors.New("pass --new or --expected-hash, not both")
	}
	var content []byte
	if sub == "put" {
		if *local != "" {
			content, err = os.ReadFile(*local)
		} else {
			content, err = io.ReadAll(io.LimitReader(in, 2<<20))
		}
		if err != nil {
			return err
		}
	}
	kernel, ctx, closeKernel, err := openKernel(*slug)
	if err != nil {
		return err
	}
	defer closeKernel()

	// currentHash is the guard for put, mv and rm when the person gave none:
	// the hash of the file as it is now, so the operator replaces what is there.
	currentHash := func(path string) (string, error) {
		if *expected != "" {
			return *expected, nil
		}
		res, err := kernel.Files.GetFile(ctx, &athenav1.GetFileRequest{Path: path})
		if connect.CodeOf(err) == connect.CodeNotFound {
			return "", nil
		}
		if err != nil {
			return "", rpcError(err)
		}
		return res.GetInfo().GetHash(), nil
	}

	switch sub {
	case "list":
		res, err := kernel.Files.ListFiles(ctx, &athenav1.ListFilesRequest{})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		for _, f := range res.GetFiles() {
			backs := "-"
			if b := f.GetBacking(); b != nil {
				backs = b.GetKind() + " " + b.GetKey()
				if b.GetSaved() {
					backs += " (saved)"
				}
			}
			state := ""
			if !f.GetRecorded() {
				state = "edited outside Athena"
			}
			fmt.Fprintf(w, "%s\t%d\tr%d\t%s\t%s\t%s\n", f.GetPath(), f.GetSize(), f.GetRevision(), f.GetHash()[:12], backs, state)
		}
		if err := w.Flush(); err != nil {
			return err
		}
		if missing := res.GetRecordTypesWithoutFile(); len(missing) > 0 {
			fmt.Fprintf(out, "Record Types with no file: %s\n", strings.Join(missing, ", "))
		}
		return nil
	case "get":
		res, err := kernel.Files.GetFile(ctx, &athenav1.GetFileRequest{Path: positional[0], Revision: *revision})
		if err != nil {
			return rpcError(err)
		}
		switch {
		case *outFile != "":
			if err := os.WriteFile(*outFile, res.GetContent(), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(out, "Wrote %d bytes to %s\n", len(res.GetContent()), *outFile)
		case *asJSON:
			return writeProto(out, res)
		default:
			_, err = out.Write(res.GetContent())
			return err
		}
		return nil
	case "put":
		hash := *expected
		if !*isNew {
			if hash, err = currentHash(positional[0]); err != nil {
				return err
			}
		}
		res, err := kernel.Files.PutFile(ctx, &athenav1.PutFileRequest{Path: positional[0], Content: content, ExpectedHash: hash})
		if err != nil {
			return rpcError(err)
		}
		return printFileInfo(out, *asJSON, res, "Saved", res.GetInfo())
	case "mv":
		hash, err := currentHash(positional[0])
		if err != nil {
			return err
		}
		res, err := kernel.Files.MoveFile(ctx, &athenav1.MoveFileRequest{From: positional[0], To: positional[1], ExpectedHash: hash})
		if err != nil {
			return rpcError(err)
		}
		return printFileInfo(out, *asJSON, res, "Moved to", res.GetInfo())
	case "rm":
		hash, err := currentHash(positional[0])
		if err != nil {
			return err
		}
		res, err := kernel.Files.DeleteFile(ctx, &athenav1.DeleteFileRequest{Path: positional[0], ExpectedHash: hash})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		fmt.Fprintf(out, "Deleted %s (its versions stay)\n", positional[0])
		return nil
	default: // history
		res, err := kernel.Files.ListFileVersions(ctx, &athenav1.ListFileVersionsRequest{Path: positional[0]})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		for _, v := range res.GetVersions() {
			fmt.Fprintf(w, "r%d\t%s\t%s\t%s\t%s\t%s\n", v.GetRevision(), v.GetAction(), v.GetCreatedAt(), v.GetActor(), v.GetHash()[:12], v.GetPath())
		}
		return w.Flush()
	}
}

func printFileInfo(out io.Writer, asJSON bool, message proto.Message, verb string, info *athenav1.FileInfo) error {
	if asJSON {
		return writeProto(out, message)
	}
	_, err := fmt.Fprintf(out, "%s %s (revision %d, %d bytes, hash %s)\n", verb, info.GetPath(), info.GetRevision(), info.GetSize(), info.GetHash()[:12])
	return err
}
