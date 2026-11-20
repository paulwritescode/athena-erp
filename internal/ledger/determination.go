package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// ErrNoMatch reports a determination lookup nothing matches.
var ErrNoMatch = errors.New("ledger: no determination matches")

// Determination maps a posting context to an account. Match holds
// dimension-name to value pairs; absent dimensions are wildcards. Specificity
// is the number of pairs: more pairs win, and equal winners are an error
// that names them.
type Determination struct {
	ID       string
	Label    string
	Account  string
	Match    map[string]string
	Disabled bool
	Revision int64
}

func (s *Service) SaveDetermination(ctx context.Context, determination Determination, expected int64) (Determination, error) {
	if err := checkLabel(determination.Label); err != nil {
		return Determination{}, err
	}
	account, err := s.GetAccount(ctx, determination.Account)
	if err != nil {
		return Determination{}, err
	}
	if account.IsGroup {
		return Determination{}, inputErrorf("account %q is a group; mappings point at leaves", determination.Account)
	}
	for key, value := range determination.Match {
		if key == "" || value == "" {
			return Determination{}, inputErrorf("match dimensions and values must not be empty")
		}
	}
	match, err := json.Marshal(nonNilMap(determination.Match))
	if err != nil {
		return Determination{}, err
	}
	if expected == 0 {
		id, err := uuid.NewV7()
		if err != nil {
			return Determination{}, err
		}
		_, err = s.DB.ExecContext(ctx, `INSERT INTO athena_account_determinations (id, label, account_name, match_json, disabled) VALUES (?, ?, ?, ?, ?)`,
			id.String(), determination.Label, determination.Account, match, determination.Disabled)
		if err != nil {
			return Determination{}, s.conflict(err, "determination", determination.Label)
		}
		determination.ID, determination.Revision = id.String(), 1
		return determination, nil
	}
	if determination.ID == "" {
		return Determination{}, inputErrorf("an update needs the determination id")
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE athena_account_determinations SET label = ?, account_name = ?, match_json = ?, disabled = ?, revision = revision + 1 WHERE id = ? AND revision = ?`,
		determination.Label, determination.Account, match, determination.Disabled, determination.ID, expected)
	if err != nil {
		return Determination{}, err
	}
	return determination, s.cas(ctx, result, "athena_account_determinations", "id", determination.ID, expected, &determination.Revision)
}

func (s *Service) GetDetermination(ctx context.Context, id string) (Determination, error) {
	var determination Determination
	var match []byte
	err := s.DB.QueryRowContext(ctx, `SELECT id, label, account_name, match_json, disabled, revision FROM athena_account_determinations WHERE id = ?`, id).
		Scan(&determination.ID, &determination.Label, &determination.Account, &match, &determination.Disabled, &determination.Revision)
	if err != nil {
		return Determination{}, notFound(err, "determination", id)
	}
	if err := json.Unmarshal(match, &determination.Match); err != nil {
		return Determination{}, err
	}
	return determination, nil
}

func (s *Service) ListDeterminations(ctx context.Context) ([]Determination, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, label, account_name, match_json, disabled, revision FROM athena_account_determinations ORDER BY label`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Determination
	for rows.Next() {
		var determination Determination
		var match []byte
		if err := rows.Scan(&determination.ID, &determination.Label, &determination.Account, &match, &determination.Disabled, &determination.Revision); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(match, &determination.Match); err != nil {
			return nil, err
		}
		out = append(out, determination)
	}
	return out, rows.Err()
}

// ResolveAccount picks the enabled mapping whose every pair equals the
// context, preferring the most pairs. Equal winners fail naming each tied
// mapping so the author can break the tie; nothing matching fails with
// ErrNoMatch.
func (s *Service) ResolveAccount(ctx context.Context, context_ map[string]string) (Determination, error) {
	mappings, err := s.ListDeterminations(ctx)
	if err != nil {
		return Determination{}, err
	}
	return MatchDeterminations(mappings, context_)
}

// MatchDeterminations is the pure core of ResolveAccount, so tests can
// resolve hypothetical mappings without storing them.
func MatchDeterminations(mappings []Determination, context_ map[string]string) (Determination, error) {
	var winners []Determination
	best := -1
	for _, mapping := range mappings {
		if mapping.Disabled {
			continue
		}
		matched := true
		for key, value := range mapping.Match {
			if context_[key] != value {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		if specificity := len(mapping.Match); specificity > best {
			best, winners = specificity, []Determination{mapping}
		} else if specificity == best {
			winners = append(winners, mapping)
		}
	}
	if len(winners) == 0 {
		return Determination{}, fmt.Errorf("%w for this posting", ErrNoMatch)
	}
	if len(winners) > 1 {
		sort.Slice(winners, func(i, j int) bool { return winners[i].Label < winners[j].Label })
		names := make([]string, len(winners))
		for i, winner := range winners {
			names[i] = fmt.Sprintf("%q (-> %s)", winner.Label, winner.Account)
		}
		return Determination{}, fmt.Errorf("%w: %s match with equal specificity", ErrAmbiguous, strings.Join(names, " and "))
	}
	return winners[0], nil
}

func nonNilMap(values map[string]string) map[string]string {
	if values == nil {
		return map[string]string{}
	}
	return values
}
