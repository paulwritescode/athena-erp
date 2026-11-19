package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/lifecycle"
	"athena-erp/internal/server"

	"connectrpc.com/connect/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const recordTypesUsage = `usage: athena record-types list|show|save|delete [--workspace slug] [--json]
  list                          list saved Record Types
  show <key>                    print one Record Type with its fields and attributes
  save --file <path>            create or update a Record Type from a JSON file in the
                                RecordType message format; [--dry-run] prints the plan only,
                                [--confirm-data-loss] allows steps that delete stored values
  delete <key> --yes            delete a Record Type, its table and its Records`

const recordsUsage = `usage: athena records list|create|delete|submit|cancel|amend|versions|activity <record-type> [--workspace slug] [--json]
  list <record-type>            list Records; child Record Types take [--parent id]
  create <record-type>          create a Record from --values '{"field":"value"}' (text, true, false or null);
                                child Record Types also take --parent id
  delete <record-type> <id> --revision N
                                soft-delete a master Record, or remove one of another kind
  submit|cancel|amend <record-type> <id> --revision N
                                move a submittable document from draft to submitted, from submitted
                                to cancelled, or create a draft successor of a cancelled amendable one
  versions <record-type> <id>   list the document's snapshots (submit, cancel, amend)
  activity <record-type> <id>   list the document's timeline`

// openKernel resolves the Workspace like the migrate command and opens the
// kernel services the ConnectRPC handlers use.
func openKernel(slug string) (*server.Kernel, context.Context, context.CancelFunc, error) {
	cfg, workspace, err := migrationTarget(slug)
	if err != nil {
		return nil, nil, nil, err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	release := func() { cancel(); stop() }
	kernel, err := server.OpenKernel(ctx, cfg, workspace)
	if err != nil {
		release()
		return nil, nil, nil, err
	}
	return kernel, ctx, func() { _ = kernel.Close(); release() }, nil
}

// leadingArgs splits n positional arguments off the front, so flags may follow them.
func leadingArgs(args []string, n int) ([]string, []string, error) {
	var positional []string
	for len(positional) < n && len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positional, args = append(positional, args[0]), args[1:]
	}
	if len(positional) < n {
		return nil, nil, fmt.Errorf("expected %d argument(s) before the flags", n)
	}
	return positional, args, nil
}

func isHelp(args []string) bool {
	return len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h"
}

func recordTypesCommand(args []string, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, recordTypesUsage)
		return nil
	}
	sub, args := args[0], args[1:]
	var key string
	if sub == "show" || sub == "delete" {
		positional, rest, err := leadingArgs(args, 1)
		if err != nil {
			return fmt.Errorf("record-types %s: %w", sub, err)
		}
		key, args = positional[0], rest
	}
	flags := flag.NewFlagSet("record-types "+sub, flag.ContinueOnError)
	slug := flags.String("workspace", "", "workspace slug, when running from the home directory")
	asJSON := flags.Bool("json", false, "print JSON")
	file := flags.String("file", "", "Record Type JSON file (save)")
	dryRun := flags.Bool("dry-run", false, "print the save plan without saving (save)")
	confirm := flags.Bool("confirm-data-loss", false, "allow steps that delete stored values (save)")
	yes := flags.Bool("yes", false, "confirm the deletion (delete)")
	if sub != "list" && sub != "show" && sub != "save" && sub != "delete" {
		return fmt.Errorf("unknown record-types command %q; use list, show, save or delete", sub)
	}
	if ok, err := parse(flags, args); !ok {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	// Check the cheap inputs before touching the database.
	var definition *athenav1.RecordType
	if sub == "save" {
		if *file == "" {
			return errors.New("record-types save needs --file")
		}
		var err error
		if definition, err = readRecordType(*file); err != nil {
			return err
		}
	}
	if sub == "delete" && !*yes {
		return errors.New("record-types delete removes the table and every Record in it; pass --yes to confirm")
	}
	kernel, ctx, closeKernel, err := openKernel(*slug)
	if err != nil {
		return err
	}
	defer closeKernel()
	listed, err := kernel.RecordTypes.ListRecordTypes(ctx, &athenav1.ListRecordTypesRequest{})
	if err != nil {
		return rpcError(err)
	}
	find := func(key string) *athenav1.RecordType {
		for _, item := range listed.GetRecordTypes() {
			if item.GetKey() == key {
				return item
			}
		}
		return nil
	}
	switch sub {
	case "list":
		if *asJSON {
			return writeProto(out, listed)
		}
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		for _, item := range listed.GetRecordTypes() {
			fmt.Fprintf(w, "%s\t%s\trevision %d\t%d fields\t%s\n", item.GetKey(), item.GetKind(), item.GetRevision(), len(item.GetFields()), item.GetLabel())
		}
		return w.Flush()
	case "show":
		item := find(key)
		if item == nil {
			return fmt.Errorf("Record Type %q does not exist", key)
		}
		return writeProto(out, item)
	case "delete":
		item := find(key)
		if item == nil {
			return fmt.Errorf("Record Type %q does not exist", key)
		}
		if _, err := kernel.RecordTypes.DeleteRecordType(ctx, &athenav1.DeleteRecordTypeRequest{Key: key, ExpectedRevision: item.GetRevision(), DeleteRecords: true}); err != nil {
			return rpcError(err)
		}
		fmt.Fprintf(out, "Deleted %s.\n", key)
		return nil
	}
	// save: an existing key is an update at its current revision.
	expected := int64(0)
	if existing := find(definition.GetKey()); existing != nil {
		expected = existing.GetRevision()
	}
	plan, err := kernel.RecordTypes.PlanRecordTypeSave(ctx, &athenav1.PlanRecordTypeSaveRequest{RecordType: definition, ExpectedRevision: expected})
	if err != nil {
		return rpcError(err)
	}
	if *asJSON {
		if *dryRun {
			return writeProto(out, plan)
		}
	} else {
		for _, step := range plan.GetSteps() {
			fmt.Fprintf(out, "- %s\n", step.GetDescription())
		}
	}
	if *dryRun {
		return nil
	}
	if plan.GetRequiresConfirmation() && !*confirm {
		return errors.New("this save deletes stored values or needs fill-in values; review the plan above and pass --confirm-data-loss (fill-in values are entered in the desk)")
	}
	saved, err := kernel.RecordTypes.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: definition, ExpectedRevision: expected, ConfirmDataLoss: *confirm})
	if err != nil {
		return rpcError(err)
	}
	if *asJSON {
		return writeProto(out, saved.GetRecordType())
	}
	fmt.Fprintf(out, "Saved %s at revision %d.\n", saved.GetRecordType().GetKey(), saved.GetRecordType().GetRevision())
	return nil
}

func recordsCommand(args []string, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, recordsUsage)
		return nil
	}
	sub, args := args[0], args[1:]
	want := map[string]int{"list": 1, "create": 1, "delete": 2, "submit": 2, "cancel": 2, "amend": 2, "versions": 2, "activity": 2}
	n, ok := want[sub]
	if !ok {
		return fmt.Errorf("unknown records command %q; use list, create, delete, submit, cancel, amend, versions or activity", sub)
	}
	positional, args, err := leadingArgs(args, n)
	if err != nil {
		return fmt.Errorf("records %s: %w", sub, err)
	}
	flags := flag.NewFlagSet("records "+sub, flag.ContinueOnError)
	slug := flags.String("workspace", "", "workspace slug, when running from the home directory")
	asJSON := flags.Bool("json", false, "print JSON")
	parent := flags.String("parent", "", "parent Record ID (child Record Types)")
	values := flags.String("values", "", "field values as a JSON object (create)")
	revision := flags.Int64("revision", 0, "revision of the Record you last saw (delete, submit, cancel, amend)")
	if ok, err := parse(flags, args); !ok {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	var fieldValues map[string]*athenav1.FieldValue
	if sub == "create" {
		if fieldValues, err = parseValues(*values); err != nil {
			return err
		}
	}
	if (sub == "delete" || sub == "submit" || sub == "cancel" || sub == "amend") && *revision <= 0 {
		return fmt.Errorf("records %s needs --revision, the revision you last saw", sub)
	}
	kernel, ctx, closeKernel, err := openKernel(*slug)
	if err != nil {
		return err
	}
	defer closeKernel()
	switch sub {
	case "list":
		res, err := kernel.Records.ListRecords(ctx, &athenav1.ListRecordsRequest{RecordTypeKey: positional[0], ParentId: *parent, PageSize: 200})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		for _, record := range res.GetRecords() {
			fmt.Fprintf(out, "%s\trevision %d\n", record.GetId(), record.GetRevision())
		}
		if res.GetNextPageToken() != "" {
			fmt.Fprintln(out, "More Records exist; use --json and a narrower view, or the desk.")
		}
		return nil
	case "create":
		res, err := kernel.Records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: positional[0], Values: fieldValues, ParentId: *parent})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res.GetRecord())
		}
		fmt.Fprintf(out, "Created %s.\n", res.GetRecord().GetId())
		return nil
	case "submit", "cancel", "amend":
		var record *athenav1.Record
		var err error
		switch sub {
		case "submit":
			var res *athenav1.SubmitRecordResponse
			res, err = kernel.Records.SubmitRecord(ctx, &athenav1.SubmitRecordRequest{RecordTypeKey: positional[0], Id: positional[1], ExpectedRevision: *revision})
			record = res.GetRecord()
		case "cancel":
			var res *athenav1.CancelRecordResponse
			res, err = kernel.Records.CancelRecord(ctx, &athenav1.CancelRecordRequest{RecordTypeKey: positional[0], Id: positional[1], ExpectedRevision: *revision})
			record = res.GetRecord()
		default:
			var res *athenav1.AmendRecordResponse
			res, err = kernel.Records.AmendRecord(ctx, &athenav1.AmendRecordRequest{RecordTypeKey: positional[0], Id: positional[1], ExpectedRevision: *revision})
			record = res.GetRecord()
		}
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, record)
		}
		fmt.Fprintf(out, "%s: %s is now %s (revision %d).\n", sub, record.GetName(), lifecycle.State(record.GetDocstate()), record.GetRevision())
		return nil
	case "versions":
		res, err := kernel.Records.ListRecordVersions(ctx, &athenav1.ListRecordVersionsRequest{RecordTypeKey: positional[0], Id: positional[1]})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		for _, item := range res.GetVersions() {
			fmt.Fprintf(out, "version %d\t%s\t%s\t%s\n", item.GetVersion(), item.GetEvent(), lifecycle.State(item.GetDocstate()), item.GetAt())
		}
		return nil
	case "activity":
		res, err := kernel.Records.ListRecordActivity(ctx, &athenav1.ListRecordActivityRequest{RecordTypeKey: positional[0], Id: positional[1]})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		for _, item := range res.GetEvents() {
			fmt.Fprintf(out, "%s\t%s\t%s\n", item.GetAt(), item.GetEvent(), item.GetActor())
		}
		return nil
	}
	if _, err := kernel.Records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: positional[0], Id: positional[1], ExpectedRevision: *revision}); err != nil {
		return rpcError(err)
	}
	fmt.Fprintf(out, "Deleted %s.\n", positional[1])
	return nil
}

func readRecordType(path string) (*athenav1.RecordType, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var recordType athenav1.RecordType
	if err := protojson.Unmarshal(payload, &recordType); err != nil {
		return nil, fmt.Errorf("read %s as a Record Type: %w", path, err)
	}
	// The server owns the revision; a stale one in the file must not matter.
	recordType.Revision = 0
	return &recordType, nil
}

// parseValues reads a JSON object whose values are strings, booleans or null.
func parseValues(text string) (map[string]*athenav1.FieldValue, error) {
	if text == "" {
		return nil, errors.New("records create needs --values, for example --values '{\"name\":\"Acme\"}'")
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, fmt.Errorf("--values is not a JSON object: %w", err)
	}
	values := make(map[string]*athenav1.FieldValue, len(raw))
	for key, value := range raw {
		switch v := value.(type) {
		case string:
			values[key] = &athenav1.FieldValue{Kind: &athenav1.FieldValue_StringValue{StringValue: v}}
		case bool:
			values[key] = &athenav1.FieldValue{Kind: &athenav1.FieldValue_BoolValue{BoolValue: v}}
		case nil:
			values[key] = &athenav1.FieldValue{Kind: &athenav1.FieldValue_NullValue{NullValue: true}}
		default:
			return nil, fmt.Errorf("--values: %q must be text, true, false or null; write numbers as text", key)
		}
	}
	return values, nil
}

func writeProto(out io.Writer, message proto.Message) error {
	payload, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(message)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(payload))
	return err
}

// rpcError turns a kernel error into the message the desk would show.
func rpcError(err error) error {
	var ce *connect.Error
	if errors.As(err, &ce) {
		return errors.New(ce.Message())
	}
	return err
}
