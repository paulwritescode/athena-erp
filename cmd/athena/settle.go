package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	athenav1 "athena-erp/gen/athena/v1"
)

const settleUsage = `usage: athena settle apply|reverse|open-items [--workspace slug] [--json]
  apply --company C --book B --party-type T --party-id ID --control-account A
        --side debit|credit --voucher-type VT --voucher-id VID --revision N
        --date YYYY-MM-DD --currency CCC --lines '<json>'
        [--fx-rate-type T --fx-gain G --fx-loss L] [--close-reason R]
                               apply the voucher's lines to open items; --lines
                               is JSON: [{"line_no":1,"amount":"100.00",
                               "items":[{"item_id":"..."},{"item_id":"...",
                               "amount":"60.00"}]}] (an empty item amount takes
                               an automatic share). FX needs --fx-rate-type and
                               the gain/loss accounts. --close-reason posts
                               into a soft-closed period as the operator.
  reverse --company C --book B --party-type T --party-id ID --control-account A
        --side debit|credit --voucher-type VT --voucher-id VID --revision N
        --date YYYY-MM-DD
                               unwind the voucher in the current period
  open-items --company C --book B --party-type T --party-id ID
                               list the party's open items in stable id order`

type settleLineItemInput struct {
	ItemID string `json:"item_id"`
	Amount string `json:"amount"`
}

type settleLineInput struct {
	LineNo int32                 `json:"line_no"`
	Amount string                `json:"amount"`
	Items  []settleLineItemInput `json:"items"`
}

func settleCommand(args []string, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, settleUsage)
		return nil
	}
	sub, args := args[0], args[1:]
	if sub != "apply" && sub != "reverse" && sub != "open-items" {
		return fmt.Errorf("unknown settle command %q; use apply, reverse or open-items", sub)
	}
	flags := flag.NewFlagSet("settle "+sub, flag.ContinueOnError)
	slug := flags.String("workspace", "", "workspace slug, when running from the home directory")
	asJSON := flags.Bool("json", false, "print JSON")
	company := flags.String("company", "", "company key")
	book := flags.String("book", "", "book name")
	partyType := flags.String("party-type", "", "party Record Type key")
	partyID := flags.String("party-id", "", "party Record id")
	control := flags.String("control-account", "", "control account name")
	side := flags.String("side", "", "debit (receivable) or credit (payable)")
	voucherType := flags.String("voucher-type", "", "settlement voucher Record Type key")
	voucherID := flags.String("voucher-id", "", "settlement voucher id")
	revision := flags.Int64("revision", 0, "voucher revision (apply, reverse)")
	date := flags.String("date", "", "posting date as YYYY-MM-DD (apply, reverse)")
	currency := flags.String("currency", "", "payment currency code (apply)")
	linesRaw := flags.String("lines", "", "settlement lines as JSON (apply)")
	fxRateType := flags.String("fx-rate-type", "", "exchange-rate type (apply with foreign items)")
	fxGain := flags.String("fx-gain", "", "realised-gain account (apply with foreign items)")
	fxLoss := flags.String("fx-loss", "", "realised-loss account (apply with foreign items)")
	closeReason := flags.String("close-reason", "", "reason for a soft-closed adjustment (apply)")
	if ok, err := parse(flags, args); !ok {
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
	case "open-items":
		res, err := kernel.Ledger.ListOpenItems(ctx, &athenav1.ListOpenItemsRequest{
			Company: *company, Book: *book, PartyType: *partyType, PartyId: *partyID,
		})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		if len(res.GetItems()) == 0 {
			fmt.Fprintln(out, "no open items.")
			return nil
		}
		for _, item := range res.GetItems() {
			fmt.Fprintf(out, "%s\t%s\t%s\t%s\tresidual %s\n", item.GetId(), item.GetCurrency(), item.GetSide(), item.GetStatus(), item.GetResidualAmount())
		}
		return nil
	case "reverse":
		res, err := kernel.Ledger.ReverseSettlement(ctx, &athenav1.ReverseSettlementRequest{
			Company: *company, Book: *book, PartyType: *partyType, PartyId: *partyID,
			ControlAccount: *control, Side: *side, VoucherType: *voucherType, VoucherId: *voucherID,
			Revision: *revision, PostingDate: *date,
		})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		fmt.Fprintf(out, "reversed %d application(s).\n", len(res.GetApplications()))
		for _, group := range res.GetFxCancelGroups() {
			fmt.Fprintf(out, "unwound FX group %s.\n", group)
		}
		return nil
	default:
		var raw []settleLineInput
		if err := json.Unmarshal([]byte(*linesRaw), &raw); err != nil {
			return fmt.Errorf("settle apply needs --lines as JSON: %v", err)
		}
		lines := make([]*athenav1.SettlementLine, 0, len(raw))
		for _, line := range raw {
			items := make([]*athenav1.SettlementLineItem, 0, len(line.Items))
			for _, item := range line.Items {
				items = append(items, &athenav1.SettlementLineItem{ItemId: item.ItemID, Amount: item.Amount})
			}
			lines = append(lines, &athenav1.SettlementLine{LineNo: line.LineNo, Amount: line.Amount, Items: items})
		}
		var fx *athenav1.SettlementFX
		if *fxRateType != "" || *fxGain != "" || *fxLoss != "" {
			fx = &athenav1.SettlementFX{RateType: *fxRateType, GainAccount: *fxGain, LossAccount: *fxLoss}
		}
		res, err := kernel.Ledger.ApplySettlement(ctx, &athenav1.ApplySettlementRequest{
			Company: *company, Book: *book, PartyType: *partyType, PartyId: *partyID,
			ControlAccount: *control, Side: *side, VoucherType: *voucherType, VoucherId: *voucherID,
			Revision: *revision, PostingDate: *date, PaymentCurrency: *currency,
			Fx: fx, CloseReason: *closeReason, Lines: lines,
		})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		for _, application := range res.GetApplications() {
			fmt.Fprintf(out, "line %d\titem %s\t%s %s (base %s)\n", application.GetLineNo(), application.GetItemId(),
				application.GetItemAmount(), *currency, application.GetBaseAmount())
		}
		for _, unapplied := range res.GetUnapplied() {
			fmt.Fprintf(out, "line %d\tunapplied\t%s %s\n", unapplied.GetLineNo(), unapplied.GetAmount(), *currency)
		}
		if res.GetFxGroupId() != "" {
			fmt.Fprintf(out, "realised FX group %s.\n", res.GetFxGroupId())
		}
		return nil
	}
}
