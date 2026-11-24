package secrets

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// keyLen is the AES-256 key length in bytes. Key files hold exactly
// these bytes with no encoding and no trailing newline.
const keyLen = 32

// FileProvider is the local file Provider: it reads key material from
// regular files under an operator-managed root directory. The root itself
// is operator configuration (an env path or a keychain-exported file in
// production); provider wiring for env and keychain sources lands with
// the `athena secrets` commands.
type FileProvider struct {
	root string
}

// NewFileProvider serves key files from root. Locators are slash
// separated paths relative to root.
func NewFileProvider(root string) *FileProvider {
	return &FileProvider{root: root}
}

// Name identifies the file provider in reference rows.
func (p *FileProvider) Name() string {
	return "file"
}

// Key reads the key file at locator and returns its bytes. Absolute
// paths and parent-directory escapes are refused, and the file must
// hold exactly 32 bytes. Errors name the locator, never the material.
func (p *FileProvider) Key(_ context.Context, locator string) ([]byte, error) {
	clean, err := cleanLocator(locator)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(p.root, filepath.FromSlash(clean)))
	if err != nil {
		return nil, fmt.Errorf("secrets: read key %q: %w", locator, err)
	}
	if len(data) != keyLen {
		return nil, fmt.Errorf("secrets: key %q must hold %d bytes for AES-256", locator, keyLen)
	}
	return data, nil
}

// cleanLocator rejects empty, absolute and escaping locators and
// returns the cleaned relative path.
func cleanLocator(locator string) (string, error) {
	if locator == "" || path.IsAbs(locator) {
		return "", fmt.Errorf("secrets: key locator %q must be a relative path", locator)
	}
	clean := path.Clean(locator)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("secrets: key locator %q escapes the key directory", locator)
	}
	return clean, nil
}

// newID returns a time-ordered id for a new row.
func newID() (string, error) {
	v7, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return v7.String(), nil
}
