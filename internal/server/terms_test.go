package server

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// oldTerms are names from before ADR-0008. After system migration 5 they
// may appear only in the files listed in oldTermsAllowed.
var oldTerms = regexp.MustCompile(`athena_records\b|athena_modules\b|athena_entry\w*|athena_project\b|athena_record_\w+|EntryService|target_record\b|--project\b|entries\.proto|athena-workspace\.yaml|fk_record_`)

// currentRecordTables are the system tables whose names start with athena_record_.
var currentRecordTables = map[string]bool{"athena_record_types": true, "athena_record_type_operations": true, "athena_record_history": true}

// oldTermsAllowed holds the files that must keep old names: the frozen
// legacy names, migrations 1 to 5 and the home marker upgrade.
var oldTermsAllowed = map[string]bool{
	"internal/server/legacy_names.go":      true,
	"internal/server/sysmigrations.go":     true,
	"internal/server/vocabulary_rename.go": true,
	"internal/workspace/legacy.go":         true,
}

func TestOldVocabularyIsGoneOutsideLegacyFiles(t *testing.T) {
	root := filepath.Join("..", "..")
	var found []string
	for _, dir := range []string{"cmd", "internal", "proto", "gen", "web/src"} {
		scanOldTerms(t, root, dir, &found)
	}
	if len(found) > 0 {
		t.Fatalf("old vocabulary outside the legacy files:\n%s", strings.Join(found, "\n"))
	}
}

func scanOldTerms(t *testing.T, root, dir string, found *[]string) {
	t.Helper()
	err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			switch rel {
			case "internal/webui/dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, ".proto") && !strings.HasSuffix(rel, ".vue") && !strings.HasSuffix(rel, ".ts") && !strings.HasSuffix(rel, ".json") {
			return nil
		}
		if strings.HasSuffix(rel, "_test.go") || oldTermsAllowed[rel] || strings.HasSuffix(rel, "pnpm-lock.yaml") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range oldTerms.FindAllString(string(data), -1) {
			if !currentRecordTables[match] {
				*found = append(*found, rel+": "+match)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
