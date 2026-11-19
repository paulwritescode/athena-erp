package files

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Issue is one problem in a file, with the 1-based line it is on (0 when the
// problem is not tied to a line).
type Issue struct {
	Line    int
	Message string
}

// ValidationError reports every issue found in a file. It matches
// ErrInvalidFile.
type ValidationError struct {
	Path   string
	Issues []Issue
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		if issue.Line > 0 {
			parts = append(parts, fmt.Sprintf("line %d: %s", issue.Line, issue.Message))
		} else {
			parts = append(parts, issue.Message)
		}
	}
	return fmt.Sprintf("%s is not valid: %s", e.Path, strings.Join(parts, "; "))
}

func (e *ValidationError) Is(target error) bool { return target == ErrInvalidFile }

const maxIssues = 10

// checkText refuses content that is too large or is not UTF-8 text.
func checkText(content []byte) error {
	if len(content) > MaxFileBytes {
		return fmt.Errorf("%w: the limit is %d bytes", ErrTooLarge, MaxFileBytes)
	}
	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return fmt.Errorf("%w: only UTF-8 text is managed", ErrNotText)
	}
	return nil
}

// Validate checks content against the rules for its path. Text and size are
// checked for every file. YAML and JSON files must parse, and a Record Type
// file must follow the Record Type file format (spec section 24). Rule
// sources (.star) are stored and edited but not run here.
func Validate(p string, content []byte) error {
	if err := checkText(content); err != nil {
		return err
	}
	var issues []Issue
	switch path.Ext(p) {
	case ".yaml", ".yml":
		var root yaml.Node
		if err := yaml.Unmarshal(content, &root); err != nil {
			issues = []Issue{yamlIssue(err)}
		} else if isRecordTypeFile(p) {
			issues = recordTypeIssues(p, &root)
		}
	case ".json":
		if err := json.Unmarshal(content, new(any)); err != nil {
			issues = []Issue{{Line: jsonLine(content, err), Message: "invalid JSON: " + err.Error()}}
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Path: p, Issues: issues[:min(len(issues), maxIssues)]}
	}
	return nil
}

var yamlLine = regexp.MustCompile(`line (\d+)`)

func yamlIssue(err error) Issue {
	message := strings.TrimPrefix(err.Error(), "yaml: ")
	issue := Issue{Message: message}
	if m := yamlLine.FindStringSubmatch(message); m != nil {
		issue.Line, _ = strconv.Atoi(m[1])
	}
	return issue
}

func jsonLine(content []byte, err error) int {
	var syntax *json.SyntaxError
	if !errors.As(err, &syntax) {
		return 0
	}
	return 1 + bytes.Count(content[:min(int(syntax.Offset), len(content))], []byte("\n"))
}

// isRecordTypeFile reports whether p is a Record Type definition: a YAML
// file in a record_types folder, at the top or inside a Pack.
func isRecordTypeFile(p string) bool {
	parts := strings.Split(p, "/")
	return len(parts) >= 2 && parts[len(parts)-2] == "record_types"
}

var (
	identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

	// Top-level keys of a Record Type file (spec section 24.2).
	recordTypeKeys = []string{"api", "record_type", "renamed_from", "label", "description", "icon", "module", "kind", "class",
		"scope", "tree", "effective_dated", "naming", "title_field", "search_fields", "sort", "field_groups", "fields",
		"lifecycle", "posting", "flow", "dimensions", "grants", "indexes", "constraints", "events", "audit", "retention",
		"reports", "views", "extension_points"}
	recordTypeKinds   = []string{"master", "document", "child", "ledger", "balance", "setting", "log", "snapshot", "meta"}
	recordTypeClasses = []string{"posting", "non_posting", "work", "batch"}
)

// recordTypeIssues checks the parts of the Record Type file format that do
// not depend on a field type table: required keys, identifiers, the kind and
// class vocabularies, and unique field names. The record_type key must match
// the file name, so the file-to-Record-Type mapping holds.
func recordTypeIssues(p string, doc *yaml.Node) []Issue {
	var issues []Issue
	add := func(line int, format string, args ...any) {
		issues = append(issues, Issue{Line: line, Message: fmt.Sprintf(format, args...)})
	}
	if len(doc.Content) == 0 {
		return []Issue{{Line: 1, Message: "the file is empty; a Record Type file is a mapping with api, record_type, label, kind and fields"}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return []Issue{{Line: root.Line, Message: "a Record Type file must be a mapping of keys"}}
	}
	values := map[string]*yaml.Node{}
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if _, dup := values[key.Value]; dup {
			add(key.Line, "key %q appears twice", key.Value)
			continue
		}
		values[key.Value] = value
		if !slices.Contains(recordTypeKeys, key.Value) {
			add(key.Line, "unknown key %q", key.Value)
		}
	}
	scalar := func(key string) (string, int) {
		node, ok := values[key]
		if !ok {
			add(root.Line, "missing required key %q", key)
			return "", root.Line
		}
		if node.Kind != yaml.ScalarNode || strings.TrimSpace(node.Value) == "" {
			add(node.Line, "%q must be a non-empty text value", key)
			return "", node.Line
		}
		return node.Value, node.Line
	}
	scalar("api")
	if name, line := scalar("record_type"); name != "" {
		stem := strings.TrimSuffix(path.Base(p), path.Ext(p))
		switch {
		case !identifierPattern.MatchString(name):
			add(line, "record_type %q must be lowercase snake_case, starting with a letter", name)
		case name != stem:
			add(line, "record_type %q must match the file name %q", name, stem)
		}
	}
	scalar("label")
	if kind, line := scalar("kind"); kind != "" && !slices.Contains(recordTypeKinds, kind) {
		add(line, "kind %q must be one of %s", kind, strings.Join(recordTypeKinds, ", "))
	}
	if node, ok := values["class"]; ok && !slices.Contains(recordTypeClasses, node.Value) {
		add(node.Line, "class %q must be one of %s", node.Value, strings.Join(recordTypeClasses, ", "))
	}
	fields, ok := values["fields"]
	if !ok {
		add(root.Line, "missing required key %q", "fields")
		return issues
	}
	if fields.Kind != yaml.SequenceNode {
		add(fields.Line, "fields must be a list")
		return issues
	}
	seen := map[string]bool{}
	for _, field := range fields.Content {
		if field.Kind != yaml.MappingNode {
			add(field.Line, "each field must be a mapping with a name and a type")
			continue
		}
		var name, kind string
		nameLine := field.Line
		for i := 0; i+1 < len(field.Content); i += 2 {
			switch field.Content[i].Value {
			case "name":
				name, nameLine = field.Content[i+1].Value, field.Content[i+1].Line
			case "type":
				kind = field.Content[i+1].Value
			}
		}
		switch {
		case !identifierPattern.MatchString(name):
			add(nameLine, "field name %q must be lowercase snake_case, starting with a letter", name)
		case seen[name]:
			add(nameLine, "field %q is declared twice", name)
		}
		seen[name] = true
		if strings.TrimSpace(kind) == "" {
			add(field.Line, "field %q needs a type", name)
		}
	}
	slices.SortStableFunc(issues, func(a, b Issue) int { return a.Line - b.Line })
	return issues
}

// Backing says which Record Type, Rule or Report a file holds, from its
// folder and name alone. Kinds are record_type, rule, report and
// generated_source. It returns "" for a file that backs none of them.
func Backing(p string) (kind, key string) {
	parts := strings.Split(p, "/")
	if len(parts) < 2 {
		return "", ""
	}
	dir, name := parts[len(parts)-2], parts[len(parts)-1]
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	switch {
	case parts[0] == "generated" && len(parts) == 2 && ext == ".go" && strings.HasSuffix(stem, "_gen"):
		return "generated_source", strings.TrimSuffix(stem, "_gen")
	case dir == "record_types" && (ext == ".yaml" || ext == ".yml"):
		return "record_type", stem
	case dir == "rules" && ext == ".star":
		return "rule", stem
	case dir == "rules" && (ext == ".yaml" || ext == ".yml") && strings.HasSuffix(stem, ".rule"):
		return "rule", strings.TrimSuffix(stem, ".rule")
	case dir == "reports" && (ext == ".yaml" || ext == ".yml") && strings.HasSuffix(stem, ".report"):
		return "report", strings.TrimSuffix(stem, ".report")
	}
	return "", ""
}
