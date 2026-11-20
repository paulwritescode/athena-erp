package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Setting value kinds and list merge rules.
const (
	KindScalar = "scalar"
	KindList   = "list"

	RuleReplace   = "replace"
	RuleAppend    = "append"
	RuleIntersect = "intersect"
)

// Generation statuses.
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
)

// normInstant truncates to the DATETIME(3) precision the database keeps, so
// written and compared instants always match.
func normInstant(at time.Time) time.Time { return at.UTC().Truncate(time.Millisecond) }

// ErrNoGeneration reports a resolution nothing applies to.
var ErrNoGeneration = errors.New("ledger: no setting generation applies")

// Setting defines one owned setting: its value shape and, for lists, how
// overlapping generations merge.
type Setting struct {
	OwnerPack string
	Type      string
	Label     string
	Kind      string
	ListRule  string
	Revision  int64
}

// SettingValue is one generation: the value in force for a scope chain from
// an instant. Generations are append-only; published rows never change, and
// drafts are discarded, never edited.
type SettingValue struct {
	OwnerPack     string
	Type          string
	ScopeChain    string
	EffectiveFrom time.Time
	Revision      int64
	Value         any
	Status        string
}

// Resolution is what Resolve found: the effective value and the generations
// it came from, least specific first.
type Resolution struct {
	Value   any
	Winners []SettingValue
}

func (s *Service) SaveSetting(ctx context.Context, setting Setting, expected int64) (Setting, error) {
	if err := checkName("owner pack", setting.OwnerPack); err != nil {
		return Setting{}, err
	}
	if err := checkName("setting type", setting.Type); err != nil {
		return Setting{}, err
	}
	if err := checkLabel(setting.Label); err != nil {
		return Setting{}, err
	}
	switch setting.Kind {
	case KindScalar:
		if setting.ListRule != "" {
			return Setting{}, inputErrorf("a scalar setting takes no list rule")
		}
	case KindList:
		switch setting.ListRule {
		case RuleReplace, RuleAppend, RuleIntersect:
		default:
			return Setting{}, inputErrorf("a list setting needs replace, append or intersect")
		}
	default:
		return Setting{}, inputErrorf("value kind %q must be scalar or list", setting.Kind)
	}
	if expected == 0 {
		_, err := s.DB.ExecContext(ctx, `INSERT INTO athena_settings (owner_pack, setting_type, label, value_kind, list_rule) VALUES (?, ?, ?, ?, ?)`,
			setting.OwnerPack, setting.Type, setting.Label, setting.Kind, nullText(setting.ListRule))
		if err != nil {
			return Setting{}, s.conflict(err, "setting", setting.Type)
		}
		setting.Revision = 1
		return setting, nil
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE athena_settings SET label = ?, value_kind = ?, list_rule = ?, revision = revision + 1 WHERE owner_pack = ? AND setting_type = ? AND revision = ?`,
		setting.Label, setting.Kind, nullText(setting.ListRule), setting.OwnerPack, setting.Type, expected)
	if err != nil {
		return Setting{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Setting{}, err
	}
	if affected == 0 {
		if _, err := s.GetSetting(ctx, setting.OwnerPack, setting.Type); err != nil {
			return Setting{}, err
		}
		return Setting{}, fmt.Errorf("%w: expected revision %d", ErrConflict, expected)
	}
	setting.Revision = expected + 1
	return setting, nil
}

func (s *Service) ListSettings(ctx context.Context) ([]Setting, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT owner_pack, setting_type, label, value_kind, list_rule, revision FROM athena_settings ORDER BY owner_pack, setting_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Setting
	for rows.Next() {
		var setting Setting
		var rule sql.NullString
		if err := rows.Scan(&setting.OwnerPack, &setting.Type, &setting.Label, &setting.Kind, &rule, &setting.Revision); err != nil {
			return nil, err
		}
		setting.ListRule = rule.String
		out = append(out, setting)
	}
	return out, rows.Err()
}

func (s *Service) GetSetting(ctx context.Context, ownerPack, settingType string) (Setting, error) {
	var setting Setting
	var rule sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT owner_pack, setting_type, label, value_kind, list_rule, revision FROM athena_settings WHERE owner_pack = ? AND setting_type = ?`, ownerPack, settingType).
		Scan(&setting.OwnerPack, &setting.Type, &setting.Label, &setting.Kind, &rule, &setting.Revision)
	if err != nil {
		return Setting{}, notFound(err, "setting", settingType)
	}
	setting.ListRule = rule.String
	return setting, nil
}

// SaveSettingValue appends a draft generation. Values validate against the
// setting: lists must be arrays. Effective time must not lie in the past:
// changes apply prospectively.
func (s *Service) SaveSettingValue(ctx context.Context, value SettingValue) (SettingValue, error) {
	setting, err := s.GetSetting(ctx, value.OwnerPack, value.Type)
	if err != nil {
		return SettingValue{}, err
	}
	if _, err := parseScopeChain(value.ScopeChain); err != nil {
		return SettingValue{}, err
	}
	payload, err := json.Marshal(value.Value)
	if err != nil {
		return SettingValue{}, err
	}
	if setting.Kind == KindList {
		var list []any
		if err := json.Unmarshal(payload, &list); err != nil || !isJSONArray(payload) {
			return SettingValue{}, inputErrorf("setting %q takes a list value", setting.Type)
		}
	}
	value.EffectiveFrom = normInstant(value.EffectiveFrom)
	today := time.Now().UTC().Truncate(24 * time.Hour)
	if value.EffectiveFrom.Before(today) {
		return SettingValue{}, inputErrorf("effective time lies in the past; changes apply prospectively")
	}
	var revision int64 = 1
	var maxRev sql.NullInt64
	err = s.DB.QueryRowContext(ctx, `SELECT MAX(revision) FROM athena_setting_values WHERE owner_pack = ? AND setting_type = ? AND scope_chain = ? AND effective_from = ?`,
		value.OwnerPack, value.Type, canonicalChain(value.ScopeChain), value.EffectiveFrom).Scan(&maxRev)
	if err != nil {
		return SettingValue{}, err
	}
	if maxRev.Valid {
		revision = maxRev.Int64 + 1
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO athena_setting_values (owner_pack, setting_type, scope_chain, effective_from, revision, value_json, status) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		value.OwnerPack, value.Type, canonicalChain(value.ScopeChain), value.EffectiveFrom, revision, payload, StatusDraft)
	if err != nil {
		return SettingValue{}, s.conflict(err, "setting value", value.Type)
	}
	value.ScopeChain, value.Revision, value.Status = canonicalChain(value.ScopeChain), revision, StatusDraft
	return value, nil
}

// PublishSettingValue moves a draft generation to published. Published rows
// never change; the next change is a new generation.
func (s *Service) PublishSettingValue(ctx context.Context, ownerPack, settingType, chain string, effectiveFrom time.Time, revision int64, actor string) (SettingValue, error) {
	effectiveFrom = normInstant(effectiveFrom)
	result, err := s.DB.ExecContext(ctx, `UPDATE athena_setting_values SET status = ?, published_by = ? WHERE owner_pack = ? AND setting_type = ? AND scope_chain = ? AND effective_from = ? AND revision = ? AND status = ?`,
		StatusPublished, actor, ownerPack, settingType, chain, effectiveFrom, revision, StatusDraft)
	if err != nil {
		return SettingValue{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return SettingValue{}, fmt.Errorf("%w: no such draft generation", ErrNotFound)
	}
	return s.GetSettingValue(ctx, ownerPack, settingType, chain, effectiveFrom, revision)
}

// DiscardSettingValue removes a draft generation. Published rows are kept.
func (s *Service) DiscardSettingValue(ctx context.Context, ownerPack, settingType, chain string, effectiveFrom time.Time, revision int64) error {
	effectiveFrom = normInstant(effectiveFrom)
	result, err := s.DB.ExecContext(ctx, `DELETE FROM athena_setting_values WHERE owner_pack = ? AND setting_type = ? AND scope_chain = ? AND effective_from = ? AND revision = ? AND status = ?`,
		ownerPack, settingType, chain, effectiveFrom, revision, StatusDraft)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return fmt.Errorf("%w: no such draft generation", ErrNotFound)
	}
	return nil
}

func (s *Service) GetSettingValue(ctx context.Context, ownerPack, settingType, chain string, effectiveFrom time.Time, revision int64) (SettingValue, error) {
	var value SettingValue
	var at time.Time
	var payload []byte
	effectiveFrom = normInstant(effectiveFrom)
	err := s.DB.QueryRowContext(ctx, `SELECT owner_pack, setting_type, scope_chain, effective_from, revision, value_json, status FROM athena_setting_values WHERE owner_pack = ? AND setting_type = ? AND scope_chain = ? AND effective_from = ? AND revision = ?`,
		ownerPack, settingType, chain, effectiveFrom, revision).
		Scan(&value.OwnerPack, &value.Type, &value.ScopeChain, &at, &value.Revision, &payload, &value.Status)
	if err != nil {
		return SettingValue{}, notFound(err, "setting value", settingType)
	}
	if err := json.Unmarshal(payload, &value.Value); err != nil {
		return SettingValue{}, err
	}
	value.EffectiveFrom = at
	return value, nil
}

func (s *Service) ListSettingValues(ctx context.Context, ownerPack, settingType string, publishedOnly bool) ([]SettingValue, error) {
	query := `SELECT owner_pack, setting_type, scope_chain, effective_from, revision, value_json, status FROM athena_setting_values WHERE owner_pack = ? AND setting_type = ?`
	if publishedOnly {
		query += ` AND status = 'published'`
	}
	query += ` ORDER BY scope_chain, effective_from, revision`
	rows, err := s.DB.QueryContext(ctx, query, ownerPack, settingType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SettingValue
	for rows.Next() {
		var value SettingValue
		var payload []byte
		if err := rows.Scan(&value.OwnerPack, &value.Type, &value.ScopeChain, &value.EffectiveFrom, &value.Revision, &payload, &value.Status); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &value.Value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// Resolve returns the effective value for a scope at an instant: published
// generations whose chain the scope covers, latest first, folded by kind.
func (s *Service) ResolveSetting(ctx context.Context, ownerPack, settingType string, scope map[string]string, at time.Time) (Resolution, error) {
	setting, err := s.GetSetting(ctx, ownerPack, settingType)
	if err != nil {
		return Resolution{}, err
	}
	generations, err := s.ListSettingValues(ctx, ownerPack, settingType, true)
	if err != nil {
		return Resolution{}, err
	}
	return ResolveGenerations(setting.Kind, setting.ListRule, generations, scope, at)
}

// ResolveGenerations is the pure core of Resolve, over an explicit generation
// set, so previews and tests can resolve hypothetical values without storing
// them. Per chain the latest generation at or before the instant wins, then
// the highest revision; chains fold least specific first. Scalars and
// replace lists take the most specific generation and fail when two chains
// tie for it; append and intersect fold every winner in chain order.
func ResolveGenerations(kind, listRule string, generations []SettingValue, scope map[string]string, at time.Time) (Resolution, error) {
	latest := map[string]SettingValue{}
	for _, generation := range generations {
		if generation.Status != StatusPublished {
			continue
		}
		if generation.EffectiveFrom.After(at) {
			continue
		}
		chain, err := parseScopeChain(generation.ScopeChain)
		if err != nil || !chainMatches(chain, scope) {
			continue
		}
		prior, ok := latest[generation.ScopeChain]
		if !ok || generation.EffectiveFrom.After(prior.EffectiveFrom) ||
			(generation.EffectiveFrom.Equal(prior.EffectiveFrom) && generation.Revision > prior.Revision) {
			latest[generation.ScopeChain] = generation
		}
	}
	if len(latest) == 0 {
		return Resolution{}, ErrNoGeneration
	}
	ordered := make([]SettingValue, 0, len(latest))
	for _, generation := range latest {
		ordered = append(ordered, generation)
	}
	sort.Slice(ordered, func(i, j int) bool {
		li, lj := chainLength(ordered[i].ScopeChain), chainLength(ordered[j].ScopeChain)
		if li != lj {
			return li < lj
		}
		return ordered[i].ScopeChain < ordered[j].ScopeChain
	})
	top := chainLength(ordered[len(ordered)-1].ScopeChain)
	tied := 0
	for _, generation := range ordered {
		if chainLength(generation.ScopeChain) == top {
			tied++
		}
	}
	if kind != KindList || listRule == RuleReplace {
		if tied > 1 {
			names := make([]string, 0, tied)
			for _, generation := range ordered {
				if chainLength(generation.ScopeChain) == top {
					names = append(names, scopeName(generation.ScopeChain))
				}
			}
			return Resolution{}, fmt.Errorf("%w: scopes %s tie for most specific", ErrAmbiguous, strings.Join(names, " and "))
		}
		winner := ordered[len(ordered)-1]
		return Resolution{Value: winner.Value, Winners: []SettingValue{winner}}, nil
	}
	lists := make([][]any, len(ordered))
	for i, generation := range ordered {
		list, ok := generation.Value.([]any)
		if !ok {
			return Resolution{}, inputErrorf("generation at %s is not a list", scopeName(generation.ScopeChain))
		}
		lists[i] = list
	}
	switch listRule {
	case RuleAppend:
		seen := map[string]bool{}
		var out []any
		for _, list := range lists {
			for _, item := range list {
				key, _ := json.Marshal(item)
				if !seen[string(key)] {
					seen[string(key)] = true
					out = append(out, item)
				}
			}
		}
		return Resolution{Value: out, Winners: ordered}, nil
	case RuleIntersect:
		counts := map[string]int{}
		for _, list := range lists {
			seen := map[string]bool{}
			for _, item := range list {
				key, _ := json.Marshal(item)
				if !seen[string(key)] {
					seen[string(key)] = true
					counts[string(key)]++
				}
			}
		}
		var out []any
		finest := lists[len(lists)-1]
		for _, item := range finest {
			key, _ := json.Marshal(item)
			if counts[string(key)] == len(lists) {
				out = append(out, item)
			}
		}
		if out == nil {
			out = []any{}
		}
		return Resolution{Value: out, Winners: ordered}, nil
	default:
		return Resolution{}, inputErrorf("list rule %q must be replace, append or intersect", listRule)
	}
}

// scopeName prints a chain for messages; the global chain prints as global.
func scopeName(chain string) string {
	if chain == "" {
		return "global"
	}
	return chain
}

func chainLength(chain string) int {
	if chain == "" {
		return 0
	}
	return strings.Count(chain, "/") + 1
}

// parseScopeChain reads dim:value segments. The empty chain is global and
// matches every scope.
func parseScopeChain(chain string) (map[string]string, error) {
	out := map[string]string{}
	if chain == "" {
		return out, nil
	}
	for _, segment := range strings.Split(chain, "/") {
		dim, value, ok := strings.Cut(segment, ":")
		if !ok || dim == "" || value == "" {
			return nil, inputErrorf("scope %q must be dim:value segments joined by /", chain)
		}
		out[dim] = value
	}
	return out, nil
}

func chainMatches(chain map[string]string, scope map[string]string) bool {
	for dim, value := range chain {
		if scope[dim] != value {
			return false
		}
	}
	return true
}

// canonicalChain sorts segments so one scope has one spelling.
func canonicalChain(chain string) string {
	if chain == "" {
		return ""
	}
	segments := strings.Split(chain, "/")
	sort.Strings(segments)
	return strings.Join(segments, "/")
}

// Change is one row of a diff preview: the effective value at an instant
// before and after a hypothetical generation.
type Change struct {
	At     time.Time
	Before any
	After  any
}

// DiffPreview shows what a draft generation would change and where it stops
// applying: at its own instant and at every later published instant for its
// chain, resolved with and without it. Nothing is stored.
func (s *Service) DiffPreview(ctx context.Context, ownerPack, settingType string, scope map[string]string, draft SettingValue) ([]Change, error) {
	setting, err := s.GetSetting(ctx, ownerPack, settingType)
	if err != nil {
		return nil, err
	}
	generations, err := s.ListSettingValues(ctx, ownerPack, settingType, true)
	if err != nil {
		return nil, err
	}
	draft.EffectiveFrom = normInstant(draft.EffectiveFrom)
	points := []time.Time{draft.EffectiveFrom}
	for _, generation := range generations {
		if generation.ScopeChain == canonicalChain(draft.ScopeChain) && generation.EffectiveFrom.After(draft.EffectiveFrom) {
			points = append(points, generation.EffectiveFrom)
		}
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Before(points[j]) })
	chain := canonicalChain(draft.ScopeChain)
	with := []SettingValue{{
		OwnerPack: ownerPack, Type: settingType, ScopeChain: chain,
		EffectiveFrom: draft.EffectiveFrom, Revision: draft.Revision, Value: draft.Value, Status: StatusPublished,
	}}
	// Stored generations at the same chain and instant give way to the
	// draft, which supersedes them.
	for _, generation := range generations {
		same := generation.ScopeChain == chain && generation.EffectiveFrom.Equal(draft.EffectiveFrom)
		if !same {
			with = append(with, generation)
		}
	}
	changes := make([]Change, len(points))
	for i, at := range points {
		before, beforeErr := ResolveGenerations(setting.Kind, setting.ListRule, generations, scope, at)
		after, afterErr := ResolveGenerations(setting.Kind, setting.ListRule, with, scope, at)
		changes[i] = Change{At: at}
		if beforeErr == nil {
			changes[i].Before = before.Value
		}
		if afterErr == nil {
			changes[i].After = after.Value
		}
	}
	return changes, nil
}

func isJSONArray(payload []byte) bool {
	for _, c := range payload {
		if c == ' ' || c == '\n' || c == '\t' || c == '\r' {
			continue
		}
		return c == '['
	}
	return false
}
