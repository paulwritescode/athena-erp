package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// Key version statuses of an athena_key_versions row. Rotation adds a
// new active version and retires the old one; retired versions still
// decrypt, revoked ones refuse.
const (
	VersionActive  = "active"
	VersionRetired = "retired"
	VersionRevoked = "revoked"
)

// envelopePrefix marks a field-encryption envelope. The envelope carries
// the key name, version, nonce and ciphertext as
// athsec1:<key>:<version>:<nonce>:<ciphertext>, all printable, so it
// stores in a text field. It never carries plaintext.
const envelopePrefix = "athsec1"

// Context binds a protected value to its place: the Workspace, Record
// Type, Record ID and field it was encrypted for, plus the key name.
// Decryption rebuilds the same binding, so a value copied into another
// Record fails authentication.
type Context struct {
	Workspace  string
	RecordType string
	RecordID   string
	Field      string
	KeyName    string
}

// validate checks every bound field is present and the key name parses.
func (c Context) validate() error {
	if c.Workspace == "" || c.RecordType == "" || c.RecordID == "" || c.Field == "" {
		return fmt.Errorf("secrets: encryption needs Workspace, Record Type, Record ID and field")
	}
	return validateName("key", c.KeyName)
}

// AddVersion records version for a key as active. The version must be
// positive; re-adding an existing version is an error.
func AddVersion(ctx context.Context, q Querier, keyName string, version int64) error {
	if err := validateName("key", keyName); err != nil {
		return err
	}
	if version <= 0 {
		return fmt.Errorf("secrets: key version for %q must be positive", keyName)
	}
	id, err := newID()
	if err != nil {
		return err
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO athena_key_versions (id, key_name, version, status) VALUES (?, ?, ?, ?)`, id, keyName, version, VersionActive); err != nil {
		return fmt.Errorf("secrets: add version %d for %q: %w", version, keyName, err)
	}
	return nil
}

// ActiveVersion returns the highest active version of a key.
func ActiveVersion(ctx context.Context, q Querier, keyName string) (int64, error) {
	if err := validateName("key", keyName); err != nil {
		return 0, err
	}
	var version sql.NullInt64
	if err := q.QueryRowContext(ctx, `SELECT MAX(version) FROM athena_key_versions WHERE key_name = ? AND status = ?`, keyName, VersionActive).Scan(&version); err != nil {
		return 0, fmt.Errorf("secrets: active version for %q: %w", keyName, err)
	}
	if !version.Valid {
		return 0, fmt.Errorf("secrets: key %q has no active version", keyName)
	}
	return version.Int64, nil
}

// SetVersionStatus moves one key version between active, retired and
// revoked. Rotation retires the old version after adding the new one.
func SetVersionStatus(ctx context.Context, q Querier, keyName string, version int64, status string) error {
	if err := validateName("key", keyName); err != nil {
		return err
	}
	if status != VersionActive && status != VersionRetired && status != VersionRevoked {
		return fmt.Errorf("secrets: version status %q must be active, retired or revoked", status)
	}
	res, err := q.ExecContext(ctx, `UPDATE athena_key_versions SET status = ? WHERE key_name = ? AND version = ?`, status, keyName, version)
	if err != nil {
		return fmt.Errorf("secrets: set version %d of %q to %q: %w", version, keyName, status, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("secrets: set version %d of %q to %q: %w", version, keyName, status, err)
	}
	if n == 0 {
		return fmt.Errorf("secrets: version %d of %q not found", version, keyName)
	}
	return nil
}

// versionStatus loads the status of one key version.
func versionStatus(ctx context.Context, q Querier, keyName string, version int64) (string, error) {
	var status string
	if err := q.QueryRowContext(ctx, `SELECT status FROM athena_key_versions WHERE key_name = ? AND version = ?`, keyName, version).Scan(&status); err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("secrets: version %d of %q not found", version, keyName)
		}
		return "", fmt.Errorf("secrets: load version %d of %q: %w", version, keyName, err)
	}
	return status, nil
}

// Encrypt seals plaintext for a bound context under the key's active
// version and returns the printable envelope. The span carries the
// operation and outcome only.
func Encrypt(ctx context.Context, q Querier, p Provider, c Context, plaintext []byte) (string, error) {
	ctx, span := otel.Tracer("athena/secrets").Start(ctx, "athena.secrets.encrypt")
	defer span.End()
	outcome := "ok"
	defer func() { span.SetAttributes(attribute.String("athena.outcome", outcome)) }()
	envelope, err := encrypt(ctx, q, p, c, plaintext)
	if err != nil {
		outcome = "error"
		span.RecordError(err)
	}
	return envelope, err
}

func encrypt(ctx context.Context, q Querier, p Provider, c Context, plaintext []byte) (string, error) {
	if err := c.validate(); err != nil {
		return "", err
	}
	ref, err := GetRef(ctx, q, c.KeyName)
	if err != nil {
		return "", err
	}
	if ref.Provider != p.Name() {
		return "", fmt.Errorf("secrets: reference %q needs provider %q", c.KeyName, ref.Provider)
	}
	version, err := ActiveVersion(ctx, q, c.KeyName)
	if err != nil {
		return "", err
	}
	key, err := p.Key(ctx, ref.ProviderRef+"/v"+strconv.FormatInt(version, 10))
	if err != nil {
		return "", err
	}
	gcm, err := gcmCipher(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("secrets: random nonce: %w", err)
	}
	sealed := gcm.Seal(nil, nonce, plaintext, contextBytes(c, version))
	return envelopePrefix + ":" + c.KeyName + ":" + strconv.FormatInt(version, 10) + ":" +
		base64.RawURLEncoding.EncodeToString(nonce) + ":" + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Decrypt opens an envelope for its bound context. A value copied into
// another Record, a revoked or unknown version, or any tampering fails
// with an error that carries no secret material.
func Decrypt(ctx context.Context, q Querier, p Provider, c Context, envelope string) ([]byte, error) {
	ctx, span := otel.Tracer("athena/secrets").Start(ctx, "athena.secrets.decrypt")
	defer span.End()
	outcome := "ok"
	defer func() { span.SetAttributes(attribute.String("athena.outcome", outcome)) }()
	plaintext, err := decrypt(ctx, q, p, c, envelope)
	if err != nil {
		outcome = "error"
		span.RecordError(err)
	}
	return plaintext, err
}

func decrypt(ctx context.Context, q Querier, p Provider, c Context, envelope string) ([]byte, error) {
	parts := strings.Split(envelope, ":")
	if len(parts) != 5 || parts[0] != envelopePrefix {
		return nil, fmt.Errorf("secrets: envelope is not a field-encryption value")
	}
	keyName, versionText, nonceText, sealedText := parts[1], parts[2], parts[3], parts[4]
	if err := validateName("key", keyName); err != nil {
		return nil, fmt.Errorf("secrets: envelope is not a field-encryption value")
	}
	version, err := strconv.ParseInt(versionText, 10, 64)
	if err != nil || version <= 0 {
		return nil, fmt.Errorf("secrets: envelope is not a field-encryption value")
	}
	c.KeyName = keyName
	if err := c.validate(); err != nil {
		return nil, err
	}
	ref, err := GetRef(ctx, q, keyName)
	if err != nil {
		return nil, err
	}
	if ref.Provider != p.Name() {
		return nil, fmt.Errorf("secrets: reference %q needs provider %q", keyName, ref.Provider)
	}
	status, err := versionStatus(ctx, q, keyName, version)
	if err != nil {
		return nil, err
	}
	if status == VersionRevoked {
		return nil, fmt.Errorf("secrets: version %d of %q is revoked", version, keyName)
	}
	key, err := p.Key(ctx, ref.ProviderRef+"/v"+strconv.FormatInt(version, 10))
	if err != nil {
		return nil, err
	}
	gcm, err := gcmCipher(key)
	if err != nil {
		return nil, err
	}
	nonce, err := base64.RawURLEncoding.DecodeString(nonceText)
	if err != nil {
		return nil, fmt.Errorf("secrets: decryption failed")
	}
	sealed, err := base64.RawURLEncoding.DecodeString(sealedText)
	if err != nil {
		return nil, fmt.Errorf("secrets: decryption failed")
	}
	plaintext, err := gcm.Open(nil, nonce, sealed, contextBytes(c, version))
	if err != nil {
		return nil, fmt.Errorf("secrets: decryption failed")
	}
	return plaintext, nil
}

// gcmCipher builds the AES-256-GCM cipher for key material.
func gcmCipher(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secrets: key must hold %d bytes for AES-256", keyLen)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secrets: AES-GCM unavailable: %w", err)
	}
	return gcm, nil
}

// contextBytes serializes the binding with length prefixes, so no two
// different contexts serialize alike.
func contextBytes(c Context, version int64) []byte {
	var out []byte
	for _, field := range []string{c.Workspace, c.RecordType, c.RecordID, c.Field, c.KeyName, strconv.FormatInt(version, 10)} {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(field)))
		out = append(out, length[:]...)
		out = append(out, field...)
	}
	return out
}
