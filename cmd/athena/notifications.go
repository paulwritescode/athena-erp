package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"

	"athena-erp/internal/auth"
	"athena-erp/internal/notifications"
)

const inboxUsage = "usage: athena notifications list [--unread] [--limit N] [--page token] | read <id>... | read-all [--user name] [--workspace slug]"

func notificationsCommand(args []string, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, inboxUsage)
		return nil
	}
	verb, args := args[0], args[1:]
	f := flag.NewFlagSet("notifications "+verb, flag.ContinueOnError)
	user := f.String("user", auth.RootUsername, "inbox owner's username")
	slug := f.String("workspace", "", "Workspace slug")
	var ids []string
	var unread bool
	var limit int
	var page string
	switch verb {
	case "read":
		for len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
			ids = append(ids, args[0])
			args = args[1:]
		}
	case "list":
		f.BoolVar(&unread, "unread", false, "show unread notifications")
		f.IntVar(&limit, "limit", 30, "page size (1 to 100)")
		f.StringVar(&page, "page", "", "next page token")
	case "read-all":
	default:
		return errors.New(inboxUsage)
	}
	if ok, err := parse(f, args); !ok {
		return err
	}
	if verb == "read" {
		ids = append(ids, f.Args()...)
		if len(ids) == 0 {
			return errors.New(inboxUsage)
		}
	} else if f.NArg() > 0 {
		return errors.New(inboxUsage)
	}
	if verb == "list" && (limit < 1 || limit > 100) {
		return errors.New("limit must be between 1 and 100")
	}
	k, ctx, close, err := openKernel(*slug)
	if err != nil {
		return err
	}
	defer close()
	var id string
	db := k.Users.DB
	if err := db.QueryRowContext(ctx, "SELECT id FROM athena_users WHERE username=?", *user).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("unknown inbox owner")
		}
		return err
	}
	switch verb {
	case "list":
		result, err := notifications.List(ctx, db, id, int32(limit), page, unread)
		if err != nil {
			return err
		}
		return writeJSON(out, result)
	case "read":
		changed, err := notifications.MarkRead(ctx, db, id, ids)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%d marked as read\n", changed)
	case "read-all":
		changed, err := notifications.MarkAllRead(ctx, db, id)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%d marked as read\n", changed)
	}
	return nil
}
