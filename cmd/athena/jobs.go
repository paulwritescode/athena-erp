package main

import (
	"flag"
	"fmt"
	"io"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/server"
)

const jobsUsage = `usage: athena jobs list|show|retry|occurrences [--workspace slug] [--json]
  list [--status queued|leased|succeeded|dead] [--kind kind] [--limit N] [--page token]
                               list jobs, newest first
  show <id>                      show one job with its payload and result
  retry <id>                     run dead work again as a new job; the dead
                               row stays as history
  occurrences --schedule name [--scope scope] [--status pending|leased|done|skipped|dead] [--limit N]
                               list schedule occurrences in period order`

func jobsCommand(args []string, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, jobsUsage)
		return nil
	}
	sub, rest := args[0], args[1:]
	if sub != "list" && sub != "show" && sub != "retry" && sub != "occurrences" {
		return fmt.Errorf("unknown jobs command %q; use list, show, retry or occurrences", sub)
	}
	var positional []string
	need := 0
	if sub == "show" || sub == "retry" {
		need = 1
	}
	if need > 0 {
		var err error
		if positional, rest, err = leadingArgs(rest, need); err != nil {
			return fmt.Errorf("jobs %s: %w", sub, err)
		}
	}
	flags := flag.NewFlagSet("jobs "+sub, flag.ContinueOnError)
	slug := flags.String("workspace", "", "workspace slug, when running from the home directory")
	asJSON := flags.Bool("json", false, "print JSON")
	status := flags.String("status", "", "status filter")
	kind := flags.String("kind", "", "job kind filter (list)")
	limit := flags.Int("limit", 0, "rows listed")
	page := flags.String("page", "", "next page token from the previous page (list)")
	schedule := flags.String("schedule", "", "schedule filter (occurrences)")
	scope := flags.String("scope", "", "scope filter (occurrences)")
	if ok, err := parse(flags, rest); !ok {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if *limit < 0 || *limit > 500 {
		return fmt.Errorf("jobs %s needs --limit between 0 and 500", sub)
	}
	kernel, ctx, closeKernel, err := openKernel(*slug)
	if err != nil {
		return err
	}
	defer closeKernel()
	svc := server.NewJobService(kernel.Users.DB)
	switch sub {
	case "list":
		res, err := svc.ListJobs(ctx, &athenav1.ListJobsRequest{Status: *status, Kind: *kind, PageSize: int32(*limit), PageToken: *page})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		for _, job := range res.GetJobs() {
			fmt.Fprintf(out, "%s\t%s\t%s\t%d/%d\t%s\n", job.GetId(), job.GetKind(), job.GetStatus(),
				job.GetAttempts(), job.GetMaxAttempts(), job.GetUpdatedAt())
		}
		if res.GetNextPageToken() != "" {
			fmt.Fprintf(out, "next page token: %s\n", res.GetNextPageToken())
		}
		return nil
	case "show":
		res, err := svc.GetJob(ctx, &athenav1.GetJobRequest{Id: positional[0]})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		job := res.GetJob()
		fmt.Fprintf(out, "id: %s\nkind: %s\nstatus: %s\nattempts: %d of %d\n", job.GetId(), job.GetKind(), job.GetStatus(), job.GetAttempts(), job.GetMaxAttempts())
		if job.GetNextRunAt() != "" {
			fmt.Fprintf(out, "next run: %s\n", job.GetNextRunAt())
		}
		if job.GetLeaseOwner() != "" {
			fmt.Fprintf(out, "lease: %s until %s\n", job.GetLeaseOwner(), job.GetLeaseExpiresAt())
		}
		if job.GetOccurrenceKey() != "" {
			fmt.Fprintf(out, "occurrence: %s\n", job.GetOccurrenceKey())
		}
		if job.GetOwnerUserId() != "" || job.GetOwnerRole() != "" {
			fmt.Fprintf(out, "owner: %s %s\n", job.GetOwnerUserId(), job.GetOwnerRole())
		}
		if job.GetLastError() != "" {
			fmt.Fprintf(out, "last error: %s\n", job.GetLastError())
		}
		fmt.Fprintf(out, "payload: %s\n", job.GetPayload())
		if job.GetResult() != "" {
			fmt.Fprintf(out, "result: %s\n", job.GetResult())
		}
		fmt.Fprintf(out, "created: %s\nupdated: %s\n", job.GetCreatedAt(), job.GetUpdatedAt())
		return nil
	case "retry":
		res, err := svc.RetryJob(ctx, &athenav1.RetryJobRequest{Id: positional[0]})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		fmt.Fprintf(out, "retried %s as %s.\n", positional[0], res.GetJobId())
		return nil
	default:
		res, err := svc.ListOccurrences(ctx, &athenav1.ListOccurrencesRequest{Schedule: *schedule, Scope: *scope, Status: *status, Limit: int32(*limit)})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		for _, occurrence := range res.GetOccurrences() {
			fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\n", occurrence.GetPeriod(), occurrence.GetStatus(),
				occurrence.GetScope(), occurrence.GetTimezone(), occurrence.GetJobId())
		}
		return nil
	}
}
