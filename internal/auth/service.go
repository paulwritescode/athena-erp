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
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

// Sign-in limits (spec 21). A user is locked for LockDuration after
// MaxFailedAttempts wrong passwords in a row. A session ends after IdleTimeout
// without a request and after AbsoluteLifetime whatever its use.
const (
	MaxFailedAttempts = 5
	LockDuration      = 15 * time.Minute
	IdleTimeout       = 8 * time.Hour
	AbsoluteLifetime  = 7 * 24 * time.Hour
	// touchInterval limits how often a busy session writes its last-seen time.
	touchInterval = time.Minute
)

// Errors the callers map to API codes.
var (
	// ErrInvalidCredentials covers a wrong password, an unknown or disabled
	// user and a locked user, so a sign-in reveals nothing about which.
	ErrInvalidCredentials = errors.New("wrong username or password, or the user is locked")
	ErrUnauthenticated    = errors.New("sign in to continue")
	ErrNotFound           = errors.New("not found")
	ErrExists             = errors.New("already exists")
	ErrLastOwner          = errors.New("the instance needs at least one enabled owner")
	ErrInvalid            = errors.New("invalid input")
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

// User is one account. It never carries a password hash.
type User struct {
	ID          string
	Username    string
	DisplayName string
	Owner       bool
	Disabled    bool
	LockedUntil *time.Time
	LastLoginAt *time.Time
	Roles       []string
}

// Locked reports whether the user is locked at time at.
func (u User) Locked(at time.Time) bool { return u.LockedUntil != nil && u.LockedUntil.After(at) }

// Role names a set of system permissions and Grants.
type Role struct {
	Name              string
	Description       string
	SystemPermissions []string
	Grants            []Grant
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

// CreateUser adds an account. owner makes it an instance owner.
func (s *Service) CreateUser(ctx context.Context, username, displayName, password string, owner bool) (User, error) {
	username, err := NormalizeUsername(username)
	if err != nil {
		return User{}, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = username
	}
	if len(displayName) > 255 {
		return User{}, fmt.Errorf("%w: the display name is too long", ErrInvalid)
	}
	if err := CheckPassword(username, password); err != nil {
		return User{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	id := newID()
	if _, err := s.DB.ExecContext(ctx, "INSERT INTO athena_users (id, username, display_name, password_hash, is_owner) VALUES (?,?,?,?,?)", id, username, displayName, hash, owner); err != nil {
		if isDuplicate(err) {
			return User{}, fmt.Errorf("%w: the username %q is taken", ErrExists, username)
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return User{ID: id, Username: username, DisplayName: displayName, Owner: owner}, nil
}

// HasUsers reports whether any account exists.
func (s *Service) HasUsers(ctx context.Context) (bool, error) {
	var exists bool
	err := s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM athena_users)").Scan(&exists)
	return exists, err
}

const userColumns = "id, username, display_name, is_owner, disabled, locked_until, last_login_at"

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	var locked, last sql.NullTime
	if err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Owner, &u.Disabled, &locked, &last); err != nil {
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
}

// UpdateUser changes an account. Disabling a user ends their sessions. It
// refuses to disable the last enabled owner.
func (s *Service) UpdateUser(ctx context.Context, username string, update UserUpdate) (User, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return User{}, fmt.Errorf("update user: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	user, err := s.userByName(ctx, tx, username, "FOR UPDATE")
	if err != nil {
		return User{}, err
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
		if *update.Disabled && user.Owner {
			var others int
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM athena_users WHERE is_owner = 1 AND disabled = 0 AND id <> ?", user.ID).Scan(&others); err != nil {
				return User{}, fmt.Errorf("update user: %w", err)
			}
			if others == 0 {
				return User{}, ErrLastOwner
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE athena_users SET disabled = ? WHERE id = ?", *update.Disabled, user.ID); err != nil {
			return User{}, fmt.Errorf("update user: %w", err)
		}
		if *update.Disabled {
			if err := revokeSessions(ctx, tx, user.ID, "", s.now()); err != nil {
				return User{}, err
			}
		}
	}
	if update.Unlock {
		if _, err := tx.ExecContext(ctx, "UPDATE athena_users SET failed_attempts = 0, locked_until = NULL WHERE id = ?", user.ID); err != nil {
			return User{}, fmt.Errorf("update user: %w", err)
		}
	}
	if update.Roles != nil {
		if _, err := tx.ExecContext(ctx, "DELETE FROM athena_user_roles WHERE user_id = ?", user.ID); err != nil {
			return User{}, fmt.Errorf("update user roles: %w", err)
		}
		seen := map[string]bool{}
		for _, name := range *update.Roles {
			if seen[name] {
				continue
			}
			seen[name] = true
			res, err := tx.ExecContext(ctx, "INSERT INTO athena_user_roles (user_id, role_id) SELECT ?, id FROM athena_roles WHERE name = ?", user.ID, name)
			if err != nil {
				return User{}, fmt.Errorf("update user roles: %w", err)
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return User{}, fmt.Errorf("%w: Role %q", ErrNotFound, name)
			}
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

// ResetPassword sets a new password without knowing the old one and ends the
// user's sessions. The caller has already checked the right to do so.
func (s *Service) ResetPassword(ctx context.Context, username, password string) (User, error) {
	user, err := s.userByName(ctx, s.DB, username, "")
	if err != nil {
		return User{}, err
	}
	if err := s.storePassword(ctx, user, password, ""); err != nil {
		return User{}, err
	}
	return user, nil
}

// ChangePassword sets a new password after checking the current one. It ends
// the user's other sessions and keeps keepSession.
func (s *Service) ChangePassword(ctx context.Context, userID, current, password, keepSession string) error {
	var hash, username string
	if err := s.DB.QueryRowContext(ctx, "SELECT username, password_hash FROM athena_users WHERE id = ?", userID).Scan(&username, &hash); err != nil {
		return fmt.Errorf("%w: user", ErrNotFound)
	}
	if ok, _ := VerifyPassword(current, hash); !ok {
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
	if _, err := tx.ExecContext(ctx, "UPDATE athena_users SET password_hash = ?, failed_attempts = 0, locked_until = NULL WHERE id = ?", hash, user.ID); err != nil {
		return fmt.Errorf("set password: %w", err)
	}
	if err := revokeSessions(ctx, tx, user.ID, keepSession, s.now()); err != nil {
		return err
	}
	return tx.Commit()
}

func revokeSessions(ctx context.Context, tx *sql.Tx, userID, keep string, at time.Time) error {
	if _, err := tx.ExecContext(ctx, "UPDATE athena_sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL AND id <> ?", at, userID, keep); err != nil {
		return fmt.Errorf("end sessions: %w", err)
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

// Login checks a username and password and starts a session. Every failure
// returns ErrInvalidCredentials. It writes the sign-in outcome to the Access
// Log and locks the user after MaxFailedAttempts wrong passwords in a row.
func (s *Service) Login(ctx context.Context, username, password, requestID string) (LoginResult, error) {
	now := s.now()
	name := strings.ToLower(strings.TrimSpace(username))
	var user User
	var hash string
	var attempts int
	var locked sql.NullTime
	err := s.DB.QueryRowContext(ctx, "SELECT id, username, display_name, password_hash, disabled, failed_attempts, locked_until FROM athena_users WHERE username = ?", name).
		Scan(&user.ID, &user.Username, &user.DisplayName, &hash, &user.Disabled, &attempts, &locked)
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
	if user.Disabled || (locked.Valid && locked.Time.After(now)) {
		VerifyPassword(password, dummyHash)
		detail := "disabled"
		if !user.Disabled {
			detail = "locked"
		}
		s.LogAccess(ctx, AccessEvent{Event: EventLoginFailed, Outcome: OutcomeDenied, UserID: user.ID, Username: user.Username, RequestID: requestID, Detail: detail})
		return LoginResult{}, ErrInvalidCredentials
	}
	ok, rehash := VerifyPassword(password, hash)
	if !ok {
		attempts++
		event := EventLoginFailed
		if attempts >= MaxFailedAttempts {
			if _, err := s.DB.ExecContext(ctx, "UPDATE athena_users SET failed_attempts = 0, locked_until = ? WHERE id = ?", now.Add(LockDuration), user.ID); err != nil {
				return LoginResult{}, fmt.Errorf("lock user: %w", err)
			}
			event = EventLockout
		} else if _, err := s.DB.ExecContext(ctx, "UPDATE athena_users SET failed_attempts = ? WHERE id = ?", attempts, user.ID); err != nil {
			return LoginResult{}, fmt.Errorf("count failed sign-in: %w", err)
		}
		s.LogAccess(ctx, AccessEvent{Event: event, Outcome: OutcomeDenied, UserID: user.ID, Username: user.Username, RequestID: requestID, Detail: "wrong password"})
		return LoginResult{}, ErrInvalidCredentials
	}
	if rehash {
		if newHash, err := HashPassword(password); err == nil {
			_, _ = s.DB.ExecContext(ctx, "UPDATE athena_users SET password_hash = ? WHERE id = ?", newHash, user.ID)
		}
	}
	if _, err := s.DB.ExecContext(ctx, "UPDATE athena_users SET failed_attempts = 0, locked_until = NULL, last_login_at = ? WHERE id = ?", now, user.ID); err != nil {
		return LoginResult{}, fmt.Errorf("sign in: %w", err)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return LoginResult{}, fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	sessionID, expires := newID(), now.Add(AbsoluteLifetime)
	if _, err := s.DB.ExecContext(ctx, "INSERT INTO athena_sessions (id, token_hash, user_id, created_at, last_seen_at, expires_at) VALUES (?,?,?,?,?,?)", sessionID, hashToken(token), user.ID, now, now, expires); err != nil {
		return LoginResult{}, fmt.Errorf("start session: %w", err)
	}
	principal, err := s.loadPrincipal(ctx, user.ID)
	if err != nil {
		return LoginResult{}, err
	}
	principal.SessionID = sessionID
	s.LogAccess(ctx, AccessEvent{Event: EventLogin, Outcome: OutcomeOK, UserID: user.ID, Username: user.Username, RequestID: requestID})
	return LoginResult{Token: token, Principal: principal, ExpiresAt: expires}, nil
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
	if token == "" {
		return nil, ErrUnauthenticated
	}
	now := s.now()
	var sessionID, userID string
	var lastSeen time.Time
	err := s.DB.QueryRowContext(ctx, `SELECT s.id, s.user_id, s.last_seen_at FROM athena_sessions s JOIN athena_users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.revoked_at IS NULL AND s.expires_at > ? AND s.last_seen_at > ? AND u.disabled = 0`,
		hashToken(token), now, now.Add(-IdleTimeout)).Scan(&sessionID, &userID, &lastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnauthenticated
	}
	if err != nil {
		return nil, fmt.Errorf("read session: %w", err)
	}
	if now.Sub(lastSeen) > touchInterval {
		_, _ = s.DB.ExecContext(ctx, "UPDATE athena_sessions SET last_seen_at = ? WHERE id = ?", now, sessionID)
	}
	principal, err := s.loadPrincipal(ctx, userID)
	if err != nil {
		return nil, err
	}
	principal.SessionID = sessionID
	return principal, nil
}

// Logout ends a session.
func (s *Service) Logout(ctx context.Context, sessionID string) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE athena_sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL", s.now(), sessionID)
	return err
}

func (s *Service) loadPrincipal(ctx context.Context, userID string) (*Principal, error) {
	p := &Principal{UserID: userID, SystemPermissions: map[string]bool{}}
	if err := s.DB.QueryRowContext(ctx, "SELECT username, display_name, is_owner FROM athena_users WHERE id = ?", userID).Scan(&p.Username, &p.DisplayName, &p.Owner); err != nil {
		return nil, fmt.Errorf("read user: %w", err)
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT r.system_permissions FROM athena_roles r JOIN athena_user_roles ur ON ur.role_id = r.id WHERE ur.user_id = ?", userID)
	if err != nil {
		return nil, fmt.Errorf("read roles: %w", err)
	}
	for rows.Next() {
		var payload []byte
		var permissions []string
		if err := rows.Scan(&payload); err == nil && json.Unmarshal(payload, &permissions) == nil {
			for _, permission := range permissions {
				p.SystemPermissions[permission] = true
			}
		}
	}
	rows.Close()
	grants, err := s.DB.QueryContext(ctx, `SELECT g.record_type, g.actions, g.permission_level, g.row_condition, g.separate_duties
		FROM athena_grants g JOIN athena_user_roles ur ON ur.role_id = g.role_id WHERE ur.user_id = ? ORDER BY g.id`, userID)
	if err != nil {
		return nil, fmt.Errorf("read grants: %w", err)
	}
	defer grants.Close()
	for grants.Next() {
		g, err := scanGrant(grants)
		if err != nil {
			return nil, err
		}
		p.Grants = append(p.Grants, g)
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
	rows, err := s.DB.QueryContext(ctx, "SELECT id, name, description, system_permissions FROM athena_roles ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	var roles []Role
	ids := map[string]int{}
	for rows.Next() {
		var id string
		var r Role
		var payload []byte
		if err := rows.Scan(&id, &r.Name, &r.Description, &payload); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read role: %w", err)
		}
		_ = json.Unmarshal(payload, &r.SystemPermissions)
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

// SaveRole creates or replaces the Role with r.Name, with all its Grants.
func (s *Service) SaveRole(ctx context.Context, r Role) error {
	if err := ValidateRole(r); err != nil {
		return err
	}
	sort.Strings(r.SystemPermissions)
	permissions, _ := json.Marshal(append([]string{}, r.SystemPermissions...))
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save role: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var id string
	switch err := tx.QueryRowContext(ctx, "SELECT id FROM athena_roles WHERE name = ? FOR UPDATE", r.Name).Scan(&id); {
	case errors.Is(err, sql.ErrNoRows):
		id = newID()
		if _, err := tx.ExecContext(ctx, "INSERT INTO athena_roles (id, name, description, system_permissions) VALUES (?,?,?,?)", id, r.Name, r.Description, permissions); err != nil {
			return fmt.Errorf("save role: %w", err)
		}
	case err != nil:
		return fmt.Errorf("save role: %w", err)
	default:
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
	return tx.Commit()
}

// DeleteRole removes a Role. Users who held it lose its permissions.
func (s *Service) DeleteRole(ctx context.Context, name string) error {
	res, err := s.DB.ExecContext(ctx, "DELETE FROM athena_roles WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%w: Role %q", ErrNotFound, name)
	}
	return nil
}
