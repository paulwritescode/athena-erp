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
)

const rulesUsage = `usage: athena rules list|show|save|rm|versions|simulate|runs [key] [--workspace slug] [--json]
  Manage Starlark Rules with the same code as the desk. A Rule is a Starlark program the kernel
  runs at a document event (validation, derivation, guard) or when called (transform). It reads
  and sets fields only through Go host functions, under step and time limits, with no files,
  network, clock or floats. Starlark Rules are not production-ready (ADR-0016, Proposed). These
  commands act as the operator and start no server, so --addr does not apply.
  list                          list Rules with type, Record Type, event, version and state
  show <key>                    print a Rule's attributes and source ([--out file] writes the source)
  save <key> --file rule.star   create or update a Rule from a local file. A new Rule needs
                                --type, and --record-type unless it is a transform; --event picks
                                the lifecycle event (validation: validate or before_submit;
                                derivation: validate; guard: before_submit, before_cancel or
                                on_trash). On an existing Rule, flags you leave out keep their value.
                                [--label text] [--priority n] [--disable] [--enable]
                                [--expected-version n] fails when the Rule changed since you read it
  rm <key> --yes                delete a Rule; its versions and runs stay
  versions <key>                list the versions of a Rule, newest first
  simulate <key>                dry run, rolled back: [--file rule.star] runs a draft instead of the
                                saved source; [--record-id id] runs on a stored Record;
                                [--value field=text]... lays values over it. Prints the outcome and
                                a diff of the fields the Rule would set
  runs                          print the Rule Run log, newest first: [--rule key] [--record-type key]
                                [--record-id id] [--request-id id] [--outcome ok|rejected|error]
                                [--limit n]`

type valueFlags map[string]string

func (v valueFlags) set(text string) error {
	key, value, ok := strings.Cut(text, "=")
	if !ok || key == "" {
		return errors.New("--value takes field=text")
	}
	v[key] = value
	return nil
}

func rulesCommand(args []string, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, rulesUsage)
		return nil
	}
	sub, args := args[0], args[1:]
	want := map[string]int{"list": 0, "show": 1, "save": 1, "rm": 1, "versions": 1, "simulate": 1, "runs": 0}
	n, ok := want[sub]
	if !ok {
		return fmt.Errorf("unknown rules command %q; use list, show, save, rm, versions, simulate or runs", sub)
	}
	positional, args, err := leadingArgs(args, n)
	if err != nil {
		return fmt.Errorf("rules %s: %w", sub, err)
	}
	flags := flag.NewFlagSet("rules "+sub, flag.ContinueOnError)
	slug := flags.String("workspace", "", "workspace slug, when running from the home directory")
	asJSON := flags.Bool("json", false, "print JSON")
	file := flags.String("file", "", "local Starlark source (save, simulate)")
	outFile := flags.String("out", "", "write the source to this local file (show)")
	kind := flags.String("type", "", "validation, derivation, guard or transform (save)")
	recordType := flags.String("record-type", "", "Record Type key (save; runs filter)")
	event := flags.String("event", "", "lifecycle event (save)")
	label := flags.String("label", "", "label (save)")
	priority := flags.Int("priority", -1, "run order within an event, 0 to 1000 (save)")
	disable := flags.Bool("disable", false, "turn the Rule off (save)")
	enable := flags.Bool("enable", false, "turn the Rule on (save)")
	expected := flags.Int64("expected-version", -1, "fail unless the Rule is at this version (save, rm)")
	yes := flags.Bool("yes", false, "confirm the deletion (rm)")
	recordID := flags.String("record-id", "", "Record ID (simulate; runs filter)")
	rule := flags.String("rule", "", "Rule key (runs filter)")
	requestID := flags.String("request-id", "", "request ID (runs filter)")
	outcome := flags.String("outcome", "", "ok, rejected or error (runs filter)")
	limit := flags.Int("limit", 50, "most rows to print (runs)")
	values := valueFlags{}
	flags.Func("value", "field=text laid over the document (simulate), repeatable", values.set)
	if ok, err := parse(flags, args); !ok {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if sub == "rm" && !*yes {
		return errors.New("rules rm deletes the Rule; pass --yes to confirm (its versions and runs stay)")
	}
	if *disable && *enable {
		return errors.New("pass --enable or --disable, not both")
	}
	var source string
	if *file != "" {
		data, err := os.ReadFile(*file)
		if err != nil {
			return err
		}
		source = string(data)
	}
	kernel, ctx, closeKernel, err := openKernel(*slug)
	if err != nil {
		return err
	}
	defer closeKernel()
	current := func(key string) (*athenav1.Rule, error) {
		res, err := kernel.Rules.GetRule(ctx, &athenav1.GetRuleRequest{Key: key})
		if connect.CodeOf(err) == connect.CodeNotFound {
			return nil, nil
		}
		if err != nil {
			return nil, rpcError(err)
		}
		return res.GetRule(), nil
	}
	switch sub {
	case "list":
		res, err := kernel.Rules.ListRules(ctx, &athenav1.ListRulesRequest{})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		for _, r := range res.GetRules() {
			d := r.GetDefinition()
			state := "on"
			if !d.GetEnabled() {
				state = "off"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\tv%d\t%s\n", d.GetKey(), d.GetRuleType(), dash(d.GetRecordTypeKey()), dash(d.GetEvent()), r.GetVersion(), state)
		}
		return w.Flush()
	case "show":
		r, err := current(positional[0])
		if err != nil {
			return err
		}
		if r == nil {
			return fmt.Errorf("the Rule %q does not exist", positional[0])
		}
		if *outFile != "" {
			if err := os.WriteFile(*outFile, []byte(r.GetDefinition().GetSource()), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(out, "Wrote %d bytes to %s\n", len(r.GetDefinition().GetSource()), *outFile)
			return nil
		}
		if *asJSON {
			return writeProto(out, r)
		}
		d := r.GetDefinition()
		fmt.Fprintf(out, "%s (%s, %s, %s) version %d, hash %s\n\n%s", d.GetKey(), d.GetRuleType(), dash(d.GetRecordTypeKey()), dash(d.GetEvent()), r.GetVersion(), r.GetSourceHash()[:12], d.GetSource())
		return nil
	case "save":
		if source == "" {
			return errors.New("rules save needs --file with the Starlark source")
		}
		existing, err := current(positional[0])
		if err != nil {
			return err
		}
		def := &athenav1.RuleDefinition{Key: positional[0], Label: positional[0], Enabled: true, Priority: 100}
		version := int64(0)
		if existing != nil {
			def, version = existing.GetDefinition(), existing.GetVersion()
		}
		if *expected >= 0 {
			version = *expected
		}
		def.Source = source
		set := func(target *string, value string) {
			if value != "" {
				*target = value
			}
		}
		set(&def.RuleType, *kind)
		set(&def.RecordTypeKey, *recordType)
		set(&def.Event, *event)
		set(&def.Label, *label)
		if *priority >= 0 {
			def.Priority = int32(*priority)
		}
		if *enable {
			def.Enabled = true
		}
		if *disable {
			def.Enabled = false
		}
		res, err := kernel.Rules.SaveRule(ctx, &athenav1.SaveRuleRequest{Definition: def, ExpectedVersion: version})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		r := res.GetRule()
		_, err = fmt.Fprintf(out, "Saved %s (version %d, hash %s)\n", r.GetDefinition().GetKey(), r.GetVersion(), r.GetSourceHash()[:12])
		return err
	case "rm":
		r, err := current(positional[0])
		if err != nil {
			return err
		}
		if r == nil {
			return fmt.Errorf("the Rule %q does not exist", positional[0])
		}
		version := r.GetVersion()
		if *expected >= 0 {
			version = *expected
		}
		if _, err := kernel.Rules.DeleteRule(ctx, &athenav1.DeleteRuleRequest{Key: positional[0], ExpectedVersion: version}); err != nil {
			return rpcError(err)
		}
		fmt.Fprintf(out, "Deleted %s (its versions and runs stay)\n", positional[0])
		return nil
	case "versions":
		res, err := kernel.Rules.ListRuleVersions(ctx, &athenav1.ListRuleVersionsRequest{Key: positional[0]})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		for _, v := range res.GetVersions() {
			fmt.Fprintf(w, "v%d\t%s\t%s\t%s\n", v.GetVersion(), v.GetCreatedAt(), v.GetActor(), v.GetSourceHash()[:12])
		}
		return w.Flush()
	case "simulate":
		r, err := current(positional[0])
		if err != nil {
			return err
		}
		var def *athenav1.RuleDefinition
		if r != nil {
			def = r.GetDefinition()
		} else if *file == "" {
			return fmt.Errorf("the Rule %q does not exist; give --file to simulate a draft", positional[0])
		} else {
			def = &athenav1.RuleDefinition{Key: positional[0], Label: positional[0], Enabled: true, RuleType: *kind, RecordTypeKey: *recordType, Event: *event}
		}
		if source != "" {
			def.Source = source
		}
		fields := map[string]*athenav1.FieldValue{}
		for key, value := range values {
			fields[key] = &athenav1.FieldValue{Kind: &athenav1.FieldValue_StringValue{StringValue: value}}
		}
		res, err := kernel.Rules.SimulateRule(ctx, &athenav1.SimulateRuleRequest{Definition: def, RecordId: *recordID, Values: fields})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		fmt.Fprintf(out, "Outcome: %s", res.GetOutcome())
		if res.GetCode() != "" {
			fmt.Fprintf(out, " (%s) %s", res.GetCode(), res.GetMessage())
		}
		fmt.Fprintf(out, "\nSteps %d, reads %d, input %s\n", res.GetSteps(), res.GetReads(), res.GetInputHash()[:12])
		for _, c := range res.GetChanges() {
			fmt.Fprintf(out, "set %s: %s -> %s\n", c.GetField(), orEmpty(c.OldValue), orEmpty(c.NewValue))
		}
		for _, m := range res.GetWarnings() {
			fmt.Fprintf(out, "warning %s: %s\n", m.GetCode(), m.GetText())
		}
		for _, m := range res.GetLogs() {
			fmt.Fprintf(out, "log %s: %s\n", m.GetCode(), m.GetText())
		}
		return nil
	default: // runs
		res, err := kernel.Rules.ListRuleRuns(ctx, &athenav1.ListRuleRunsRequest{RuleKey: *rule, RecordTypeKey: *recordType, RecordId: *recordID, RequestId: *requestID, Outcome: *outcome, PageSize: int32(*limit)})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		for _, r := range res.GetRuns() {
			dry := ""
			if r.GetDryRun() {
				dry = "dry run"
			}
			fmt.Fprintf(w, "%s\t%s v%d\t%s\t%s\t%s\t%d steps\t%s\t%s\n", r.GetCreatedAt(), r.GetRuleKey(), r.GetRuleVersion(), r.GetEvent(), r.GetOutcome(), r.GetCode(), r.GetSteps(), r.GetActor(), dry)
		}
		return w.Flush()
	}
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
