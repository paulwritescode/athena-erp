package main

import (
	"flag"
	"fmt"
	"io"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/server"
)

const eventsUsage = `usage: athena events list|show|outbox|requeue|subscriptions [--workspace slug] [--json]
  list [--type type] [--subject-type type] [--subject id] [--limit N] [--page token]
                               list committed events, newest first
  show <id>                      show one event envelope with its payload
  outbox [--status pending|leased|dispatched|dead] [--limit N] [--page token]
                               list outbox delivery rows, newest first
  requeue <outbox-id>            emit a dead delivery again as a fresh event;
                               the dead row stays as history
  subscriptions [--type type]    list the declared consumer subscriptions`

func eventsCommand(args []string, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, eventsUsage)
		return nil
	}
	sub, rest := args[0], args[1:]
	if sub != "list" && sub != "show" && sub != "outbox" && sub != "requeue" && sub != "subscriptions" {
		return fmt.Errorf("unknown events command %q; use list, show, outbox, requeue or subscriptions", sub)
	}
	var positional []string
	if sub == "show" || sub == "requeue" {
		var err error
		if positional, rest, err = leadingArgs(rest, 1); err != nil {
			return fmt.Errorf("events %s: %w", sub, err)
		}
	}
	flags := flag.NewFlagSet("events "+sub, flag.ContinueOnError)
	slug := flags.String("workspace", "", "workspace slug, when running from the home directory")
	asJSON := flags.Bool("json", false, "print JSON")
	eventType := flags.String("type", "", "event type filter (list, subscriptions)")
	subjectType := flags.String("subject-type", "", "subject type filter (list)")
	subject := flags.String("subject", "", "subject id filter (list)")
	status := flags.String("status", "", "status filter (outbox)")
	limit := flags.Int("limit", 0, "rows listed")
	page := flags.String("page", "", "next page token from the previous page (list, outbox)")
	if ok, err := parse(flags, rest); !ok {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if *limit < 0 || *limit > 500 {
		return fmt.Errorf("events %s needs --limit between 0 and 500", sub)
	}
	kernel, ctx, closeKernel, err := openKernel(*slug)
	if err != nil {
		return err
	}
	defer closeKernel()
	svc := server.NewEventService(kernel.Users.DB)
	switch sub {
	case "list":
		res, err := svc.ListEvents(ctx, &athenav1.ListEventsRequest{EventType: *eventType, SubjectType: *subjectType, SubjectId: *subject, PageSize: int32(*limit), PageToken: *page})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		for _, event := range res.GetEvents() {
			fmt.Fprintf(out, "%d\t%s\t%s\t%s %s\t%s\n", event.GetSequence(), event.GetId(),
				event.GetEventType(), event.GetSubjectType(), event.GetSubjectId(), event.GetCreatedAt())
		}
		if res.GetNextPageToken() != "" {
			fmt.Fprintf(out, "next page token: %s\n", res.GetNextPageToken())
		}
		return nil
	case "show":
		res, err := svc.GetEvent(ctx, &athenav1.GetEventRequest{Id: positional[0]})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		event := res.GetEvent()
		fmt.Fprintf(out, "id: %s\nsequence: %d\ntype: %s\nschema: %d\n", event.GetId(), event.GetSequence(), event.GetEventType(), event.GetSchemaVersion())
		fmt.Fprintf(out, "subject: %s %s revision %d\n", event.GetSubjectType(), event.GetSubjectId(), event.GetSubjectRevision())
		if event.GetActor() != "" {
			fmt.Fprintf(out, "actor: %s\n", event.GetActor())
		}
		if event.GetOperationId() != "" {
			fmt.Fprintf(out, "operation: %s\n", event.GetOperationId())
		}
		fmt.Fprintf(out, "correlation: %s\n", event.GetCorrelationId())
		if event.GetCausationId() != "" {
			fmt.Fprintf(out, "caused by: %s\n", event.GetCausationId())
		}
		fmt.Fprintf(out, "payload: %s\ncreated: %s\n", event.GetPayload(), event.GetCreatedAt())
		return nil
	case "outbox":
		res, err := svc.ListOutbox(ctx, &athenav1.ListOutboxRequest{Status: *status, PageSize: int32(*limit), PageToken: *page})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		for _, row := range res.GetRows() {
			line := fmt.Sprintf("%s\t%s\t%d\t%s\t%s", row.GetId(), row.GetStatus(), row.GetAttempts(),
				row.GetEventType(), row.GetEventId())
			if row.GetLastError() != "" {
				line += "\t" + row.GetLastError()
			}
			fmt.Fprintln(out, line)
		}
		if res.GetNextPageToken() != "" {
			fmt.Fprintf(out, "next page token: %s\n", res.GetNextPageToken())
		}
		return nil
	case "requeue":
		res, err := svc.RequeueOutbox(ctx, &athenav1.RequeueOutboxRequest{Id: positional[0]})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		fmt.Fprintf(out, "requeued %s as event %s.\n", positional[0], res.GetEventId())
		return nil
	default:
		res, err := svc.ListSubscriptions(ctx, &athenav1.ListSubscriptionsRequest{EventType: *eventType})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		for _, sub := range res.GetSubscriptions() {
			fmt.Fprintf(out, "%s\t%s\t%s\t%s\tenabled=%v\t%s\n", sub.GetConsumer(), sub.GetEventType(),
				sub.GetMode(), sub.GetAction(), sub.GetEnabled(), sub.GetPack())
		}
		return nil
	}
}
