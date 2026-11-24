// Package secrets is the kernel's secret-reference and field-encryption
// service (batch-2 G8, ADR-0033, Proposed): the database holds references
// and ciphertext only, never secret values or key material. Named
// references resolve through a Provider (a local file provider ships;
// OpenBao or AWS stay extension points), key versions rotate per key, and
// AES-256-GCM field encryption binds Workspace, Record Type, Record ID and
// field into the authenticated context, so a protected value copied into
// another Record fails to decrypt. Errors, logs and spans carry names and
// outcome codes only, never secret material. The tables live in system
// migration 22.
package secrets

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Querier runs statements on a database or an open transaction.
type Querier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Provider fetches key material by locator. The locator is provider
// defined (a relative path for the file provider) and names key material
// without containing it. Implementations must never log the material.
type Provider interface {
	// Name identifies the provider in reference rows ("file" for the
	// local file provider).
	Name() string
	// Key returns the raw key bytes for a locator.
	Key(ctx context.Context, locator string) ([]byte, error)
}

// Ref is one athena_secret_refs row: a logical name bound to a provider
// and a provider-side locator. It never holds a secret value.
type Ref struct {
	ID          string
	Name        string
	Provider    string
	ProviderRef string
}

// maxNameLen and maxRefLen are the column widths of athena_secret_refs.
const (
	maxNameLen     = 140
	maxProviderLen = 40
	maxRefLen      = 600
)

// validateName rejects empty, overlong and colon-bearing reference and
// key names. Colons would break envelope parsing, so they never enter
// the tables.
func validateName(kind, name string) error {
	if name == "" {
		return fmt.Errorf("secrets: %s name is empty", kind)
	}
	if len(name) > maxNameLen {
		return fmt.Errorf("secrets: %s name %q is over %d characters", kind, name, maxNameLen)
	}
	if strings.Contains(name, ":") {
		return fmt.Errorf("secrets: %s name %q must not contain a colon", kind, name)
	}
	return nil
}

// SaveRef stores a reference by name, creating it or replacing its
// provider and locator, and returns the stored row.
func SaveRef(ctx context.Context, q Querier, name, provider, providerRef string) (Ref, error) {
	if err := validateName("reference", name); err != nil {
		return Ref{}, err
	}
	if provider == "" || len(provider) > maxProviderLen {
		return Ref{}, fmt.Errorf("secrets: provider name %q is empty or over %d characters", provider, maxProviderLen)
	}
	if providerRef == "" || len(providerRef) > maxRefLen {
		return Ref{}, fmt.Errorf("secrets: provider reference for %q is empty or over %d characters", name, maxRefLen)
	}
	id, err := newID()
	if err != nil {
		return Ref{}, err
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO athena_secret_refs (id, name, provider, provider_ref) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE provider = VALUES(provider), provider_ref = VALUES(provider_ref)`, id, name, provider, providerRef); err != nil {
		return Ref{}, fmt.Errorf("secrets: save reference %q: %w", name, err)
	}
	return GetRef(ctx, q, name)
}

// GetRef loads a reference by name.
func GetRef(ctx context.Context, q Querier, name string) (Ref, error) {
	if err := validateName("reference", name); err != nil {
		return Ref{}, err
	}
	var ref Ref
	if err := q.QueryRowContext(ctx, `SELECT id, name, provider, provider_ref FROM athena_secret_refs WHERE name = ?`, name).Scan(&ref.ID, &ref.Name, &ref.Provider, &ref.ProviderRef); err != nil {
		if err == sql.ErrNoRows {
			return Ref{}, fmt.Errorf("secrets: reference %q not found", name)
		}
		return Ref{}, fmt.Errorf("secrets: load reference %q: %w", name, err)
	}
	return ref, nil
}

// DeleteRef removes a reference by name.
func DeleteRef(ctx context.Context, q Querier, name string) error {
	if err := validateName("reference", name); err != nil {
		return err
	}
	res, err := q.ExecContext(ctx, `DELETE FROM athena_secret_refs WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("secrets: delete reference %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("secrets: delete reference %q: %w", name, err)
	}
	if n == 0 {
		return fmt.Errorf("secrets: reference %q not found", name)
	}
	return nil
}
