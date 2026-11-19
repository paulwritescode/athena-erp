package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"
	"athena-erp/internal/server"

	"google.golang.org/protobuf/encoding/protojson"
)

const userUsage = `usage: athena user create|list|set|reset-password <username> [--workspace slug] [--json]
  create <username>             create a user; [--owner] makes an instance owner, who holds every
                                permission (owners are created here only, never in the desk);
                                [--display-name text] [--roles a,b]. The password is read from
                                ATHENA_USER_PASSWORD or prompted for, and needs at least 12 characters
  list                          list users with their Roles, lock and disabled state
  set <username>                [--display-name text] [--roles a,b] replaces the Roles (--roles ""
                                removes them), [--disable] or [--enable], [--unlock] clears a lockout.
                                Disabling a user ends their sessions
  reset-password <username>     set a new password (ATHENA_USER_PASSWORD or prompt) and end the
                                user's sessions`

const roleUsage = `usage: athena role list|show|save|delete [name] [--workspace slug] [--json]
  list                          list Roles with their permissions and Grants
  show <name>                   print one Role as JSON
  save --file <path>            create or replace a Role from a JSON file in the Role message format:
                                {"name":"Clerk","systemPermissions":["build"],"grants":[{"recordTypeKey":
                                "invoice","actions":["read","create"],"permissionLevel":0,
                                "condition":"doc.owner == user.name","separateDuties":false}]}
  delete <name> --yes           delete a Role; its users lose its Grants at once`

const accessLogUsage = `usage: athena access-log [--event name] [--limit N] [--before id] [--workspace slug] [--json]
  Print the Access Log newest first: sign-ins, failed sign-ins, lockouts, sign-outs, denials and
  user and Role changes. It holds usernames, operations and outcome codes, never a password, token
  or Record value. --event filters, for example --event denied or --event login_failed`

// newPassword reads a password for user create and reset-password.
func newPassword() (string, error) {
	if password, ok := os.LookupEnv("ATHENA_USER_PASSWORD"); ok {
		return password, nil
	}
	password, err := promptSecret("New password: ")
	if err != nil {
		return "", errors.New("no terminal for the password prompt; set ATHENA_USER_PASSWORD")
	}
	again, err := promptSecret("Repeat the password: ")
	if err != nil {
		return "", err
	}
	if again != password {
		return "", errors.New("the passwords do not match")
	}
	return password, nil
}

func splitList(text string) []string {
	var out []string
	for _, item := range strings.Split(text, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func userCommand(args []string, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, userUsage)
		return nil
	}
	sub, args := args[0], args[1:]
	want := map[string]int{"create": 1, "list": 0, "set": 1, "reset-password": 1}
	n, ok := want[sub]
	if !ok {
		return fmt.Errorf("unknown user command %q; use create, list, set or reset-password", sub)
	}
	positional, args, err := leadingArgs(args, n)
	if err != nil {
		return fmt.Errorf("user %s: %w", sub, err)
	}
	flags := flag.NewFlagSet("user "+sub, flag.ContinueOnError)
	slug := flags.String("workspace", "", "workspace slug, when running from the home directory")
	asJSON := flags.Bool("json", false, "print JSON")
	owner := flags.Bool("owner", false, "make the user an instance owner (create)")
	displayName := flags.String("display-name", "", "name shown in the desk (create, set)")
	roles := flags.String("roles", "", "comma-separated Role names (create, set)")
	disable := flags.Bool("disable", false, "disable the user and end their sessions (set)")
	enable := flags.Bool("enable", false, "enable a disabled user (set)")
	unlock := flags.Bool("unlock", false, "clear a sign-in lockout (set)")
	if ok, err := parse(flags, args); !ok {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if *disable && *enable {
		return errors.New("pass --disable or --enable, not both")
	}
	set := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { set[f.Name] = true })
	var password string
	if sub == "create" || sub == "reset-password" {
		if password, err = newPassword(); err != nil {
			return err
		}
	}
	kernel, ctx, closeKernel, err := openKernel(*slug)
	if err != nil {
		return err
	}
	defer closeKernel()
	switch sub {
	case "list":
		res, err := kernel.Auth.ListUsers(ctx, &athenav1.ListUsersRequest{})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		for _, u := range res.GetUsers() {
			var state []string
			if u.GetOwner() {
				state = append(state, "owner")
			}
			if u.GetDisabled() {
				state = append(state, "disabled")
			}
			if u.GetLocked() {
				state = append(state, "locked")
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", u.GetUsername(), u.GetDisplayName(), strings.Join(state, ","), strings.Join(u.GetRoles(), ","))
		}
		return w.Flush()
	case "create":
		// The operator creates users through the auth store directly, which
		// is the only way to create an instance owner.
		user, err := kernel.Users.CreateUser(ctx, positional[0], *displayName, password, *owner)
		if err != nil {
			return err
		}
		if names := splitList(*roles); len(names) > 0 {
			if user, err = kernel.Users.UpdateUser(ctx, user.Username, auth.UserUpdate{Roles: &names}); err != nil {
				return err
			}
		}
		kernel.Users.LogAccess(ctx, auth.AccessEvent{Event: auth.EventUserCreated, Outcome: auth.OutcomeOK, Detail: "user " + user.Username + " (cli)"})
		if *asJSON {
			return writeProto(out, server.AccessUserMessage(user, time.Now()))
		}
		kind := "user"
		if *owner {
			kind = "instance owner"
		}
		fmt.Fprintf(out, "Created %s %s.\n", kind, user.Username)
		return nil
	case "reset-password":
		if _, err := kernel.Auth.ResetPassword(ctx, &athenav1.ResetPasswordRequest{Username: positional[0], NewPassword: password}); err != nil {
			return rpcError(err)
		}
		fmt.Fprintf(out, "Set a new password for %s and ended their sessions.\n", positional[0])
		return nil
	}
	req := &athenav1.UpdateUserRequest{Username: positional[0], Unlock: *unlock}
	if set["display-name"] {
		req.DisplayName = displayName
	}
	if *disable || *enable {
		req.Disabled = disable
	}
	if set["roles"] {
		req.Roles = &athenav1.RoleNames{Names: splitList(*roles)}
	}
	res, err := kernel.Auth.UpdateUser(ctx, req)
	if err != nil {
		return rpcError(err)
	}
	if *asJSON {
		return writeProto(out, res.GetUser())
	}
	fmt.Fprintf(out, "Updated %s.\n", res.GetUser().GetUsername())
	return nil
}

func roleCommand(args []string, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, roleUsage)
		return nil
	}
	sub, args := args[0], args[1:]
	want := map[string]int{"list": 0, "show": 1, "save": 0, "delete": 1}
	n, ok := want[sub]
	if !ok {
		return fmt.Errorf("unknown role command %q; use list, show, save or delete", sub)
	}
	positional, args, err := leadingArgs(args, n)
	if err != nil {
		return fmt.Errorf("role %s: %w", sub, err)
	}
	flags := flag.NewFlagSet("role "+sub, flag.ContinueOnError)
	slug := flags.String("workspace", "", "workspace slug, when running from the home directory")
	asJSON := flags.Bool("json", false, "print JSON")
	file := flags.String("file", "", "Role JSON file (save)")
	yes := flags.Bool("yes", false, "confirm the deletion (delete)")
	if ok, err := parse(flags, args); !ok {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	var role athenav1.Role
	if sub == "save" {
		if *file == "" {
			return errors.New("role save needs --file")
		}
		payload, err := os.ReadFile(*file)
		if err != nil {
			return err
		}
		if err := protojson.Unmarshal(payload, &role); err != nil {
			return fmt.Errorf("read %s: %w", *file, err)
		}
	}
	if sub == "delete" && !*yes {
		return errors.New("role delete removes the Role from every user who holds it; pass --yes to confirm")
	}
	kernel, ctx, closeKernel, err := openKernel(*slug)
	if err != nil {
		return err
	}
	defer closeKernel()
	switch sub {
	case "save":
		res, err := kernel.Auth.SaveRole(ctx, &athenav1.SaveRoleRequest{Role: &role})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res.GetRole())
		}
		fmt.Fprintf(out, "Saved Role %s with %d Grant(s).\n", res.GetRole().GetName(), len(res.GetRole().GetGrants()))
		return nil
	case "delete":
		if _, err := kernel.Auth.DeleteRole(ctx, &athenav1.DeleteRoleRequest{Name: positional[0]}); err != nil {
			return rpcError(err)
		}
		fmt.Fprintf(out, "Deleted Role %s.\n", positional[0])
		return nil
	}
	res, err := kernel.Auth.ListRoles(ctx, &athenav1.ListRolesRequest{})
	if err != nil {
		return rpcError(err)
	}
	if sub == "show" {
		for _, r := range res.GetRoles() {
			if r.GetName() == positional[0] {
				return writeProto(out, r)
			}
		}
		return fmt.Errorf("Role %q does not exist", positional[0])
	}
	if *asJSON {
		return writeProto(out, res)
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	for _, r := range res.GetRoles() {
		var grants []string
		for _, g := range r.GetGrants() {
			grants = append(grants, fmt.Sprintf("%s:%s@%d", g.GetRecordTypeKey(), strings.Join(g.GetActions(), "+"), g.GetPermissionLevel()))
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.GetName(), strings.Join(r.GetSystemPermissions(), ","), strings.Join(grants, " "))
	}
	return w.Flush()
}

func accessLogCommand(args []string, out io.Writer) error {
	if len(args) > 0 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(out, accessLogUsage)
		return nil
	}
	flags := flag.NewFlagSet("access-log", flag.ContinueOnError)
	slug := flags.String("workspace", "", "workspace slug, when running from the home directory")
	asJSON := flags.Bool("json", false, "print JSON")
	event := flags.String("event", "", "only this event, such as denied or login_failed")
	limit := flags.Int("limit", 50, "most rows to print, at most 200")
	before := flags.Int64("before", 0, "only rows with a smaller ID, for paging")
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
	res, err := kernel.Auth.ListAccessLog(ctx, &athenav1.ListAccessLogRequest{PageSize: int32(*limit), BeforeId: *before, Event: *event})
	if err != nil {
		return rpcError(err)
	}
	if *asJSON {
		return writeProto(out, res)
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	for _, e := range res.GetEvents() {
		target := e.GetRecordTypeKey()
		if e.GetAction() != "" {
			target = strings.TrimPrefix(target+" "+e.GetAction(), " ")
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n", e.GetId(), e.GetAt(), e.GetEvent(), e.GetOutcome(), e.GetUsername(), target, e.GetDetail())
	}
	return w.Flush()
}
