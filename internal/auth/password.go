// Package auth holds Athena's users, password hashing, database-backed
// sessions, Roles with Grants, and the Access Log (spec 21, ADR-0014).
//
// Passwords are hashed with Argon2id from golang.org/x/crypto/argon2, the
// reference implementation maintained by the Go project; the standard library
// has no Argon2. Tokens and passwords never leave this package in a log line,
// a span attribute or an error message.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Params are the Argon2id cost settings. The defaults follow the second
// recommended option of RFC 9106 section 4 (64 MiB, 3 passes, 4 lanes). Tests
// lower them. Every hash stores the settings it was made with, so changing
// them later still verifies old hashes and marks them for a rehash.
type Params struct {
	MemoryKiB uint32
	Time      uint32
	Threads   uint8
}

// DefaultParams is the cost used for new hashes.
var DefaultParams = Params{MemoryKiB: 64 * 1024, Time: 3, Threads: 4}

const (
	saltLength = 16
	keyLength  = 32
	// Upper bounds on the cost read back from a stored hash, so a corrupted
	// or hostile row cannot make one login allocate gigabytes.
	maxMemoryKiB = 256 * 1024
	maxTime      = 10
	maxThreads   = 16

	// MinPasswordLength and MaxPasswordLength bound a new password in
	// characters. The upper bound keeps hashing time and memory bounded.
	MinPasswordLength = 12
	MaxPasswordLength = 256
)

// hashSlots bounds how many Argon2id computations run at once, so a burst of
// sign-in attempts cannot exhaust memory (each one holds MemoryKiB).
var hashSlots = make(chan struct{}, 4)

func acquireSlot() func() {
	hashSlots <- struct{}{}
	return func() { <-hashSlots }
}

// ErrPasswordPolicy is returned for a password that is too short or too long.
var ErrPasswordPolicy = fmt.Errorf("a password needs %d to %d characters", MinPasswordLength, MaxPasswordLength)

// CheckPassword applies the password policy for the user with the given name.
func CheckPassword(username, password string) error {
	n := utf8.RuneCountInString(password)
	if n < MinPasswordLength || n > MaxPasswordLength {
		return ErrPasswordPolicy
	}
	if strings.EqualFold(password, username) {
		return errors.New("the password may not be the username")
	}
	return nil
}

// HashPassword returns the PHC string of password: $argon2id$v=19$m=..,t=..,p=..$salt$hash.
func HashPassword(password string) (string, error) {
	return hashWith(password, DefaultParams)
}

func hashWith(password string, p Params) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	release := acquireSlot()
	defer release()
	key := argon2.IDKey([]byte(password), salt, p.Time, p.MemoryKiB, p.Threads, keyLength)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, p.MemoryKiB, p.Time, p.Threads, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

type parsedHash struct {
	params Params
	salt   []byte
	key    []byte
}

func parseHash(encoded string) (parsedHash, error) {
	var out parsedHash
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return out, errors.New("not an argon2id hash")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return out, errors.New("unsupported argon2 version")
	}
	var threads uint
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &out.params.MemoryKiB, &out.params.Time, &threads); err != nil {
		return out, errors.New("unreadable argon2 parameters")
	}
	if out.params.MemoryKiB == 0 || out.params.MemoryKiB > maxMemoryKiB || out.params.Time == 0 || out.params.Time > maxTime || threads == 0 || threads > maxThreads {
		return out, errors.New("argon2 parameters out of range")
	}
	out.params.Threads = uint8(threads)
	var err error
	enc := base64.RawStdEncoding
	if out.salt, err = enc.DecodeString(parts[4]); err != nil || len(out.salt) == 0 {
		return out, errors.New("unreadable salt")
	}
	if out.key, err = enc.DecodeString(parts[5]); err != nil || len(out.key) == 0 {
		return out, errors.New("unreadable hash")
	}
	return out, nil
}

// VerifyPassword reports whether password matches the stored hash, and
// whether the hash was made with weaker settings than DefaultParams (the
// caller then stores a new hash while it has the password). A malformed hash
// never matches.
func VerifyPassword(password, encoded string) (match, needsRehash bool) {
	h, err := parseHash(encoded)
	if err != nil {
		return false, false
	}
	release := acquireSlot()
	key := argon2.IDKey([]byte(password), h.salt, h.params.Time, h.params.MemoryKiB, h.params.Threads, uint32(len(h.key)))
	release()
	if subtle.ConstantTimeCompare(key, h.key) != 1 {
		return false, false
	}
	return true, h.params != DefaultParams
}

// dummyHash is verified when a username does not exist, so a sign-in for an
// unknown user takes about as long as one for a known user.
var dummyHash = func() string {
	h, err := hashWith("athena-dummy-password", DefaultParams)
	if err != nil {
		panic(err)
	}
	return h
}()
