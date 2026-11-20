package main

import (
	"encoding/json"
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

const userUsage = `usage: athena user invite|list|set|resend|reset|suspend|reactivate|sessions <username> [--workspace slug] [--json]
  invite <username>             invite a user; [--owner] makes an instance owner, who holds
                                the Instance Owner role (owners are created here only, never
                                in the desk);
                                [--display-name text] [--roles a,b]. Prints a one-time invite
                                challenge, shown once; the user chooses their own password with it.
                                create is the same command under its old name
  list                          list users with their Roles, lock and disabled state
  set <username>                [--display-name text] [--roles a,b] replaces the Roles (--roles ""
                                removes them), [--disable] or [--enable], [--unlock] clears a lockout.
                                Disabling a user ends their sessions
  resend <username>             invalidate an invited user's earlier Challenges and print a
                                new one-time invite challenge, shown once
  reset <username>              move an active user to reset_required, end their sessions
                                and print a one-time reset challenge, shown once
  suspend <username>            disable a user and end their sessions
  reactivate <username>         enable a suspended user; review their privileged
                                assignments afterwards
  sessions <username>           list the user's live sessions; [--revoke id] ends one`

const roleUsage = `usage: athena role list|show|save|delete|assign|revoke|explain [name] [--workspace slug] [--json]
  list                          list Roles with their permissions and Grants
  show <name>                   print one Role as JSON
  save --file <path>            create or replace a Role from a JSON file in the Role message format:
                                {"name":"Clerk","systemPermissions":["build"],"grants":[{"recordTypeKey":
                                "invoice","actions":["read","create"],"permissionLevel":0,
                                "condition":"doc.owner == user.name","separateDuties":false}]}
  delete <name> --yes           delete a Role; its users lose its Grants at once
  assign <username> <role>      give the user the Role with its own Scope and dates:
                                [--scope company=acme,globex;branch=north]
                                [--from YYYY-MM-DD] [--to YYYY-MM-DD]
  revoke <username> <role>      remove every Assignment of the Role from the user
  explain <username> <action> <record-type>
                                show which Assignment allows the action or why each
                                one fails; [--record id] also tests row conditions
                                and Scopes against that Record`

const accessLogUsage = `usage: athena access-log [--event name] [--limit N] [--before id] [--workspace slug] [--json]
  Print the Access Log newest first: sign-ins, failed sign-ins, lockouts, sign-outs, denials and
  user and Role changes. It holds usernames, operations and outcome codes, never a password, token
  or Record value. --event filters, for example --event denied or --event login_failed`

// writeInvite prints the invited user with the one-time Challenge in JSON.
func writeInvite(out io.Writer, user *athenav1.AccessUser, challenge string) error {
	payload, err := protojson.Marshal(user)
	if err != nil {
		return err
	}
	var userJSON map[string]any
	if err := json.Unmarshal([]byte(payload), &userJSON); err != nil {
		return err
	}
	wrapped, err := json.MarshalIndent(map[string]any{"user": userJSON, "inviteChallenge": challenge}, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(wrapped))
	return err
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

// parseScopeFlag reads an Assignment Scope in dimension=value,... form,
// with dimensions separated by semicolons: company=acme,globex;branch=north.
func parseScopeFlag(text string) (auth.Scope, error) {
	scope := auth.Scope{}
	if strings.TrimSpace(text) == "" {
		return scope, nil
	}
	for _, part := range strings.Split(text, ";") {
		dimension, values, ok := strings.Cut(part, "=")
		if dimension = strings.TrimSpace(dimension); !ok || dimension == "" {
			return nil, fmt.Errorf("scope %q needs dimension=value,...", part)
		}
		var list []string
		for _, value := range strings.Split(values, ",") {
			list = append(list, strings.TrimSpace(value))
		}
		scope[dimension] = list
	}
	return scope, nil
}

func parseDateFlag(text string) (*time.Time, error) {
	if text == "" {
		return nil, nil
	}
	at, err := time.Parse("2006-01-02", text)
	if err != nil {
		return nil, fmt.Errorf("date %q: use YYYY-MM-DD", text)
	}
	return &at, nil
}

func userCommand(args []string, out io.Writer) error {
	if isHelp(args) {
		fmt.Fprintln(out, userUsage)
		return nil
	}
	sub, args := args[0], args[1:]
	want := map[string]int{"create": 1, "invite": 1, "list": 0, "set": 1, "resend": 1, "reset": 1, "suspend": 1, "reactivate": 1, "sessions": 1}
	n, ok := want[sub]
	if !ok {
		return fmt.Errorf("unknown user command %q; use invite, list, set, resend, reset, suspend, reactivate or sessions", sub)
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
	revoke := flags.String("revoke", "", "end one session by ID (sessions)")
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
	case "create", "invite":
		// The operator invites users through the auth store directly, which
		// is the only way to create an instance owner. The invite Challenge
		// is shown once here; the user chooses their own password with it.
		var user auth.User
		var challenge string
		if *owner {
			user, challenge, err = kernel.Users.BootstrapOwner(ctx, positional[0], *displayName, "")
		} else {
			user, challenge, err = kernel.Users.CreateUser(ctx, positional[0], *displayName, false, "")
		}
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
			return writeInvite(out, server.AccessUserMessage(user, time.Now()), challenge)
		}
		kind := "user"
		if *owner {
			kind = "instance owner"
		}
		fmt.Fprintf(out, "Created %s %s.\nInvite challenge, shown once: %s\nThey choose their own password with it at first sign-in.\n", kind, user.Username, challenge)
		return nil
	case "resend":
		user, challenge, err := kernel.Users.ResendInvite(ctx, positional[0], "")
		if err != nil {
			return err
		}
		if *asJSON {
			return writeInvite(out, server.AccessUserMessage(user, time.Now()), challenge)
		}
		fmt.Fprintf(out, "Resent the invite for %s.\nInvite challenge, shown once: %s\nThey choose their own password with it at first sign-in.\n", user.Username, challenge)
		return nil
	case "reset":
		res, err := kernel.Auth.StartReset(ctx, &athenav1.StartResetRequest{Username: positional[0]})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeInvite(out, res.GetUser(), res.GetChallenge())
		}
		fmt.Fprintf(out, "Reset started for %s and ended their sessions.\nReset challenge, shown once: %s\nThey choose a new password with it.\n", res.GetUser().GetUsername(), res.GetChallenge())
		return nil
	case "suspend":
		suspended := true
		if _, err := kernel.Users.UpdateUser(ctx, positional[0], auth.UserUpdate{Disabled: &suspended}); err != nil {
			return err
		}
		fmt.Fprintf(out, "Suspended %s and ended their sessions.\n", positional[0])
		return nil
	case "reactivate":
		suspended := false
		if _, err := kernel.Users.UpdateUser(ctx, positional[0], auth.UserUpdate{Disabled: &suspended}); err != nil {
			return err
		}
		fmt.Fprintf(out, "Reactivated %s. Review their privileged assignments.\n", positional[0])
		return nil
	case "sessions":
		if *revoke != "" {
			if err := kernel.Users.RevokeSession(ctx, "", *revoke, true); err != nil {
				return err
			}
			fmt.Fprintf(out, "Ended session %s.\n", *revoke)
			return nil
		}
		sessions, err := kernel.Users.ListSessions(ctx, positional[0])
		if err != nil {
			return err
		}
		if *asJSON {
			res := &athenav1.ListSessionsResponse{}
			for _, session := range sessions {
				res.Sessions = append(res.Sessions, server.SessionMessage(session, false))
			}
			return writeProto(out, res)
		}
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		for _, session := range sessions {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", session.ID, session.CreatedAt.UTC().Format(time.RFC3339),
				session.LastSeenAt.UTC().Format(time.RFC3339), session.ExpiresAt.UTC().Format(time.RFC3339), session.Device)
		}
		return w.Flush()
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
	want := map[string]int{"list": 0, "show": 1, "save": 0, "delete": 1, "assign": 2, "revoke": 2, "explain": 3}
	n, ok := want[sub]
	if !ok {
		return fmt.Errorf("unknown role command %q; use list, show, save, delete, assign, revoke or explain", sub)
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
	scopeText := flags.String("scope", "", "Assignment Scope as dimension=value,... pairs separated by ; (assign)")
	from := flags.String("from", "", "active-from date YYYY-MM-DD (assign)")
	to := flags.String("to", "", "expiry date YYYY-MM-DD (assign)")
	recordID := flags.String("record", "", "Record ID to test row conditions and Scopes against (explain)")
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
	case "assign":
		scope, err := parseScopeFlag(*scopeText)
		if err != nil {
			return err
		}
		activeFrom, err := parseDateFlag(*from)
		if err != nil {
			return err
		}
		expiresAt, err := parseDateFlag(*to)
		if err != nil {
			return err
		}
		if _, err := kernel.Users.SaveAssignment(ctx, positional[0], auth.AssignmentInput{Role: positional[1], Scope: scope, ActiveFrom: activeFrom, ExpiresAt: expiresAt}); err != nil {
			return err
		}
		var detail []string
		if *scopeText != "" {
			detail = append(detail, "scope "+*scopeText)
		}
		if *from != "" || *to != "" {
			start, end := *from, *to
			if start == "" {
				start = "unbounded"
			}
			if end == "" {
				end = "unbounded"
			}
			detail = append(detail, "active "+start+" to "+end)
		}
		suffix := ""
		if len(detail) > 0 {
			suffix = " (" + strings.Join(detail, ", ") + ")"
		}
		fmt.Fprintf(out, "Assigned %s to %s%s.\n", positional[1], positional[0], suffix)
		return nil
	case "revoke":
		assignments, err := kernel.Users.ListAssignments(ctx, positional[0])
		if err != nil {
			return err
		}
		revoked := 0
		for _, assignment := range assignments {
			if assignment.RoleName != positional[1] {
				continue
			}
			if err := kernel.Users.RevokeAssignment(ctx, assignment.ID); err != nil {
				return err
			}
			revoked++
		}
		if revoked == 0 {
			return fmt.Errorf("user %s holds no %s assignment", positional[0], positional[1])
		}
		plural := "s"
		if revoked == 1 {
			plural = ""
		}
		fmt.Fprintf(out, "Revoked %d assignment%s of %s from %s.\n", revoked, plural, positional[1], positional[0])
		return nil
	case "explain":
		res, err := kernel.Auth.ExplainAccess(ctx, &athenav1.ExplainAccessRequest{Username: positional[0], Action: positional[1], RecordTypeKey: positional[2], RecordId: *recordID})
		if err != nil {
			return rpcError(err)
		}
		if *asJSON {
			return writeProto(out, res)
		}
		verdict, scope := "may not", ""
		if res.GetAllowed() {
			verdict = "may"
		}
		if *recordID != "" {
			scope = " on record " + *recordID
		}
		fmt.Fprintf(out, "%s %s %s %s%s.\n", positional[0], verdict, positional[1], positional[2], scope)
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		for _, assignment := range res.GetAssignments() {
			result := "matches"
			if !assignment.GetMatched() {
				result = strings.Join(assignment.GetReasons(), "; ")
			}
			if assignment.GetSeparateDutiesConflict() {
				result += " (separate duties conflict)"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", assignment.GetRole(), assignment.GetAssignmentId(), result)
		}
		return w.Flush()
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
