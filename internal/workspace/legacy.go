package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

// legacyHomeFile is the marker name from before the vocabulary rename
// (ADR-0008), when a home was called a workspace. Nothing else may read it.
const legacyHomeFile = "athena-workspace.yaml"

// UpgradeLegacyHome renames an old athena-workspace.yaml in dir to
// athena-home.yaml once. The file's keys are unchanged. It reports whether
// it renamed anything and refuses to overwrite an existing athena-home.yaml.
func UpgradeLegacyHome(dir string) (bool, error) {
	oldPath := filepath.Join(dir, legacyHomeFile)
	if !exists(oldPath) {
		return false, nil
	}
	newPath := filepath.Join(dir, HomeFile)
	if exists(newPath) {
		return false, fmt.Errorf("%s and %s both exist in %s; remove the old %s", legacyHomeFile, HomeFile, dir, legacyHomeFile)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return false, fmt.Errorf("rename %s to %s: %w", legacyHomeFile, HomeFile, err)
	}
	return true, nil
}
