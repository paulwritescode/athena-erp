package main

import (
	"flag"
	"fmt"
	"io"
	"strings"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/lifecycle"
	"athena-erp/internal/types"
)

const postUsage = `usage: athena post preview|submit|cancel|status|log <record-type> <id> [--workspace slug] [--json]
  preview <record-type> <id>   show the plans a submit would write now, with
                               the combined hash to approve
  submit <record-type> <id> --revision N [--preview-hash H]
                               submit the voucher; with H it refuses when the
                               plan changed since the preview
  cancel <record-type> <id> --revision N
                               cancel the voucher and mirror its posts
  status <record-type> <id>    show the voucher's posting groups and whether
                               a reversal unwound each
  log <record-type> <id> [--outcome posted|replayed|conflict|rejected|error] [--limit N]
                               list the voucher's posting attempts, newest first`

func postCommand(args []string, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, postUsage)
		return nil
	}
	sub, rest := args[0], args[1:]
	if sub != "preview" && sub != "submit" && sub != "cancel" && sub != "status" && sub != "log" {
		return fmt.Errorf("unknown post command %q; use preview, submit, cancel, status or log", sub)
	}
	positional, args, err := leadingArgs(rest, 2)
	if err != nil {
		return fmt.Errorf("post %s: %w", sub, err)
	}
	flags := flag.NewFlagSet("post "+sub, flag.ContinueOnError)
	slug := flags.String("workspace", "", "workspace slug, when running from the home directory")
	asJSON := flags.Bool("json", false, "print JSON")
	revision := flags.Int64("revision", 0, "revision edited (submit, cancel)")
	previewHash := flags.String("preview-hash", "", "approved plan hash (submit)")
	outcome := flags.String("outcome", "", "outcome filter (log)")
	limit := flags.Int("limit", 0, "entries listed (log)")
	if ok, err := parse(flags, args); !ok {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if (sub == "submit" || sub == "cancel") && *revision < 1 {
		return fmt.Errorf("post %s needs --revision N, the revision shown on the voucher", sub)
	}
	kernel, ctx, closeKernel, err := openKernel(*slug)
	if err != nil {
		return err
	}
	defer closeKernel()
	recordType, id := positional[0], positional[1]
	switch sub {
	case "preview":
		res, err := kernel.Ledger.PreviewPosting(ctx, &athenav1.PreviewPostingRequest{RecordTypeKey: recordType, Id: id})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		if len(res.GetPlans()) == 0 {
			fmt.Fprintln(out, "no posting Rules: a submit would post nothing.")
			return nil
		}
		for i, plan := range res.GetPlans() {
			fmt.Fprintf(out, "plan %d: %s v%d, book %s, %s\n", i+1, plan.GetRuleKey(), plan.GetRuleVersion(), plan.GetBook(), plan.GetPostingDate())
			for _, line := range plan.GetLines() {
				fmt.Fprintf(out, "  %s %s %s\n", line.GetCompany(), line.GetAccount(), postAmount(line))
			}
		}
		fmt.Fprintf(out, "plan hash: %s\n", res.GetPlanHash())
		return nil
	case "submit":
		res, err := kernel.Records.SubmitRecord(ctx, &athenav1.SubmitRecordRequest{RecordTypeKey: recordType, Id: id, ExpectedRevision: *revision, PostingPreviewHash: *previewHash})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res.GetRecord())
		}
		record := res.GetRecord()
		fmt.Fprintf(out, "submit: %s is now %s (revision %d).\n", record.GetName(), lifecycle.State(record.GetDocstate()), record.GetRevision())
		return nil
	case "cancel":
		res, err := kernel.Records.CancelRecord(ctx, &athenav1.CancelRecordRequest{RecordTypeKey: recordType, Id: id, ExpectedRevision: *revision})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res.GetRecord())
		}
		record := res.GetRecord()
		fmt.Fprintf(out, "cancel: %s is now %s (revision %d).\n", record.GetName(), lifecycle.State(record.GetDocstate()), record.GetRevision())
		return nil
	case "status":
		res, err := kernel.Ledger.GetPostingStatus(ctx, &athenav1.GetPostingStatusRequest{RecordTypeKey: recordType, Id: id})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		if len(res.GetGroups()) == 0 {
			fmt.Fprintln(out, "no postings yet.")
			return nil
		}
		for _, group := range res.GetGroups() {
			state := "posted " + group.GetPostedAt()
			if group.GetReversed() {
				state = "reversed"
			}
			fmt.Fprintf(out, "revision %d\t%s\t%s\t%d rows\t%s\n", group.GetRevision(), group.GetEvent(), group.GetBook(), group.GetRowCount(), state)
		}
		return nil
	default:
		res, err := kernel.Ledger.ListPostingLog(ctx, &athenav1.ListPostingLogRequest{RecordTypeKey: recordType, Id: id, Outcome: *outcome, PageSize: int32(*limit)})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		for _, entry := range res.GetEntries() {
			message := entry.GetMessage()
			if code := entry.GetCode(); code != "" && code != "ok" {
				message = code + ": " + message
			}
			fmt.Fprintf(out, "%s\trevision %d\t%s\t%s\t%s\t%s\n", entry.GetCreatedAt(), entry.GetRevision(), entry.GetEvent(), entry.GetBook(), entry.GetOutcome(), strings.TrimSpace(message))
		}
		return nil
	}
}

// postAmount renders the posting side of a previewed leg with its
// transaction amount when the leg carries one. Previewed plans are valid,
// so exactly one side of each pair sits above zero.
func postAmount(line *athenav1.PostingPlanLine) string {
	amount, side := line.GetDebitBase(), "debit"
	if credit, err := types.ParseDecimal(line.GetCreditBase()); err == nil && credit.Sign() > 0 {
		amount, side = line.GetCreditBase(), "credit"
	}
	out := side + " " + amount
	if line.GetTxnCurrency() != "" {
		txn, txnSide := line.GetTxnDebit(), "debit"
		if credit, err := types.ParseDecimal(line.GetTxnCredit()); err == nil && credit.Sign() > 0 {
			txn, txnSide = line.GetTxnCredit(), "credit"
		}
		out += fmt.Sprintf(" (%s %s %s)", txnSide, txn, line.GetTxnCurrency())
	}
	return out
}
