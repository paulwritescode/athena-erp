package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"

	"athena-erp/internal/events"
	"athena-erp/internal/notifications"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

// Sign-in limits (spec 21). A user is locked for LockDuration after
// MaxFailedAttempts wrong passwords in a row. IdleTimeout and
// AbsoluteLifetime are the default everyday-user session lifetimes; each
// session fixes its own deadlines at creation from the Workspace security
// settings (security.session.* in security.go).
const (
	MaxFailedAttempts = 5
	LockDuration      = 15 * time.Minute
	IdleTimeout       = 30 * time.Minute
	AbsoluteLifetime  = 12 * time.Hour
)

// Errors the callers map to API codes.
var (
	// ErrInvalidCredentials covers a wrong password, an unknown or disabled
	// user and a locked user, so a sign-in reveals nothing about which.
	ErrInvalidCredentials = errors.New("wrong username or password, or the user is locked")
	// ErrLocked and ErrDisabled are returned only when the right password was
	// given, so they reveal nothing to someone guessing names.
	ErrLocked          = errors.New("this account is locked after too many failed sign-ins")
	ErrDisabled        = errors.New("this account is disabled")
	ErrUnauthenticated = errors.New("sign in to continue")
	// ErrChallengeRefused is the one public refusal for a Challenge that is
	// unknown, expired, consumed, superseded or presented wrongly too often.
	// The Access Log holds the specific reason; the caller never learns it.
	ErrChallengeRefused = errors.New("the code is unknown, expired or already used")
	// ErrWrongPassword reports a wrong password for an already-identified
	// user: a step-up or privileged-action check. Unlike
	// ErrInvalidCredentials it names only the password, never the username.
	ErrWrongPassword = errors.New("the password is wrong")
	ErrNotFound      = errors.New("not found")
	ErrExists        = errors.New("already exists")
	ErrLastOwner     = errors.New("the instance needs at least one enabled owner")
	ErrInvalid       = errors.New("invalid input")
	// ErrConflict reports a write whose expected revision no longer matches.
	ErrConflict = errors.New("someone else changed this; reload and try again")
	// ErrForbidden reports a change beyond the actor's delegation.
	ErrForbidden = errors.New("beyond your delegation")
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,62}$`)

// NormalizeUsername lowercases and trims a username and checks its form. A
// valid username holds no quote or space, which lets a row condition embed it
// as a literal safely.
func NormalizeUsername(username string) (string, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if !usernamePattern.MatchString(username) {
		return "", fmt.Errorf("%w: a username has 2 to 63 characters: letters, digits, dot, dash and underscore, starting with a letter or digit", ErrInvalid)
	}
	return username, nil
}

// Service reads and writes the authentication tables.
type Service struct {
	DB *sql.DB
	// Now returns the current time; tests replace it.
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// User states. Only active users sign in. Invited users hold no password;
// reset_required users must complete a reset Challenge; suspended users are
// refused like disabled ones (ADR-0017).
const (
	UserStateInvited       = "invited"
	UserStateActive        = "active"
	UserStateResetRequired = "reset_required"
	UserStateSuspended     = "suspended"
)

// Invite delivery channels of an account. Manual hands the single-use code
// to the administrator to show once; any other value names the
// destination alias the outbox delivery event carries for a future mail
// relay. Setup marks the workspace root (ADR-0018) and is never a
// destination.
const (
	DeliveryManual = "manual"
	DeliverySetup  = "setup"
)

// Delivery event types of the outbox invite and reset events (G8.11). The
// payload carries the destination alias, the kind, the Challenge id and
// the expiry only: the Challenge value itself stays out of the outbox,
// because secret custody is undecided (Z4).
const (
	TypeInviteCreated = "auth.invite_created"
	TypeResetStarted  = "auth.reset_started"
)

// Delivery kinds carried in the invite and reset event payload.
const (
	DeliveryKindInvite = "invite"
	DeliveryKindReset  = "reset"
)

// User is one account. It never carries a password hash.
type User struct {
	ID          string
	Username    string
	DisplayName string
	Owner       bool
	Disabled    bool
	State       string
	LockedUntil *time.Time
	LastLoginAt *time.Time
	Roles       []string
	// Delivery is the invite_delivery channel: manual, or a destination
	// alias for outbox delivery.
	Delivery string
}

// Locked reports whether the user is locked at time at.
func (u User) Locked(at time.Time) bool { return u.LockedUntil != nil && u.LockedUntil.After(at) }

// System Role names shipped by the core.
const (
	RolePlatformAdministrator = "Platform Administrator"
	RoleInstanceOwner         = "Instance Owner"
	RoleSystemManager         = "System Manager"
)

// Role names a set of system permissions and Grants.
type Role struct {
	ID                string
	Name              string
	Description       string
	SystemPermissions []string
	// RequiresScope lists the Scope dimensions an Assignment of this Role
	// must give. An Assignment that misses one allows nothing.
	RequiresScope []string
	// System marks the platform Roles. Assigning one needs the ownership
	// permission; the flag is set by seeding, never through the API.
	System bool
	Grants []Grant
}

func isDuplicate(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

func newID() string {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.NewString()
	}
	return id.String()
}

// validateNewUser checks a username and display name for a new account.
func validateNewUser(username, displayName string) (string, string, error) {
	username, err := NormalizeUsername(username)
	if err != nil {
		return "", "", err
	}
	if IsRoot(username) {
		return "", "", fmt.Errorf("%w: the username %q is reserved for the workspace root", ErrInvalid, RootUsername)
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = username
	}
	if len(displayName) > 255 {
		return "", "", fmt.Errorf("%w: the display name is too long", ErrInvalid)
	}
	return username, displayName, nil
}

// insertInvitedUser stores an invited account with no password at generation
// zero. The caller issues the invite Challenge in the same transaction.
// An empty delivery stores manual.
func insertInvitedUser(ctx context.Context, db execQuerier, username, displayName string, owner bool, delivery string) (User, error) {
	if delivery == "" {
		delivery = DeliveryManual
	}
	id := newID()
	if _, err := db.ExecContext(ctx, "INSERT INTO athena_users (id, username, display_name, password_hash, is_owner, state, generation, invite_delivery) VALUES (?,?,?,NULL,?,?,0,?)",
		id, username, displayName, owner, UserStateInvited, delivery); err != nil {
		if isDuplicate(err) {
			return User{}, fmt.Errorf("%w: the username %q is taken", ErrExists, username)
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return User{ID: id, Username: username, DisplayName: displayName, Owner: owner, State: UserStateInvited, Delivery: delivery}, nil
}

// CreateUser invites an account: it stores an invited user with no password
// and returns the single-use first-login Challenge, which the caller shows
// once. owner makes it an instance owner. Use BootstrapOwner for the first
// owner so two competing initialisations cannot both succeed.
// assignRoleTx gives the user a plain Assignment of the named Role.
func assignRoleTx(ctx context.Context, tx *sql.Tx, userID, role string) error {
	res, err := tx.ExecContext(ctx, "INSERT INTO athena_role_assignments (id, user_id, role_id, scope, revision) SELECT ?, ?, id, '{}', 1 FROM athena_roles WHERE name = ?", newID(), userID, role)
	if err != nil {
		return fmt.Errorf("assign role: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%w: Role %q", ErrNotFound, role)
	}
	return nil
}

func (s *Service) CreateUser(ctx context.Context, username, displayName string, owner bool, requestID string) (User, string, error) {
	return s.CreateUserWithDelivery(ctx, username, displayName, owner, "", requestID)
}

// CreateUserWithDelivery invites an account like CreateUser and stores
// the invite delivery channel with it: empty or manual keeps the manual
// code path, and a destination alias additionally enqueues an outbox
// delivery event in the same transaction. The single-use Challenge is
// returned either way, so the flow completes while no relay is deployed.
func (s *Service) CreateUserWithDelivery(ctx context.Context, username, displayName string, owner bool, delivery, requestID string) (User, string, error) {
	username, displayName, err := validateNewUser(username, displayName)
	if err != nil {
		return User{}, "", err
	}
	if delivery == "" {
		delivery = DeliveryManual
	}
	if err := checkDelivery(delivery); err != nil {
		return User{}, "", err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return User{}, "", fmt.Errorf("create user: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if owner {
		// Seed before inserting: the backfill would otherwise assign the
		// new owner a second time below.
		if err := ensureSystemRolesTx(ctx, tx); err != nil {
			return User{}, "", err
		}
	}
	user, err := insertInvitedUser(ctx, tx, username, displayName, owner, delivery)
	if err != nil {
		return User{}, "", err
	}
	challenge, token, err := IssueChallenge(ctx, tx, user.ID, ChallengePurposeFirstLogin, 0, s.now())
	if err != nil {
		return User{}, "", err
	}
	if err := emitDelivery(ctx, tx, user, 0, TypeInviteCreated, DeliveryKindInvite, challenge, requestID); err != nil {
		return User{}, "", err
	}
	if owner {
		if err := assignRoleTx(ctx, tx, user.ID, RoleInstanceOwner); err != nil {
			return User{}, "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return User{}, "", fmt.Errorf("create user: %w", err)
	}
	s.LogAccess(ctx, AccessEvent{Event: EventInviteCreated, Outcome: OutcomeOK, UserID: user.ID, Username: user.Username, RequestID: requestID, Detail: "first login"})
	return user, token, nil
}

// maxDeliveryLength is the invite_delivery column width.
const maxDeliveryLength = 20

// checkDelivery refuses a blank, overlong or reserved delivery channel.
// Setup belongs to the workspace root seeding only.
func checkDelivery(delivery string) error {
	if delivery == "" || len(delivery) > maxDeliveryLength {
		return fmt.Errorf("%w: the delivery channel needs 1 to 20 characters", ErrInvalid)
	}
	if delivery == DeliverySetup {
		return fmt.Errorf("%w: the delivery channel %q is reserved", ErrInvalid, delivery)
	}
	return nil
}

// wantsOutboxDelivery reports whether the channel names a destination a
// relay should deliver to. Manual and setup accounts keep the show-once
// code path.
func wantsOutboxDelivery(delivery string) bool {
	switch delivery {
	case "", DeliveryManual, DeliverySetup:
		return false
	default:
		return true
	}
}

// emitDelivery enqueues the invite or reset delivery event in the caller's
// transaction where the user has a destination, and does nothing for the
// manual code path. The payload carries the destination alias, the kind,
// the Challenge id and the expiry; the Challenge value itself stays out,
// because secret custody is undecided (Z4). generation is the Challenge's
// user generation, carried as the subject revision.
func emitDelivery(ctx context.Context, tx *sql.Tx, user User, generation int, eventType, kind string, challenge Challenge, requestID string) error {
	if !wantsOutboxDelivery(user.Delivery) {
		return nil
	}
	_, err := events.Emit(ctx, tx, events.Envelope{
		EventType:       eventType,
		Producer:        "auth",
		SubjectType:     "user",
		SubjectID:       user.ID,
		SubjectRevision: int64(generation),
		Actor:           ActorID(ctx),
		CorrelationID:   requestID,
		Payload: map[string]any{
			"destination":  user.Delivery,
			"kind":         kind,
			"challenge_id": challenge.ID,
			"expires_at":   challenge.ExpiresAt.UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		return fmt.Errorf("enqueue delivery: %w", err)
	}
	return nil
}

// BootstrapOwner invites the first instance owner. It holds a named lock
// while checking that no owner exists, so two competing initialisations
// yield one owner and one refusal. Like CreateUser it returns the invite
// Challenge to show once.
func (s *Service) BootstrapOwner(ctx context.Context, username, displayName, requestID string) (User, string, error) {
	username, displayName, err := validateNewUser(username, displayName)
	if err != nil {
		return User{}, "", err
	}
	// A transaction alone cannot guard this: two concurrent initialisations
	// would both see no owner and both insert. The named lock serialises
	// them; it is MariaDB-only, like the duplicate-key check above.
	var got sql.NullInt64
	if err := s.DB.QueryRowContext(ctx, `SELECT GET_LOCK(CONCAT('athena_owner_bootstrap_', DATABASE()), 10)`).Scan(&got); err != nil {
		return User{}, "", fmt.Errorf("bootstrap owner: %w", err)
	}
	if !got.Valid || got.Int64 != 1 {
		return User{}, "", errors.New("bootstrap owner: another initialisation is running")
	}
	defer func() {
		var released sql.NullInt64
		_ = s.DB.QueryRowContext(context.Background(), `SELECT RELEASE_LOCK(CONCAT('athena_owner_bootstrap_', DATABASE()))`).Scan(&released)
	}()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return User{}, "", fmt.Errorf("bootstrap owner: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var owners int
	// The root account does not count: it is flagged, but the bootstrap
	// stays available as emergency recovery while no owner exists.
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM athena_users WHERE is_owner = 1 AND username <> ?", RootUsername).Scan(&owners); err != nil {
		return User{}, "", fmt.Errorf("bootstrap owner: %w", err)
	}
	if owners > 0 {
		return User{}, "", fmt.Errorf("%w: an instance owner already exists", ErrExists)
	}
	// Seed before inserting: the backfill would otherwise assign the new
	// owner a second time below.
	if err := ensureSystemRolesTx(ctx, tx); err != nil {
		return User{}, "", err
	}
	user, err := insertInvitedUser(ctx, tx, username, displayName, true, DeliveryManual)
	if err != nil {
		return User{}, "", err
	}
	challenge, token, err := IssueChallenge(ctx, tx, user.ID, ChallengePurposeFirstLogin, 0, s.now())
	if err != nil {
		return User{}, "", err
	}
	// The first setup is always local, so this stays manual today; the
	// call keeps every Challenge issue on the delivery path.
	if err := emitDelivery(ctx, tx, user, 0, TypeInviteCreated, DeliveryKindInvite, challenge, requestID); err != nil {
		return User{}, "", err
	}
	if err := assignRoleTx(ctx, tx, user.ID, RoleInstanceOwner); err != nil {
		return User{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return User{}, "", fmt.Errorf("bootstrap owner: %w", err)
	}
	s.LogAccess(ctx, AccessEvent{Event: EventInviteCreated, Outcome: OutcomeOK, UserID: user.ID, Username: user.Username, RequestID: requestID, Detail: "bootstrap"})
	return user, token, nil
}

// ResendInvite invalidates an invited user's earlier Challenges and issues a
// new first-login Challenge, returned to show once.
func (s *Service) ResendInvite(ctx context.Context, username, requestID string) (User, string, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return User{}, "", fmt.Errorf("resend invite: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var user User
	var generation int
	err = tx.QueryRowContext(ctx, "SELECT id, username, display_name, state, generation, invite_delivery FROM athena_users WHERE username = ? FOR UPDATE",
		strings.ToLower(strings.TrimSpace(username))).Scan(&user.ID, &user.Username, &user.DisplayName, &user.State, &generation, &user.Delivery)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return User{}, "", fmt.Errorf("%w: user %q", ErrNotFound, username)
	case err != nil:
		return User{}, "", fmt.Errorf("resend invite: %w", err)
	}
	if user.State != UserStateInvited {
		return User{}, "", fmt.Errorf("%w: only an invited user can be re-invited", ErrInvalid)
	}
	generation++
	if _, err := tx.ExecContext(ctx, "UPDATE athena_users SET generation = ? WHERE id = ?", generation, user.ID); err != nil {
		return User{}, "", fmt.Errorf("resend invite: %w", err)
	}
	challenge, token, err := IssueChallenge(ctx, tx, user.ID, ChallengePurposeFirstLogin, generation, s.now())
	if err != nil {
		return User{}, "", err
	}
	if err := emitDelivery(ctx, tx, user, generation, TypeInviteCreated, DeliveryKindInvite, challenge, requestID); err != nil {
		return User{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return User{}, "", fmt.Errorf("resend invite: %w", err)
	}
	s.LogAccess(ctx, AccessEvent{Event: EventInviteResent, Outcome: OutcomeOK, UserID: user.ID, Username: user.Username, RequestID: requestID, Detail: "first login"})
	return user, token, nil
}

// StartReset moves an active user to reset_required, revokes their sessions,
// invalidates earlier Challenges and issues a reset Challenge to show once.
// Starting a reset again replaces the earlier reset Challenge. A signed-in
// administrator proves themself with their own password on every call: the
// command-line operator has no password and is exempt.
// verifyActorPassword re-verifies a signed-in user's password for a
// privileged action: step-up, ownership transfer or reset start. A wrong
// password counts toward the same failed-attempt counter and lockout as
// Login and is logged without the password; a right password clears the
// counter. The root account is never locked or counted. op names the action
// for the log ("step up", "transfer ownership", "start reset"). Callers skip
// the check for the command-line operator, which has no password; a nil
// actor here is refused.
func (s *Service) verifyActorPassword(ctx context.Context, actor *Principal, password, op, requestID string) error {
	if actor == nil {
		return ErrUnauthenticated
	}
	if password == "" {
		return fmt.Errorf("%w: the password is required", ErrInvalid)
	}
	now := s.now()
	var username string
	var hash sql.NullString
	var attempts int
	var locked sql.NullTime
	if err := s.DB.QueryRowContext(ctx, "SELECT username, password_hash, failed_attempts, locked_until FROM athena_users WHERE id = ?", actor.UserID).
		Scan(&username, &hash, &attempts, &locked); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	failed := func() error {
		s.LogAccess(ctx, AccessEvent{Event: EventStepUpFailed, Outcome: OutcomeDenied, UserID: actor.UserID, Username: username, RequestID: requestID, Detail: op + " wrong password"})
		return ErrWrongPassword
	}
	ok := hash.Valid
	if ok {
		ok, _ = VerifyPassword(password, hash.String)
	}
	if IsRoot(username) {
		// The root account verifies with the right password and never
		// locks: a wrong password fails without counting.
		if !ok {
			return failed()
		}
		return nil
	}
	if locked.Valid && locked.Time.After(now) {
		// Only the owner of the password learns the lockout, as in Login.
		if !ok {
			return failed()
		}
		s.LogAccess(ctx, AccessEvent{Event: EventStepUpFailed, Outcome: OutcomeDenied, UserID: actor.UserID, Username: username, RequestID: requestID, Detail: op + " locked"})
		return &LockedError{Until: locked.Time}
	}
	if !ok {
		attempts++
		if attempts >= MaxFailedAttempts {
			if _, err := s.DB.ExecContext(ctx, "UPDATE athena_users SET failed_attempts = 0, locked_until = ? WHERE id = ?", now.Add(LockDuration), actor.UserID); err != nil {
				return fmt.Errorf("%s: lock user: %w", op, err)
			}
			s.LogAccess(ctx, AccessEvent{Event: EventLockout, Outcome: OutcomeDenied, UserID: actor.UserID, Username: username, RequestID: requestID, Detail: op + " wrong password"})
		} else if _, err := s.DB.ExecContext(ctx, "UPDATE athena_users SET failed_attempts = ? WHERE id = ?", attempts, actor.UserID); err != nil {
			return fmt.Errorf("%s: count failed password: %w", op, err)
		}
		return failed()
	}
	if _, err := s.DB.ExecContext(ctx, "UPDATE athena_users SET failed_attempts = 0, locked_until = NULL WHERE id = ?", actor.UserID); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// TransferOwnership appoints the recipient an Instance Owner and, when asked,
// relinquishes the caller's own ownership assignments. The caller must hold
// the ownership permission and prove themself with their password; the
// recipient must be active. Everything applies in one guarded transaction,
// so relinquishing never strands the Workspace.
func (s *Service) TransferOwnership(ctx context.Context, username, actorPassword string, relinquish bool, requestID string) (User, error) {
	var recipient User
	err := retryGuarded(func() error {
		var err error
		recipient, err = s.transferOwnershipOnce(ctx, username, actorPassword, relinquish, requestID)
		return err
	})
	return recipient, err
}

func (s *Service) transferOwnershipOnce(ctx context.Context, username, actorPassword string, relinquish bool, requestID string) (User, error) {
	actor := PrincipalFrom(ctx)
	if actor == nil {
		return User{}, ErrUnauthenticated
	}
	if !actor.HasSystem(PermissionAccessOwnership) {
		return User{}, fmt.Errorf("%w: only an Instance Owner transfers ownership", ErrForbidden)
	}
	if err := s.verifyActorPassword(ctx, actor, actorPassword, "transfer ownership", requestID); err != nil {
		return User{}, err
	}
	name := strings.ToLower(strings.TrimSpace(username))
	if name == strings.ToLower(actor.Username) {
		return User{}, fmt.Errorf("%w: ownership cannot move to the caller themself", ErrInvalid)
	}
	if IsRoot(name) {
		return User{}, fmt.Errorf("%w: the Administrator account needs no ownership", ErrInvalid)
	}
	tx, err := beginGuardedTx(ctx, s.DB)
	if err != nil {
		return User{}, fmt.Errorf("transfer ownership: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	today := s.now().Format("2006-01-02")
	before, err := beginOwnershipGuard(ctx, tx, today)
	if err != nil {
		return User{}, err
	}
	recipient, err := s.userByName(ctx, tx, name, "")
	if err != nil {
		return User{}, err
	}
	if recipient.State != UserStateActive {
		return User{}, fmt.Errorf("%w: only an active user can receive ownership", ErrInvalid)
	}
	if err := ensureSystemRolesTx(ctx, tx); err != nil {
		return User{}, err
	}
	granted, err := tx.ExecContext(ctx, `INSERT INTO athena_role_assignments (id, user_id, role_id, scope, revision)
		SELECT ?, ?, id, '{}', 1 FROM athena_roles WHERE name = ?
		AND NOT EXISTS (SELECT 1 FROM athena_role_assignments a JOIN athena_roles r ON r.id = a.role_id
			WHERE a.user_id = ? AND JSON_CONTAINS(r.system_permissions, '"access:ownership"'))`,
		newID(), recipient.ID, RoleInstanceOwner, recipient.ID)
	if err != nil {
		return User{}, fmt.Errorf("transfer ownership: %w", err)
	}
	if changed, _ := granted.RowsAffected(); changed > 0 {
		if err := notifyAccess(ctx, tx, recipient.ID); err != nil {
			return User{}, err
		}
	}
	if err := syncOwnerFlag(ctx, tx, recipient.ID); err != nil {
		return User{}, err
	}
	var dropped int64
	if relinquish {
		removed, err := tx.ExecContext(ctx, `DELETE a FROM athena_role_assignments a JOIN athena_roles r ON r.id = a.role_id
			WHERE a.user_id = ? AND JSON_CONTAINS(r.system_permissions, '"access:ownership"')`, actor.UserID)
		if err != nil {
			return User{}, fmt.Errorf("transfer ownership: %w", err)
		}
		if dropped, err = removed.RowsAffected(); err != nil {
			return User{}, err
		}
		if err := notifyAccess(ctx, tx, actor.UserID); err != nil {
			return User{}, err
		}
		if err := syncOwnerFlag(ctx, tx, actor.UserID); err != nil {
			return User{}, err
		}
	}
	if err := endOwnershipGuard(ctx, tx, today, before); err != nil {
		return User{}, err
	}
	// Only a change ends sessions: re-appointing an owner, or relinquishing
	// with nothing to give up, leaves live sessions alone.
	if n, err := granted.RowsAffected(); err != nil {
		return User{}, err
	} else if n > 0 {
		if err := revokeSessions(ctx, tx, recipient.ID, "", s.now(), true); err != nil {
			return User{}, err
		}
	}
	if dropped > 0 {
		if err := revokeSessions(ctx, tx, actor.UserID, "", s.now(), true); err != nil {
			return User{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return User{}, fmt.Errorf("transfer ownership: %w", err)
	}
	return recipient, nil
}

func (s *Service) StartReset(ctx context.Context, username, actorPassword, requestID string) (User, string, error) {
	if actor := PrincipalFrom(ctx); actor != nil {
		if err := s.verifyActorPassword(ctx, actor, actorPassword, "start reset", requestID); err != nil {
			return User{}, "", err
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return User{}, "", fmt.Errorf("start reset: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var user User
	var generation int
	err = tx.QueryRowContext(ctx, "SELECT id, username, display_name, state, generation, invite_delivery FROM athena_users WHERE username = ? FOR UPDATE",
		strings.ToLower(strings.TrimSpace(username))).Scan(&user.ID, &user.Username, &user.DisplayName, &user.State, &generation, &user.Delivery)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return User{}, "", fmt.Errorf("%w: user %q", ErrNotFound, username)
	case err != nil:
		return User{}, "", fmt.Errorf("start reset: %w", err)
	}
	switch user.State {
	case UserStateActive, UserStateResetRequired:
	case UserStateInvited:
		return User{}, "", fmt.Errorf("%w: the user has no password yet; resend the invite", ErrInvalid)
	default:
		return User{}, "", fmt.Errorf("%w: a suspended user cannot be reset", ErrInvalid)
	}
	generation++
	if _, err := tx.ExecContext(ctx, "UPDATE athena_users SET state = ?, generation = ? WHERE id = ?", UserStateResetRequired, generation, user.ID); err != nil {
		return User{}, "", fmt.Errorf("start reset: %w", err)
	}
	if err := revokeSessions(ctx, tx, user.ID, "", s.now(), true); err != nil {
		return User{}, "", err
	}
	challenge, token, err := IssueChallenge(ctx, tx, user.ID, ChallengePurposeReset, generation, s.now())
	if err != nil {
		return User{}, "", err
	}
	if err := emitDelivery(ctx, tx, user, generation, TypeResetStarted, DeliveryKindReset, challenge, requestID); err != nil {
		return User{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return User{}, "", fmt.Errorf("start reset: %w", err)
	}
	user.State = UserStateResetRequired
	s.LogAccess(ctx, AccessEvent{Event: EventResetStarted, Outcome: OutcomeOK, UserID: user.ID, Username: user.Username, RequestID: requestID, Detail: "reset"})
	return user, token, nil
}

// CompleteChallenge consumes a first-login or reset Challenge, stores the
// chosen password, activates the user and starts a session. The password is
// checked before the Challenge is touched, so a weak choice never burns a
// good Challenge. Every refusal returns ErrChallengeRefused; the Access Log
// holds the specific reason. Two concurrent completions yield one success.
func (s *Service) CompleteChallenge(ctx context.Context, username, purpose, token, password, requestID string) (LoginResult, error) {
	return s.CompleteChallengeWithOptions(ctx, username, purpose, token, password, requestID, SessionOptions{})
}

func (s *Service) CompleteChallengeWithOptions(ctx context.Context, username, purpose, token, password, requestID string, options SessionOptions) (LoginResult, error) {
	var want string
	switch purpose {
	case ChallengePurposeFirstLogin:
		want = UserStateInvited
	case ChallengePurposeReset:
		want = UserStateResetRequired
	default:
		return LoginResult{}, fmt.Errorf("%w: Challenge purpose %q", ErrInvalid, purpose)
	}
	name := strings.ToLower(strings.TrimSpace(username))
	if err := CheckPassword(name, password); err != nil {
		return LoginResult{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	now := s.now()
	fail := func(userID, username, reason string) (LoginResult, error) {
		s.LogAccess(ctx, AccessEvent{Event: EventChallengeFailed, Outcome: OutcomeDenied, UserID: userID, Username: username, RequestID: requestID, Detail: purpose + " " + reason})
		return LoginResult{}, ErrChallengeRefused
	}
	var user User
	var generation int
	err := s.DB.QueryRowContext(ctx, "SELECT id, username, display_name, state, generation FROM athena_users WHERE username = ?", name).
		Scan(&user.ID, &user.Username, &user.DisplayName, &user.State, &generation)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// The attempted name is not logged: it may be a mistyped password.
		return fail("", "", "unknown user")
	case err != nil:
		return LoginResult{}, fmt.Errorf("complete Challenge: %w", err)
	}
	if user.State != want {
		return fail(user.ID, user.Username, "wrong state")
	}
	c, ok, err := latestChallenge(ctx, s.DB, user.ID, purpose)
	switch {
	case errors.Is(err, errChallengeConsumed):
		return fail(user.ID, user.Username, "consumed")
	case err != nil:
		return LoginResult{}, fmt.Errorf("complete Challenge: %w", err)
	case !ok:
		return fail(user.ID, user.Username, "no Challenge")
	}
	if reason, wrong := validateChallenge(c, token, generation, now); reason != "" {
		if wrong {
			if err := countAttempt(ctx, s.DB, c.ID); err != nil {
				return LoginResult{}, err
			}
		}
		return fail(user.ID, user.Username, reason)
	}
	hash, err := HashPassword(password)
	if err != nil {
		return LoginResult{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return LoginResult{}, fmt.Errorf("complete Challenge: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var state string
	if err := tx.QueryRowContext(ctx, "SELECT state, generation FROM athena_users WHERE id = ? FOR UPDATE", user.ID).Scan(&state, &generation); err != nil {
		return LoginResult{}, fmt.Errorf("complete Challenge: %w", err)
	}
	if state != want {
		return fail(user.ID, user.Username, "wrong state")
	}
	if reason, err := lockAndConsume(ctx, tx, c, token, generation, now); err != nil || reason != "" {
		if err != nil {
			return LoginResult{}, err
		}
		return fail(user.ID, user.Username, reason)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE athena_users SET password_hash = ?, state = ?, failed_attempts = 0, locked_until = NULL, last_login_at = ? WHERE id = ?",
		hash, UserStateActive, now, user.ID); err != nil {
		return LoginResult{}, fmt.Errorf("complete Challenge: %w", err)
	}
	result, err := s.startSession(ctx, tx, user.ID, now, options)
	if err != nil {
		return LoginResult{}, err
	}
	if purpose == ChallengePurposeFirstLogin && c.IssuerID != "" {
		if err := notifications.Write(ctx, tx, c.IssuerID, notifications.Event{Kind: notifications.InviteAccepted, SubjectType: "user", SubjectID: user.ID, SubjectLabel: user.DisplayName, ActorID: user.ID}); err != nil {
			return LoginResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return LoginResult{}, fmt.Errorf("complete Challenge: %w", err)
	}
	s.LogAccess(ctx, AccessEvent{Event: EventChallengeComplete, Outcome: OutcomeOK, UserID: user.ID, Username: user.Username, RequestID: requestID, Detail: purpose + " completed"})
	return result, nil
}

// lockAndConsume re-reads the Challenge row under lock, validates it once
// more and consumes it. It returns a refusal reason, or "" on success. The
// row lock serialises concurrent completions; the conditional consume is the
// final guard.
func lockAndConsume(ctx context.Context, tx *sql.Tx, c Challenge, token string, generation int, now time.Time) (string, error) {
	locked, err := challengeByID(ctx, tx, c.ID, true)
	if errors.Is(err, errChallengeConsumed) {
		return "consumed", nil
	}
	if err != nil {
		return "", err
	}
	if reason, _ := validateChallenge(locked, token, generation, now); reason != "" {
		return reason, nil
	}
	consumed, err := consumeChallenge(ctx, tx, c.ID, now)
	if err != nil {
		return "", err
	}
	if !consumed {
		return "consumed", nil
	}
	return "", nil
}

// IssueActivationCode stores a short activation code for an active user and
// returns it to show once. A later call supersedes the earlier code.
func (s *Service) IssueActivationCode(ctx context.Context, username, requestID string) (User, string, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return User{}, "", fmt.Errorf("issue activation code: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var user User
	var generation int
	err = tx.QueryRowContext(ctx, "SELECT id, username, display_name, state, generation FROM athena_users WHERE username = ? FOR UPDATE",
		strings.ToLower(strings.TrimSpace(username))).Scan(&user.ID, &user.Username, &user.DisplayName, &user.State, &generation)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return User{}, "", fmt.Errorf("%w: user %q", ErrNotFound, username)
	case err != nil:
		return User{}, "", fmt.Errorf("issue activation code: %w", err)
	}
	if user.State != UserStateActive {
		return User{}, "", fmt.Errorf("%w: only an active user can receive an activation code", ErrInvalid)
	}
	generation++
	if _, err := tx.ExecContext(ctx, "UPDATE athena_users SET generation = ? WHERE id = ?", generation, user.ID); err != nil {
		return User{}, "", fmt.Errorf("issue activation code: %w", err)
	}
	_, code, err := IssueChallenge(ctx, tx, user.ID, ChallengePurposeActivation, generation, s.now())
	if err != nil {
		return User{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return User{}, "", fmt.Errorf("issue activation code: %w", err)
	}
	s.LogAccess(ctx, AccessEvent{Event: EventChallengeIssued, Outcome: OutcomeOK, UserID: user.ID, Username: user.Username, RequestID: requestID, Detail: "activation"})
	return user, code, nil
}

// VerifyActivationCode consumes the user's activation code. Unlike
// CompleteChallenge it sets no password and starts no session; the caller
// decides what the proven possession authorises.
func (s *Service) VerifyActivationCode(ctx context.Context, username, code, requestID string) (User, error) {
	name := strings.ToLower(strings.TrimSpace(username))
	now := s.now()
	fail := func(userID, username, reason string) (User, error) {
		s.LogAccess(ctx, AccessEvent{Event: EventChallengeFailed, Outcome: OutcomeDenied, UserID: userID, Username: username, RequestID: requestID, Detail: "activation " + reason})
		return User{}, ErrChallengeRefused
	}
	var user User
	var generation int
	err := s.DB.QueryRowContext(ctx, "SELECT id, username, display_name, state, generation FROM athena_users WHERE username = ?", name).
		Scan(&user.ID, &user.Username, &user.DisplayName, &user.State, &generation)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return fail("", "", "unknown user")
	case err != nil:
		return User{}, fmt.Errorf("verify activation code: %w", err)
	}
	if user.State != UserStateActive {
		return fail(user.ID, user.Username, "wrong state")
	}
	c, ok, err := latestChallenge(ctx, s.DB, user.ID, ChallengePurposeActivation)
	switch {
	case errors.Is(err, errChallengeConsumed):
		return fail(user.ID, user.Username, "consumed")
	case err != nil:
		return User{}, fmt.Errorf("verify activation code: %w", err)
	case !ok:
		return fail(user.ID, user.Username, "no Challenge")
	}
	if reason, wrong := validateChallenge(c, code, generation, now); reason != "" {
		if wrong {
			if err := countAttempt(ctx, s.DB, c.ID); err != nil {
				return User{}, err
			}
		}
		return fail(user.ID, user.Username, reason)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return User{}, fmt.Errorf("verify activation code: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var state string
	if err := tx.QueryRowContext(ctx, "SELECT state, generation FROM athena_users WHERE id = ? FOR UPDATE", user.ID).Scan(&state, &generation); err != nil {
		return User{}, fmt.Errorf("verify activation code: %w", err)
	}
	if state != UserStateActive {
		return fail(user.ID, user.Username, "wrong state")
	}
	if reason, err := lockAndConsume(ctx, tx, c, code, generation, now); err != nil || reason != "" {
		if err != nil {
			return User{}, err
		}
		return fail(user.ID, user.Username, reason)
	}
	if err := tx.Commit(); err != nil {
		return User{}, fmt.Errorf("verify activation code: %w", err)
	}
	s.LogAccess(ctx, AccessEvent{Event: EventChallengeComplete, Outcome: OutcomeOK, UserID: user.ID, Username: user.Username, RequestID: requestID, Detail: "activation completed"})
	return user, nil
}

// HasUsers reports whether any account exists.
func (s *Service) HasUsers(ctx context.Context) (bool, error) {
	var exists bool
	err := s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM athena_users)").Scan(&exists)
	return exists, err
}

const userColumns = "id, username, display_name, is_owner, disabled, state, locked_until, last_login_at, invite_delivery"

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	var locked, last sql.NullTime
	if err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Owner, &u.Disabled, &u.State, &locked, &last, &u.Delivery); err != nil {
		return User{}, err
	}
	if locked.Valid {
		u.LockedUntil = &locked.Time
	}
	if last.Valid {
		u.LastLoginAt = &last.Time
	}
	return u, nil
}

// ListUsers returns every account with its Role names, by username.
func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT "+userColumns+" FROM athena_users ORDER BY username")
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	var users []User
	byID := map[string]int{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("read user: %w", err)
		}
		byID[u.ID] = len(users)
		users = append(users, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	roleRows, err := s.DB.QueryContext(ctx, "SELECT ur.user_id, r.name FROM athena_user_roles ur JOIN athena_roles r ON r.id = ur.role_id ORDER BY r.name")
	if err != nil {
		return nil, fmt.Errorf("list user roles: %w", err)
	}
	defer roleRows.Close()
	for roleRows.Next() {
		var id, name string
		if err := roleRows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("read user role: %w", err)
		}
		if i, ok := byID[id]; ok {
			users[i].Roles = append(users[i].Roles, name)
		}
	}
	return users, roleRows.Err()
}

func (s *Service) userByName(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, username, suffix string) (User, error) {
	u, err := scanUser(q.QueryRowContext(ctx, "SELECT "+userColumns+" FROM athena_users WHERE username = ? "+suffix, strings.ToLower(strings.TrimSpace(username))))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, fmt.Errorf("%w: user %q", ErrNotFound, username)
	}
	return u, err
}

// UserUpdate lists the changes UpdateUser makes. A nil pointer leaves a
// property alone.
type UserUpdate struct {
	DisplayName *string
	Disabled    *bool
	Roles       *[]string
	// Unlock clears a lockout and the failed sign-in count.
	Unlock bool
	// Delivery sets the invite delivery channel: manual, or a destination
	// alias whose invites and resets additionally enqueue an outbox
	// delivery event.
	Delivery *string
}

// UpdateUser changes an account. Disabling a user ends their sessions. It
// refuses to leave no active owner, in the same transaction as the change.
// A clash with a concurrent access change retries the transaction.
func (s *Service) UpdateUser(ctx context.Context, username string, update UserUpdate) (User, error) {
	var user User
	err := retryGuarded(func() error {
		var err error
		user, err = s.updateUserOnce(ctx, username, update)
		return err
	})
	return user, err
}

func (s *Service) updateUserOnce(ctx context.Context, username string, update UserUpdate) (User, error) {
	tx, err := beginGuardedTx(ctx, s.DB)
	if err != nil {
		return User{}, fmt.Errorf("update user: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	user, err := s.userByName(ctx, tx, username, "FOR UPDATE")
	if err != nil {
		return User{}, err
	}
	// The workspace root account is never edited or given Roles: it
	// carries no assignments (ADR-0018) and its name is fixed, so every
	// path that changes an account refuses it the same way.
	if IsRoot(user.Username) {
		if update.DisplayName != nil {
			return User{}, fmt.Errorf("%w: the Administrator account cannot be edited", ErrInvalid)
		}
		if update.Roles != nil {
			return User{}, fmt.Errorf("%w: the Administrator account cannot be given Roles", ErrInvalid)
		}
		if update.Delivery != nil {
			return User{}, fmt.Errorf("%w: the Administrator account cannot change delivery", ErrInvalid)
		}
	}
	today := s.now().Format("2006-01-02")
	guarded := (update.Disabled != nil && *update.Disabled != user.Disabled) || update.Roles != nil
	before := 0
	if guarded {
		if before, err = beginOwnershipGuard(ctx, tx, today); err != nil {
			return User{}, err
		}
	}
	if update.DisplayName != nil {
		name := strings.TrimSpace(*update.DisplayName)
		if name == "" || len(name) > 255 {
			return User{}, fmt.Errorf("%w: the display name needs 1 to 255 characters", ErrInvalid)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE athena_users SET display_name = ? WHERE id = ?", name, user.ID); err != nil {
			return User{}, fmt.Errorf("update user: %w", err)
		}
	}
	if update.Disabled != nil && *update.Disabled != user.Disabled {
		if *update.Disabled && IsRoot(user.Username) {
			return User{}, fmt.Errorf("%w: the Administrator account cannot be suspended", ErrInvalid)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE athena_users SET disabled = ? WHERE id = ?", *update.Disabled, user.ID); err != nil {
			return User{}, fmt.Errorf("update user: %w", err)
		}
		if *update.Disabled {
			if err := revokeSessions(ctx, tx, user.ID, "", s.now(), true); err != nil {
				return User{}, err
			}
		}
	}
	if update.Unlock {
		if _, err := tx.ExecContext(ctx, "UPDATE athena_users SET failed_attempts = 0, locked_until = NULL WHERE id = ?", user.ID); err != nil {
			return User{}, fmt.Errorf("update user: %w", err)
		}
	}
	if update.Delivery != nil {
		delivery := strings.TrimSpace(*update.Delivery)
		if err := checkDelivery(delivery); err != nil {
			return User{}, err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE athena_users SET invite_delivery = ? WHERE id = ?", delivery, user.ID); err != nil {
			return User{}, fmt.Errorf("update user: %w", err)
		}
	}
	if update.Roles != nil {
		actor := PrincipalFrom(ctx)
		seen := map[string]bool{}
		var unique []string
		for _, name := range *update.Roles {
			if seen[name] {
				continue
			}
			seen[name] = true
			unique = append(unique, name)
		}
		// Delegation is checked over the whole proposed diff before
		// anything is replaced: one Role beyond the actor's reach refuses
		// the entire change. Plain Assignments carry an empty Scope, so
		// only an actor with equally broad reach may grant them.
		for _, name := range unique {
			role, err := s.roleByName(ctx, tx, name)
			if err != nil {
				return User{}, err
			}
			if err := checkDelegation(actor, role, Scope{}); err != nil {
				return User{}, err
			}
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM athena_user_roles WHERE user_id = ?", user.ID); err != nil {
			return User{}, fmt.Errorf("update user roles: %w", err)
		}
		for _, name := range unique {
			if _, err := tx.ExecContext(ctx, "INSERT INTO athena_user_roles (user_id, role_id) SELECT ?, id FROM athena_roles WHERE name = ?", user.ID, name); err != nil {
				return User{}, fmt.Errorf("update user roles: %w", err)
			}
		}
		// Replacing the Roles replaces the Assignments too, with plain
		// ones: a scoped or dated Assignment of a removed Role must not
		// survive and keep granting what the administrator just revoked.
		// The assignment editors save scoped Assignments directly.
		removed, err := tx.ExecContext(ctx, "DELETE FROM athena_role_assignments WHERE user_id = ?", user.ID)
		if err != nil {
			return User{}, fmt.Errorf("update user roles: %w", err)
		}
		for _, name := range unique {
			if _, err := tx.ExecContext(ctx, "INSERT INTO athena_role_assignments (id, user_id, role_id, scope, revision) SELECT ?, ?, id, '{}', 1 FROM athena_roles WHERE name = ?", newID(), user.ID, name); err != nil {
				return User{}, fmt.Errorf("update user roles: %w", err)
			}
		}
		count, err := removed.RowsAffected()
		if err != nil {
			return User{}, err
		}
		if count > 0 || len(unique) > 0 {
			if err := revokeSessions(ctx, tx, user.ID, "", s.now(), true); err != nil {
				return User{}, err
			}
		}
		if err := syncOwnerFlag(ctx, tx, user.ID); err != nil {
			return User{}, err
		}
		if n, _ := removed.RowsAffected(); n > 0 || len(unique) > 0 {
			if err := notifyAccess(ctx, tx, user.ID); err != nil {
				return User{}, err
			}
		}
	}
	if guarded {
		if err := endOwnershipGuard(ctx, tx, today, before); err != nil {
			return User{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return User{}, fmt.Errorf("update user: %w", err)
	}
	users, err := s.ListUsers(ctx)
	if err != nil {
		return User{}, err
	}
	for _, u := range users {
		if u.ID == user.ID {
			return u, nil
		}
	}
	return User{}, ErrNotFound
}

// ChangePassword sets a new password after checking the current one. It ends
// the user's other sessions and keeps keepSession.
func (s *Service) ChangePassword(ctx context.Context, userID, current, password, keepSession string) error {
	var username string
	var hash sql.NullString
	if err := s.DB.QueryRowContext(ctx, "SELECT username, password_hash FROM athena_users WHERE id = ?", userID).Scan(&username, &hash); err != nil {
		return fmt.Errorf("%w: user", ErrNotFound)
	}
	if !hash.Valid {
		return fmt.Errorf("%w: no password is set yet; complete the invite first", ErrInvalid)
	}
	if ok, _ := VerifyPassword(current, hash.String); !ok {
		return fmt.Errorf("%w: the current password is wrong", ErrInvalid)
	}
	return s.storePassword(ctx, User{ID: userID, Username: username}, password, keepSession)
}

func (s *Service) storePassword(ctx context.Context, user User, password, keepSession string) error {
	if err := CheckPassword(user.Username, password); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set password: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// The generation bump invalidates outstanding Challenges: a Challenge
	// issued before this password must not set another one after it.
	if _, err := tx.ExecContext(ctx, "UPDATE athena_users SET password_hash = ?, failed_attempts = 0, locked_until = NULL, generation = generation + 1 WHERE id = ?", hash, user.ID); err != nil {
		return fmt.Errorf("set password: %w", err)
	}
	if err := revokeSessions(ctx, tx, user.ID, keepSession, s.now(), false); err != nil {
		return err
	}
	return tx.Commit()
}

func revokeSessions(ctx context.Context, tx *sql.Tx, userID, keep string, at time.Time, administrative bool) error {
	result, err := tx.ExecContext(ctx, "UPDATE athena_sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL AND id <> ?", at, userID, keep)
	if err != nil {
		return fmt.Errorf("end sessions: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed > 0 && administrative && ActorID(ctx) != userID {
		return notifications.Write(ctx, tx, userID, notifications.Event{Kind: notifications.SessionEnded, SubjectType: "user", SubjectID: userID, ActorID: ActorID(ctx)})
	}
	return nil
}

// LoginResult is a new session. Token goes to the browser in a cookie and is
// never stored; the database holds only its hash.
type LoginResult struct {
	Token     string
	Principal *Principal
	ExpiresAt time.Time
}

// Login checks a username and password and starts a session. Only active
// users sign in; invited, reset_required and suspended users are refused.
// Every failure returns ErrInvalidCredentials, except that the owner of the
// right password learns a suspension or lockout. It writes the sign-in
// outcome to the Access Log and locks the user after MaxFailedAttempts wrong
// passwords in a row.
func (s *Service) Login(ctx context.Context, username, password, requestID string) (LoginResult, error) {
	return s.LoginWithOptions(ctx, username, password, requestID, SessionOptions{})
}

func (s *Service) LoginWithOptions(ctx context.Context, username, password, requestID string, options SessionOptions) (LoginResult, error) {
	now := s.now()
	name := strings.ToLower(strings.TrimSpace(username))
	var user User
	var hash sql.NullString
	var locked sql.NullTime
	err := s.DB.QueryRowContext(ctx, "SELECT id, username, display_name, password_hash, disabled, state, locked_until FROM athena_users WHERE username = ?", name).
		Scan(&user.ID, &user.Username, &user.DisplayName, &hash, &user.Disabled, &user.State, &locked)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Same work as for a real user, so timing does not reveal the account.
		VerifyPassword(password, dummyHash)
		// The attempted name is not logged: it may be a mistyped password.
		s.LogAccess(ctx, AccessEvent{Event: EventLoginFailed, Outcome: OutcomeDenied, RequestID: requestID, Detail: "unknown user"})
		return LoginResult{}, ErrInvalidCredentials
	case err != nil:
		return LoginResult{}, fmt.Errorf("sign in: %w", err)
	}
	// The root account always signs in with the right password: no lockout,
	// no suspension, no state checks. A wrong password still fails without
	// locking the account.
	root := IsRoot(user.Username)
	suspended := user.Disabled || user.State == UserStateSuspended
	if !root && (suspended || (locked.Valid && locked.Time.After(now))) {
		// The real hash is checked so that only the owner of the password
		// learns the state; anyone else gets the generic failure. A user
		// with no password hash never matches.
		correct := false
		if hash.Valid {
			correct, _ = VerifyPassword(password, hash.String)
		}
		detail := "locked"
		if user.Disabled {
			detail = "disabled"
		} else if user.State == UserStateSuspended {
			detail = "suspended"
		}
		s.LogAccess(ctx, AccessEvent{Event: EventLoginFailed, Outcome: OutcomeDenied, UserID: user.ID, Username: user.Username, RequestID: requestID, Detail: detail})
		switch {
		case !correct:
			return LoginResult{}, ErrInvalidCredentials
		case suspended:
			return LoginResult{}, ErrDisabled
		}
		return LoginResult{}, &LockedError{Until: locked.Time}
	}
	if !root && user.State != UserStateActive {
		detail := "not active"
		if user.State == UserStateInvited {
			detail = "invited"
		} else if user.State == UserStateResetRequired {
			detail = "reset required"
		}
		s.LogAccess(ctx, AccessEvent{Event: EventLoginFailed, Outcome: OutcomeDenied, UserID: user.ID, Username: user.Username, RequestID: requestID, Detail: detail})
		return LoginResult{}, ErrInvalidCredentials
	}
	ok, rehash := VerifyPassword(password, hash.String)
	if !ok {
		if err := s.countFailedLogin(ctx, user, now, requestID); err != nil {
			return LoginResult{}, err
		}
		return LoginResult{}, ErrInvalidCredentials
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return LoginResult{}, err
	}
	defer tx.Rollback()
	// Bind the verified password and account state to session creation. A
	// concurrent reset or suspension must win before a new cookie is issued.
	var current sql.NullString
	var disabled bool
	var state string
	if err := tx.QueryRowContext(ctx, "SELECT password_hash,disabled,state FROM athena_users WHERE id=? FOR UPDATE", user.ID).Scan(&current, &disabled, &state); err != nil {
		return LoginResult{}, err
	}
	if current != hash || (!root && (disabled || state != UserStateActive)) {
		return LoginResult{}, ErrInvalidCredentials
	}
	if rehash {
		if newHash, err := HashPassword(password); err == nil {
			if _, err := tx.ExecContext(ctx, "UPDATE athena_users SET password_hash=? WHERE id=?", newHash, user.ID); err != nil {
				return LoginResult{}, err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE athena_users SET failed_attempts=0,locked_until=NULL,last_login_at=? WHERE id=?", now, user.ID); err != nil {
		return LoginResult{}, err
	}
	result, err := s.startSession(ctx, tx, user.ID, now, options)
	if err != nil {
		return LoginResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return LoginResult{}, err
	}
	s.LogAccess(ctx, AccessEvent{Event: EventLogin, Outcome: OutcomeOK, UserID: user.ID, Username: user.Username, RequestID: requestID})
	return result, nil
}

func newSessionToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CSRFToken derives the token a client must send in X-CSRF-Token from the
// session token. Only a page that learned it from the server can send it: the
// session cookie is HttpOnly, so another site cannot compute it.
func CSRFToken(sessionToken string) string {
	mac := hmac.New(sha256.New, []byte(sessionToken))
	mac.Write([]byte("athena-csrf-v1"))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// CheckCSRF compares a token sent by a client with the one derived from the
// session token, in constant time.
func CheckCSRF(sessionToken, sent string) bool {
	return hmac.Equal([]byte(CSRFToken(sessionToken)), []byte(sent))
}

// Authenticate resolves a session token to the signed-in principal. It
// returns ErrUnauthenticated for a missing, expired, revoked or disabled
// session.
func (s *Service) Authenticate(ctx context.Context, token string) (*Principal, error) {
	return s.authenticate(ctx, token, false)
}

// AuthenticatePassive checks the session exactly like Authenticate but does
// not move last_seen_at. Background polls (notification counts, session
// reads) use it so an unattended open tab cannot keep itself signed in
// forever; only activity the user chose extends idle.
func (s *Service) AuthenticatePassive(ctx context.Context, token string) (*Principal, error) {
	return s.authenticate(ctx, token, true)
}

func (s *Service) authenticate(ctx context.Context, token string, passive bool) (*Principal, error) {
	if token == "" {
		return nil, &SessionEnded{"revoked"}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id, userID string
	var lastSeen, expires time.Time
	var step, revoked, boundary sql.NullTime
	var idle int
	var disabled, remembered bool
	var rateID sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT s.id,s.user_id,s.last_seen_at,s.expires_at,s.step_up_at,s.revoked_at,s.permissions_until,s.idle_minutes,u.disabled,s.remembered,s.rate_id
	FROM athena_sessions s JOIN athena_users u ON u.id=s.user_id WHERE s.token_hash=? FOR UPDATE`, hashToken(token)).
		Scan(&id, &userID, &lastSeen, &expires, &step, &revoked, &boundary, &idle, &disabled, &remembered, &rateID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &SessionEnded{"revoked"}
	}
	if err != nil {
		return nil, fmt.Errorf("read session: %w", err)
	}
	now := s.now()
	idleDuration := time.Duration(idle) * time.Minute
	switch {
	case disabled || revoked.Valid || !step.Valid || (boundary.Valid && !now.Before(boundary.Time)):
		return nil, &SessionEnded{"revoked"}
	case !now.Before(expires):
		return nil, &SessionEnded{"absolute"}
	case !now.Before(lastSeen.Add(idleDuration)):
		return nil, &SessionEnded{"idle"}
	}
	if !passive && now.After(lastSeen) {
		if _, err := tx.ExecContext(ctx, "UPDATE athena_sessions SET last_seen_at=? WHERE id=?", now, id); err != nil {
			return nil, err
		}
		lastSeen = now
	}
	p, err := s.readPrincipal(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	p.RateID, p.Remembered = rateID.String, remembered
	p.SessionID, p.StepUpAt, p.ExpiresAt, p.IdleDuration, p.IdleExpiresAt = id, step.Time, expires, idleDuration, lastSeen.Add(idleDuration)
	return p, tx.Commit()
}

// Logout ends a session.
func (s *Service) Logout(ctx context.Context, sessionID string) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE athena_sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL", s.now(), sessionID)
	return err
}

// Session is one live signed-in session. It never carries the session token.
type Session struct {
	ID         string
	UserID     string
	Username   string
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	Device     string
}

// ListSessions returns the user's live sessions, newest first: revoked,
// expired and idle-timed-out sessions are omitted, matching Authenticate.
func (s *Service) ListSessions(ctx context.Context, username string) ([]Session, error) {
	user, err := s.userByName(ctx, s.DB, username, "")
	if err != nil {
		return nil, err
	}
	now := s.now()
	rows, err := s.DB.QueryContext(ctx, `SELECT s.id, s.user_id, u.username, s.created_at, s.last_seen_at, s.expires_at, s.device
		FROM athena_sessions s JOIN athena_users u ON u.id = s.user_id
		WHERE s.user_id = ? AND s.revoked_at IS NULL AND s.expires_at > ? AND TIMESTAMPADD(MINUTE,s.idle_minutes,s.last_seen_at) > ?
        AND (s.permissions_until IS NULL OR s.permissions_until>?) AND u.disabled=0
		ORDER BY s.last_seen_at DESC`, user.ID, now, now, now)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var session Session
		var device sql.NullString
		if err := rows.Scan(&session.ID, &session.UserID, &session.Username, &session.CreatedAt, &session.LastSeenAt, &session.ExpiresAt, &device); err != nil {
			return nil, fmt.Errorf("read session: %w", err)
		}
		session.Device = device.String
		out = append(out, session)
	}
	return out, rows.Err()
}

// RevokeSession ends one session. Ending another user's session needs
// allowOther, which the caller grants only with access:sessions. Revoking a
// session that is already gone reports NotFound; losing a race with another
// revocation succeeds, since the session ends either way.
func (s *Service) RevokeSession(ctx context.Context, actorUserID, id string, allowOther bool) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owner string
	if err := tx.QueryRowContext(ctx, "SELECT user_id FROM athena_sessions WHERE id = ? AND revoked_at IS NULL", id).Scan(&owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: session", ErrNotFound)
		}
		return fmt.Errorf("revoke session: %w", err)
	}
	if owner != actorUserID && !allowOther {
		return fmt.Errorf("%w: only an authorised administrator ends another user's session", ErrForbidden)
	}
	result, err := tx.ExecContext(ctx, "UPDATE athena_sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL", s.now(), id)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if n, _ := result.RowsAffected(); n > 0 && owner != actorUserID {
		if err := notifications.Write(ctx, tx, owner, notifications.Event{Kind: notifications.SessionEnded, SubjectType: "session", SubjectID: id, ActorID: actorUserID}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Service) loadPrincipal(ctx context.Context, userID string) (*Principal, error) {
	return s.readPrincipal(ctx, s.DB, userID)
}

func (s *Service) readPrincipal(ctx context.Context, db execQuerier, userID string) (*Principal, error) {
	p := &Principal{UserID: userID, SystemPermissions: map[string]bool{}}
	if err := db.QueryRowContext(ctx, "SELECT username, display_name, is_owner FROM athena_users WHERE id = ?", userID).Scan(&p.Username, &p.DisplayName, &p.Owner); err != nil {
		return nil, fmt.Errorf("read user: %w", err)
	}
	p.Root = IsRoot(p.Username)
	now := s.now()
	rows, err := db.QueryContext(ctx, `SELECT a.id, a.role_id, r.name, a.scope, a.active_from, a.expires_at, a.revision, r.system_permissions, r.requires_scope
		FROM athena_role_assignments a JOIN athena_roles r ON r.id = a.role_id WHERE a.user_id = ? ORDER BY r.name, a.id`, userID)
	if err != nil {
		return nil, fmt.Errorf("read assignments: %w", err)
	}
	byID := map[string]int{}
	for rows.Next() {
		var a Assignment
		var scopePayload, permissionsPayload, requiresPayload []byte
		var from, expires sql.NullTime
		a.UserID = userID
		if err := rows.Scan(&a.ID, &a.RoleID, &a.RoleName, &scopePayload, &from, &expires, &a.Revision, &permissionsPayload, &requiresPayload); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read assignment: %w", err)
		}
		// An Assignment whose Scope or required dimensions cannot be read
		// matches nothing, so it is skipped and fails closed.
		scope, err := parseScope(scopePayload)
		if err != nil {
			continue
		}
		a.Scope = scope
		var requires []string
		if err := json.Unmarshal(requiresPayload, &requires); err != nil {
			continue
		}
		a.RequiresScope = requires
		var permissions []string
		if json.Unmarshal(permissionsPayload, &permissions) == nil {
			a.SystemPermissions = permissions
		}
		if from.Valid {
			at := from.Time
			a.ActiveFrom = &at
		}
		if expires.Valid {
			at := expires.Time
			a.ExpiresAt = &at
		}
		a.Active = AssignmentActive(a.ActiveFrom, a.ExpiresAt, now)
		if a.Active {
			for _, permission := range a.SystemPermissions {
				p.SystemPermissions[permission] = true
			}
		}
		byID[a.ID] = len(p.Assignments)
		p.Assignments = append(p.Assignments, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read assignments: %w", err)
	}
	grants, err := db.QueryContext(ctx, `SELECT a.id, g.record_type, g.actions, g.permission_level, g.row_condition, g.separate_duties
		FROM athena_grants g JOIN athena_role_assignments a ON a.role_id = g.role_id WHERE a.user_id = ? ORDER BY g.id`, userID)
	if err != nil {
		return nil, fmt.Errorf("read grants: %w", err)
	}
	defer grants.Close()
	for grants.Next() {
		var id string
		var g Grant
		var actions []byte
		var condition sql.NullString
		if err := grants.Scan(&id, &g.RecordType, &actions, &g.Level, &condition, &g.SeparateDuties); err != nil {
			return nil, fmt.Errorf("read grant: %w", err)
		}
		var list []string
		if err := json.Unmarshal(actions, &list); err != nil {
			return nil, fmt.Errorf("read grant: %w", err)
		}
		g.Actions = map[string]bool{}
		for _, action := range list {
			g.Actions[action] = true
		}
		g.Condition = condition.String
		if i, ok := byID[id]; ok {
			p.Assignments[i].Grants = append(p.Assignments[i].Grants, g)
		}
	}
	return p, grants.Err()
}

func scanGrant(row interface{ Scan(...any) error }) (Grant, error) {
	var g Grant
	var actions []byte
	var condition sql.NullString
	if err := row.Scan(&g.RecordType, &actions, &g.Level, &condition, &g.SeparateDuties); err != nil {
		return Grant{}, fmt.Errorf("read grant: %w", err)
	}
	var list []string
	if err := json.Unmarshal(actions, &list); err != nil {
		return Grant{}, fmt.Errorf("read grant: %w", err)
	}
	g.Actions = map[string]bool{}
	for _, action := range list {
		g.Actions[action] = true
	}
	g.Condition = condition.String
	return g, nil
}

// ListRoles returns every Role with its Grants, by name.
func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id, name, description, system_permissions, requires_scope, system FROM athena_roles ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	var roles []Role
	ids := map[string]int{}
	for rows.Next() {
		var id string
		var r Role
		var payload, requires []byte
		if err := rows.Scan(&id, &r.Name, &r.Description, &payload, &requires, &r.System); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read role: %w", err)
		}
		r.ID = id
		_ = json.Unmarshal(payload, &r.SystemPermissions)
		_ = json.Unmarshal(requires, &r.RequiresScope)
		ids[id] = len(roles)
		roles = append(roles, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	grants, err := s.DB.QueryContext(ctx, "SELECT role_id, record_type, actions, permission_level, row_condition, separate_duties FROM athena_grants ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list grants: %w", err)
	}
	defer grants.Close()
	for grants.Next() {
		var roleID string
		var g Grant
		var actions []byte
		var condition sql.NullString
		if err := grants.Scan(&roleID, &g.RecordType, &actions, &g.Level, &condition, &g.SeparateDuties); err != nil {
			return nil, fmt.Errorf("read grant: %w", err)
		}
		var list []string
		_ = json.Unmarshal(actions, &list)
		g.Actions = map[string]bool{}
		for _, action := range list {
			g.Actions[action] = true
		}
		g.Condition = condition.String
		if i, ok := ids[roleID]; ok {
			roles[i].Grants = append(roles[i].Grants, g)
		}
	}
	return roles, grants.Err()
}

var rolePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,98}$`)

// ValidateRole checks the parts of a Role that do not need Record Type
// knowledge: names, action names and levels.
func ValidateRole(r Role) error {
	if !rolePattern.MatchString(r.Name) {
		return fmt.Errorf("%w: a Role name has 1 to 99 letters, digits, spaces, dots, dashes and underscores", ErrInvalid)
	}
	for _, permission := range r.SystemPermissions {
		if !contains(SystemPermissions, permission) {
			return fmt.Errorf("%w: unknown permission %q", ErrInvalid, permission)
		}
	}
	for _, dimension := range r.RequiresScope {
		if !dimensionPattern.MatchString(dimension) {
			return fmt.Errorf("%w: required Scope dimension %q is not a field key", ErrInvalid, dimension)
		}
	}
	for _, g := range r.Grants {
		if g.RecordType == "" {
			return fmt.Errorf("%w: a Grant needs a Record Type", ErrInvalid)
		}
		if len(g.Actions) == 0 {
			return fmt.Errorf("%w: a Grant for %s allows no action", ErrInvalid, g.RecordType)
		}
		for action := range g.Actions {
			if !contains(Actions, action) {
				return fmt.Errorf("%w: unknown action %q", ErrInvalid, action)
			}
		}
		if g.Level < 0 || g.Level > MaxPermissionLevel {
			return fmt.Errorf("%w: a permission level is 0 to %d", ErrInvalid, MaxPermissionLevel)
		}
		if g.SeparateDuties && !g.Actions[ActionSubmit] {
			return fmt.Errorf("%w: separate duties applies to the submit action", ErrInvalid)
		}
		if g.RecordType == AllRecordTypes && g.Condition != "" {
			return fmt.Errorf("%w: a Grant for every Record Type cannot have a row condition", ErrInvalid)
		}
	}
	return nil
}

func contains(list []string, item string) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}

// ownerHolders lists the Assignments holding ownership that counts: an
// enabled, unsuspended user whose Assignment is inside its dates and whose
// Role grants the ownership permission. Invited and resetting owners count:
// they complete without another administrator's help, so suspending the last
// of them would strand the Workspace.
const ownerHolders = `SELECT %s FROM athena_role_assignments a
	JOIN athena_roles r ON r.id = a.role_id
	JOIN athena_users u ON u.id = a.user_id
	WHERE u.disabled = 0 AND u.state != 'suspended'
	AND (a.active_from IS NULL OR a.active_from <= ?)
	AND (a.expires_at IS NULL OR a.expires_at >= ?)
	AND JSON_CONTAINS(r.system_permissions, '"access:ownership"')`

// beginOwnershipGuard locks every ownership-holding Assignment and returns
// how many owners hold usable ownership now. Call it before mutating; the
// lock serialises concurrent demotions so two of them cannot both pass.
//
// The holder set is resolved first without locks, then locked by ID from the
// assignments table alone, in ID order. Locking through the join would lock
// user rows too, in an order that deadlocks against the user row another
// demotion already holds. A grant landing between resolve and lock only adds
// an owner, which the recount sees; demotions only touch pre-existing rows,
// so locking the resolved set still serialises them.
func beginOwnershipGuard(ctx context.Context, tx *sql.Tx, today string) (int, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(ownerHolders, "a.id"), today, today)
	if err != nil {
		return 0, fmt.Errorf("guard ownership: %w", err)
	}
	var ids []any
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("guard ownership: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("guard ownership: %w", err)
	}
	if len(ids) > 0 {
		placeholders := strings.Repeat("?,", len(ids))
		locked, err := tx.QueryContext(ctx, "SELECT id FROM athena_role_assignments WHERE id IN ("+placeholders[:len(placeholders)-1]+") ORDER BY id FOR UPDATE", ids...)
		if err != nil {
			return 0, fmt.Errorf("guard ownership: %w", err)
		}
		for locked.Next() {
			var id string
			if err := locked.Scan(&id); err != nil {
				locked.Close()
				return 0, fmt.Errorf("guard ownership: %w", err)
			}
		}
		locked.Close()
		if err := locked.Err(); err != nil {
			return 0, fmt.Errorf("guard ownership: %w", err)
		}
	}
	var count int
	if err := tx.QueryRowContext(ctx, fmt.Sprintf(ownerHolders, "COUNT(DISTINCT u.id)"), today, today).Scan(&count); err != nil {
		return 0, fmt.Errorf("guard ownership: %w", err)
	}
	return count, nil
}

// endOwnershipGuard recounts the owners after a change and refuses to leave
// none. When no owner held ownership before, there is nothing to preserve:
// the first owner is still being bootstrapped.
func endOwnershipGuard(ctx context.Context, tx *sql.Tx, today string, before int) error {
	if before == 0 {
		return nil
	}
	var after int
	if err := tx.QueryRowContext(ctx, fmt.Sprintf(ownerHolders, "COUNT(DISTINCT u.id)"), today, today).Scan(&after); err != nil {
		return fmt.Errorf("guard ownership: %w", err)
	}
	if after == 0 {
		return ErrLastOwner
	}
	return nil
}

// beginGuardedTx starts a transaction for a guarded access change at READ
// COMMITTED: every statement sees the latest committed data, so the recount
// at the end observes demotions that committed while this transaction waited
// on the ownership locks. Under REPEATABLE READ the recount would read the
// transaction snapshot, miss a concurrent demotion, and let two demotions of
// the last owners both pass.
func beginGuardedTx(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
	return db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

// isLockRetryable reports a transient locking failure: InnoDB error 1213
// (deadlock) or 1020 (a row changed under a locking read after a concurrent
// commit). Both clear when the transaction runs again.
func isLockRetryable(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && (me.Number == 1213 || me.Number == 1020)
}

// guardAttempts bounds transaction retries after transient lock failures.
const guardAttempts = 3

// retryGuarded runs fn until it succeeds, fails permanently, or exhausts its
// attempts. fn must keep every write in one transaction so a rollback
// discards a failed attempt whole.
func retryGuarded(fn func() error) error {
	var err error
	for attempt := 0; attempt < guardAttempts; attempt++ {
		if err = fn(); err == nil || !isLockRetryable(err) {
			return err
		}
	}
	return err
}

// syncOwnerFlag marks the user flagged exactly when they hold an ownership
// assignment. Every assignment write calls it in the same transaction, so a
// revocation sticks: without it the seeding backfill below would resurrect a
// deliberately revoked assignment on the next owner creation or startup.
func syncOwnerFlag(ctx context.Context, tx *sql.Tx, userID string) error {
	// The root marker is never derived: the account is flagged at setup
	// and holds no assignments by design.
	if _, err := tx.ExecContext(ctx, `UPDATE athena_users u SET is_owner = EXISTS(
		SELECT 1 FROM athena_role_assignments a JOIN athena_roles r ON r.id = a.role_id
		WHERE a.user_id = u.id AND JSON_CONTAINS(r.system_permissions, '"access:ownership"'))
		WHERE u.id = ? AND u.username <> ?`, userID, RootUsername); err != nil {
		return fmt.Errorf("sync owner marker: %w", err)
	}
	return nil
}

// syncOwnerFlagsForRole re-marks every user holding the role. Call it after
// the role's permissions changed, or after a deleted role's assignments
// cascaded away (with the holder IDs captured before the delete).
func syncOwnerFlagsForRole(ctx context.Context, tx *sql.Tx, userIDs []string) error {
	for _, userID := range userIDs {
		if err := syncOwnerFlag(ctx, tx, userID); err != nil {
			return err
		}
	}
	return nil
}

func assignmentHolders(ctx context.Context, tx *sql.Tx, roleID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT DISTINCT user_id FROM athena_role_assignments WHERE role_id = ?", roleID)
	if err != nil {
		return nil, fmt.Errorf("read assignment holders: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, fmt.Errorf("read assignment holders: %w", err)
		}
		out = append(out, userID)
	}
	return out, rows.Err()
}

// systemRoleSeeds are the Roles the core ships: the three platform Roles
// with their system permissions, and the business Role templates with no
// Grants on Record Types that do not yet exist.
var systemRoleSeeds = []struct {
	name        string
	description string
	permissions []string
	system      bool
}{
	{RolePlatformAdministrator, "Operates the core: build, rules, files and ledger setup. Holds no business grants.",
		[]string{PermissionBuild, PermissionFilesRead, PermissionFilesWrite, PermissionRulesRead, PermissionRulesWrite, PermissionLedgerRead, PermissionLedgerWrite}, true},
	{RoleInstanceOwner, "Owns the Workspace: ownership, users, assignments and recovery. Holds no business grants.",
		[]string{PermissionAccessOwnership, PermissionAccessUsers, PermissionAccessAssign, PermissionAccessRecover}, true},
	{RoleSystemManager, "Administers users, roles, assignments and sessions.",
		[]string{PermissionAccessUsers, PermissionAccessRoles, PermissionAccessAssign, PermissionAccessSessions}, true},
	{"Receiving Clerk", "Records receipts. Gains Grants when Packs install.", nil, false},
	{"Purchase User", "Drafts purchase orders. Gains Grants when Packs install.", nil, false},
	{"Accounts Payable Clerk", "Enters bills. Gains Grants when Packs install.", nil, false},
	{"Treasury Officer", "Releases approved payments. Gains Grants when Packs install.", nil, false},
	{"Accountant", "Reconciles. Gains Grants when Packs install.", nil, false},
	{"Auditor", "Reads scoped evidence. Gains Grants when Packs install.", nil, false},
	{"Report Viewer", "Runs approved reports. Gains Grants when Packs install.", nil, false},
}

// EnsureSystemRoles seeds the platform Roles and business Role templates,
// and gives the Instance Owner Role to flagged owners that lack it: users
// flagged before the Role existed keep working after the owner shortcut
// leaves. It is idempotent and safe to run at every startup.
func (s *Service) EnsureSystemRoles(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("seed system roles: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := ensureSystemRolesTx(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

// EnsureRoot creates the workspace root Administrator on first setup and
// returns "". When the database password cannot serve as the credential —
// empty or failing the password policy — it sets a random one instead and
// returns it to show once. Afterwards the credential is ordinary: the root
// rotates it with change password, and database rotation does not touch it.
// A username taken by a non-root account is left alone with a loud note,
// never claimed.
func (s *Service) EnsureRoot(ctx context.Context, dbPassword string) (string, error) {
	var delivery string
	switch err := s.DB.QueryRowContext(ctx, "SELECT invite_delivery FROM athena_users WHERE username = ?", RootUsername).Scan(&delivery); {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return "", fmt.Errorf("seed root: %w", err)
	case delivery == DeliverySetup:
		return "", nil
	default:
		log.Printf("seed root: the username %q is taken by a regular user; not claiming it", RootUsername)
		return "", nil
	}
	password := dbPassword
	generated := ""
	if err := CheckPassword(RootUsername, password); err != nil {
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			return "", fmt.Errorf("seed root: %w", err)
		}
		password = base64.RawURLEncoding.EncodeToString(raw)
		generated = password
	}
	hash, err := HashPassword(password)
	if err != nil {
		return "", err
	}
	if _, err := s.DB.ExecContext(ctx, "INSERT INTO athena_users (id, username, display_name, password_hash, is_owner, state, generation, invite_delivery) VALUES (?,?,?,?,?,?,?,?)",
		newID(), RootUsername, "Administrator", hash, true, UserStateActive, 0, "setup"); err != nil {
		// Another setup won the race; the account exists either way.
		if isDuplicate(err) {
			return "", nil
		}
		return "", fmt.Errorf("seed root: %w", err)
	}
	s.LogAccess(ctx, AccessEvent{Event: EventUserCreated, Outcome: OutcomeOK, Username: RootUsername, Detail: "administrator (setup)"})
	return generated, nil
}

func ensureSystemRolesTx(ctx context.Context, tx *sql.Tx) error {
	for _, seed := range systemRoleSeeds {
		permissions, _ := json.Marshal(append([]string{}, seed.permissions...))
		if seed.permissions == nil {
			permissions = []byte("[]")
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO athena_roles (id, name, description, system_permissions, requires_scope, system) VALUES (?,?,?,?,?,?)",
			newID(), seed.name, seed.description, permissions, "[]", seed.system)
		if err != nil && !isDuplicate(err) {
			return fmt.Errorf("seed system roles: %w", err)
		}
	}
	// The backfill only ever matches migration-era owners: flagged before
	// the Role existed and never assigned since. Every assignment write
	// keeps the marker in step, so a deliberately revoked owner is
	// unflagged and never resurrected here. The root account is excluded:
	// it is flagged but needs no assignment, and assigning one would make
	// the last-owner guard count it.
	_, err := tx.ExecContext(ctx, `INSERT INTO athena_role_assignments (id, user_id, role_id, scope, revision)
		SELECT UUID(), u.id, r.id, '{}', 1 FROM athena_users u JOIN athena_roles r ON r.name = ?
		WHERE u.is_owner = 1 AND u.username <> ? AND NOT EXISTS (SELECT 1 FROM athena_role_assignments a JOIN athena_roles r2 ON r2.id = a.role_id
			WHERE a.user_id = u.id AND JSON_CONTAINS(r2.system_permissions, '"access:ownership"'))`, RoleInstanceOwner, RootUsername)
	if err != nil {
		return fmt.Errorf("seed owner assignments: %w", err)
	}
	return nil
}

// checkDelegation refuses a Role or Assignment change beyond the actor's own
// delegation. A nil actor is the operator and may do anything. scope is the
// proposed Assignment Scope, or nil when saving a Role: the proposed Grants
// must narrow what the actor already holds, never broaden it.
func checkDelegation(actor *Principal, role Role, scope Scope) error {
	if actor == nil {
		return nil
	}
	// Ownership is full administrative delegation: the last-owner guard,
	// not this check, constrains owners. Managers stay within their reach.
	if actor.HasSystem(PermissionAccessOwnership) {
		return nil
	}
	denied := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrForbidden, fmt.Sprintf(format, args...))
	}
	if role.System || contains(role.SystemPermissions, PermissionAccessOwnership) {
		if !actor.HasSystem(PermissionAccessOwnership) {
			return denied("only an Instance Owner assigns the %s role", role.Name)
		}
	}
	for _, permission := range role.SystemPermissions {
		if !actor.HasSystem(permission) {
			return denied("the %s permission is beyond your delegation", permission)
		}
	}
	for _, g := range role.Grants {
		if !actor.delegates(g, scope) {
			return denied("the Grant for %s is beyond your delegation", g.RecordType)
		}
	}
	return nil
}

// delegates reports whether the principal's usable Assignments cover the
// proposed Grant within the proposed Scope. The covering Grant must allow at
// least the actions at at least the level; an unconditioned Grant needs an
// unconditioned covering Grant, while a conditioned one needs an
// unconditioned or identical covering condition. The proposed Scope must
// narrow the covering Assignment's Scope.
func (p *Principal) delegates(g Grant, proposed Scope) bool {
	for _, a := range p.Assignments {
		if !a.usable() {
			continue
		}
		for _, h := range a.Grants {
			if !h.covers(g.RecordType) || h.Level < g.Level {
				continue
			}
			covered := true
			for action := range g.Actions {
				if !h.Actions[action] {
					covered = false
					break
				}
			}
			if !covered {
				continue
			}
			if g.Condition == "" {
				if h.Condition != "" {
					continue
				}
			} else if h.Condition != "" && h.Condition != g.Condition {
				continue
			}
			if proposed != nil && !scopeSubset(proposed, a.Scope) {
				continue
			}
			return true
		}
	}
	return false
}

// scopeSubset reports whether every row in proposed is also in covering: the
// covering Scope's dimensions must all be constrained by the proposed Scope
// to values it allows. An empty covering Scope covers everything; an empty
// proposed Scope only fits an empty covering one.
func scopeSubset(proposed, covering Scope) bool {
	for dimension, values := range covering {
		narrowed, ok := proposed[dimension]
		if !ok {
			return false
		}
		for _, value := range narrowed {
			if !contains(values, value) {
				return false
			}
		}
	}
	return true
}

// SaveRole creates or replaces the Role with r.Name, with all its Grants.
// It refuses Grants beyond the actor's delegation and a change that would
// leave no active owner. The system flag is never written here: seeding
// owns it.
func (s *Service) SaveRole(ctx context.Context, r Role) error {
	return retryGuarded(func() error { return s.saveRoleOnce(ctx, r) })
}

func (s *Service) saveRoleOnce(ctx context.Context, r Role) error {
	if err := ValidateRole(r); err != nil {
		return err
	}
	if err := checkDelegation(PrincipalFrom(ctx), r, nil); err != nil {
		return err
	}
	sort.Strings(r.SystemPermissions)
	permissions, _ := json.Marshal(append([]string{}, r.SystemPermissions...))
	tx, err := beginGuardedTx(ctx, s.DB)
	if err != nil {
		return fmt.Errorf("save role: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	today := s.now().Format("2006-01-02")
	before, err := beginOwnershipGuard(ctx, tx, today)
	if err != nil {
		return err
	}
	var id string
	var system bool
	switch err := tx.QueryRowContext(ctx, "SELECT id, system FROM athena_roles WHERE name = ? FOR UPDATE", r.Name).Scan(&id, &system); {
	case errors.Is(err, sql.ErrNoRows):
		id = newID()
		if _, err := tx.ExecContext(ctx, "INSERT INTO athena_roles (id, name, description, system_permissions, requires_scope) VALUES (?,?,?,?,?)", id, r.Name, r.Description, permissions, "[]"); err != nil {
			return fmt.Errorf("save role: %w", err)
		}
	case err != nil:
		return fmt.Errorf("save role: %w", err)
	default:
		// Platform Roles change only for Instance Owners. The check reads
		// the stored flag: the API carries no system field, so a forged
		// Role cannot clear it.
		if system {
			if actor := PrincipalFrom(ctx); actor != nil && !actor.HasSystem(PermissionAccessOwnership) {
				return fmt.Errorf("%w: only an Instance Owner changes the %s role", ErrForbidden, r.Name)
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE athena_roles SET description = ?, system_permissions = ? WHERE id = ?", r.Description, permissions, id); err != nil {
			return fmt.Errorf("save role: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM athena_grants WHERE role_id = ?", id); err != nil {
			return fmt.Errorf("save role: %w", err)
		}
	}
	for _, g := range r.Grants {
		actions := make([]string, 0, len(g.Actions))
		for _, action := range Actions {
			if g.Actions[action] {
				actions = append(actions, action)
			}
		}
		payload, _ := json.Marshal(actions)
		var condition any
		if g.Condition != "" {
			condition = g.Condition
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO athena_grants (id, role_id, record_type, actions, permission_level, row_condition, separate_duties) VALUES (?,?,?,?,?,?,?)",
			newID(), id, g.RecordType, payload, g.Level, condition, g.SeparateDuties); err != nil {
			return fmt.Errorf("save role: %w", err)
		}
	}
	holders, err := assignmentHolders(ctx, tx, id)
	if err != nil {
		return err
	}
	for _, holder := range holders {
		if err := notifyAccess(ctx, tx, holder); err != nil {
			return err
		}
		if err := revokeSessions(ctx, tx, holder, "", s.now(), true); err != nil {
			return err
		}
	}
	if err := syncOwnerFlagsForRole(ctx, tx, holders); err != nil {
		return err
	}
	if err := endOwnershipGuard(ctx, tx, today, before); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteRole removes a Role. Users who held it lose its permissions. It
// refuses to leave no active owner.
func (s *Service) DeleteRole(ctx context.Context, name string) error {
	return retryGuarded(func() error { return s.deleteRoleOnce(ctx, name) })
}

func (s *Service) deleteRoleOnce(ctx context.Context, name string) error {
	tx, err := beginGuardedTx(ctx, s.DB)
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	today := s.now().Format("2006-01-02")
	before, err := beginOwnershipGuard(ctx, tx, today)
	if err != nil {
		return err
	}
	var id string
	if err := tx.QueryRowContext(ctx, "SELECT id FROM athena_roles WHERE name = ?", name).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: Role %q", ErrNotFound, name)
		}
		return fmt.Errorf("delete role: %w", err)
	}
	holders, err := assignmentHolders(ctx, tx, id)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM athena_roles WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	for _, holder := range holders {
		if err := notifyAccess(ctx, tx, holder); err != nil {
			return err
		}
		if err := revokeSessions(ctx, tx, holder, "", s.now(), true); err != nil {
			return err
		}
	}
	if err := syncOwnerFlagsForRole(ctx, tx, holders); err != nil {
		return err
	}
	if err := endOwnershipGuard(ctx, tx, today, before); err != nil {
		return err
	}
	return tx.Commit()
}

// LockedError is ErrLocked with the time the lock ends.
type LockedError struct{ Until time.Time }

func (e *LockedError) Error() string        { return ErrLocked.Error() }
func (e *LockedError) Is(target error) bool { return target == ErrLocked }
