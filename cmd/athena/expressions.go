package main

import (
	"flag"
	"fmt"
	"io"
	"sort"

	athenav1 "athena-erp/gen/athena/v1"
)

const expressionsUsage = `usage: athena expressions check|eval|preview <record-type> [expression] [--workspace slug] [--json]
  check <record-type> <expression>
                                type-check an expression against a Record Type and print its result
                                type, the names it reads and whether a list filter can run it in the
                                database; nothing is stored
  eval <record-type> <expression> --record id
                                evaluate the expression for one saved Record
  preview <record-type>         calculate formulas and field states for --values '{"field":"text"}'
                                that are not saved; [--record id] starts from a saved Record`

func expressionsCommand(args []string, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, expressionsUsage)
		return nil
	}
	sub, args := args[0], args[1:]
	want := map[string]int{"check": 2, "eval": 2, "preview": 1}
	n, ok := want[sub]
	if !ok {
		return fmt.Errorf("unknown expressions command %q; use check, eval or preview", sub)
	}
	positional, args, err := leadingArgs(args, n)
	if err != nil {
		return fmt.Errorf("expressions %s: %w", sub, err)
	}
	flags := flag.NewFlagSet("expressions "+sub, flag.ContinueOnError)
	slug := flags.String("workspace", "", "workspace slug, when running from the home directory")
	asJSON := flags.Bool("json", false, "print JSON")
	record := flags.String("record", "", "Record ID (eval, preview)")
	values := flags.String("values", "{}", "field values as a JSON object (preview)")
	if ok, err := parse(flags, args); !ok {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if sub == "eval" && *record == "" {
		return fmt.Errorf("expressions eval needs --record, the Record to evaluate for")
	}
	var entered map[string]*athenav1.FieldValue
	if sub == "preview" {
		if entered, err = parseValues(*values); err != nil {
			return err
		}
	}
	kernel, ctx, closeKernel, err := openKernel(*slug)
	if err != nil {
		return err
	}
	defer closeKernel()
	if sub == "preview" {
		res, err := kernel.Records.PreviewRecord(ctx, &athenav1.PreviewRecordRequest{RecordTypeKey: positional[0], Id: *record, Values: entered})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		keys := make([]string, 0, len(res.GetComputed()))
		for key := range res.GetComputed() {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := res.GetComputed()[key]
			fmt.Fprintf(out, "%s = %s\n", key, orNull(value.GetStringValue(), value.GetNullValue()))
		}
		for _, state := range res.GetStates() {
			fmt.Fprintf(out, "%s: required=%t read_only=%t hidden=%t %s\n", state.GetKey(), state.GetRequired(), state.GetReadOnly(), state.GetHidden(), state.GetError())
		}
		return nil
	}
	id := ""
	if sub == "eval" {
		id = *record
	}
	res, err := kernel.Records.EvaluateExpression(ctx, &athenav1.EvaluateExpressionRequest{RecordTypeKey: positional[0], Expression: positional[1], Id: id})
	if err != nil {
		return rpcError(err)
	}
	if *asJSON {
		return writeProto(out, res)
	}
	if res.GetEvaluated() {
		fmt.Fprintf(out, "%s\n", orNull(res.GetValue(), res.GetIsNull()))
		return nil
	}
	fmt.Fprintf(out, "type: %s\nreads: %v\n", res.GetResultType(), res.GetDependencies())
	if res.GetSqlTranslatable() {
		fmt.Fprintln(out, "filter: runs in the database")
	} else {
		fmt.Fprintf(out, "filter: not available (%s)\n", res.GetSqlReason())
	}
	return nil
}

func orNull(value string, null bool) string {
	if null {
		return "null"
	}
	return value
}
