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

	"github.com/google/uuid"
)

// ErrNoMatch reports a determination lookup nothing matches.
var ErrNoMatch = errors.New("ledger: no determination matches")

// Determination maps a posting context to an account. Match holds
// dimension-name to value pairs; absent dimensions are wildcards. Specificity
// is the number of pairs: more pairs win, and equal winners are an error
// that names them. EffectiveFrom and EffectiveTo bound the mapping to the
// end-exclusive date range it holds for; an empty bound stays open, so a
// mapping with no bounds holds at every date.
type Determination struct {
	ID            string
	Label         string
	Account       string
	Match         map[string]string
	EffectiveFrom string
	EffectiveTo   string
	Disabled      bool
	Revision      int64
}

// checkEffectiveRange validates an end-exclusive effective range the way
// effective-dated Record Types do: strict calendar dates, and from strictly
// before to, so an empty range never saves.
func checkEffectiveRange(from, to string) error {
	if from != "" {
		if _, err := time.Parse(time.DateOnly, from); err != nil {
			return inputErrorf("effective from %q is not a date as YYYY-MM-DD", from)
		}
	}
	if to != "" {
		if _, err := time.Parse(time.DateOnly, to); err != nil {
			return inputErrorf("effective to %q is not a date as YYYY-MM-DD", to)
		}
	}
	if from != "" && to != "" && from >= to {
		return inputErrorf("effective from must be before effective to")
	}
	return nil
}

// checkAsOf validates an "as of" date for determination lookups.
func checkAsOf(asOf string) error {
	if _, err := time.Parse(time.DateOnly, asOf); err != nil {
		return inputErrorf("as of must be a date as YYYY-MM-DD")
	}
	return nil
}

// effectiveOn reports whether asOf falls in the end-exclusive range
// [from, to). Callers validate the dates first; YYYY-MM-DD sorts
// chronologically, so plain string comparison decides.
func effectiveOn(from, to, asOf string) bool {
	return (from == "" || from <= asOf) && (to == "" || asOf < to)
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
	if err := checkEffectiveRange(determination.EffectiveFrom, determination.EffectiveTo); err != nil {
		return Determination{}, err
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
		_, err = s.DB.ExecContext(ctx, `INSERT INTO athena_account_determinations (id, label, account_name, match_json, effective_from, effective_to, disabled) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id.String(), determination.Label, determination.Account, match, nullText(determination.EffectiveFrom), nullText(determination.EffectiveTo), determination.Disabled)
		if err != nil {
			return Determination{}, s.conflict(err, "determination", determination.Label)
		}
		determination.ID, determination.Revision = id.String(), 1
		return determination, nil
	}
	if determination.ID == "" {
		return Determination{}, inputErrorf("an update needs the determination id")
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE athena_account_determinations SET label = ?, account_name = ?, match_json = ?, effective_from = ?, effective_to = ?, disabled = ?, revision = revision + 1 WHERE id = ? AND revision = ?`,
		determination.Label, determination.Account, match, nullText(determination.EffectiveFrom), nullText(determination.EffectiveTo), determination.Disabled, determination.ID, expected)
	if err != nil {
		return Determination{}, err
	}
	return determination, s.cas(ctx, result, "athena_account_determinations", "id", determination.ID, expected, &determination.Revision)
}

func (s *Service) GetDetermination(ctx context.Context, id string) (Determination, error) {
	var determination Determination
	var match []byte
	var from, to sql.NullTime
	err := s.DB.QueryRowContext(ctx, `SELECT id, label, account_name, match_json, effective_from, effective_to, disabled, revision FROM athena_account_determinations WHERE id = ?`, id).
		Scan(&determination.ID, &determination.Label, &determination.Account, &match, &from, &to, &determination.Disabled, &determination.Revision)
	if err != nil {
		return Determination{}, notFound(err, "determination", id)
	}
	if err := json.Unmarshal(match, &determination.Match); err != nil {
		return Determination{}, err
	}
	determination.EffectiveFrom, determination.EffectiveTo = scanNullDate(from), scanNullDate(to)
	return determination, nil
}

func (s *Service) ListDeterminations(ctx context.Context) ([]Determination, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, label, account_name, match_json, effective_from, effective_to, disabled, revision FROM athena_account_determinations ORDER BY label`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Determination
	for rows.Next() {
		var determination Determination
		var match []byte
		var from, to sql.NullTime
		if err := rows.Scan(&determination.ID, &determination.Label, &determination.Account, &match, &from, &to, &determination.Disabled, &determination.Revision); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(match, &determination.Match); err != nil {
			return nil, err
		}
		determination.EffectiveFrom, determination.EffectiveTo = scanNullDate(from), scanNullDate(to)
		out = append(out, determination)
	}
	return out, rows.Err()
}

// ResolveAccount picks the enabled mapping effective on asOf whose every
// pair equals the context, preferring the most pairs. Equal winners fail
// naming each tied mapping so the author can break the tie; nothing
// matching fails with ErrNoMatch. The winner carries the resolved account
// and the mapping revision the posting stores (R13.3).
func (s *Service) ResolveAccount(ctx context.Context, context_ map[string]string, asOf string) (Determination, error) {
	if err := checkAsOf(asOf); err != nil {
		return Determination{}, err
	}
	mappings, err := s.ListDeterminations(ctx)
	if err != nil {
		return Determination{}, err
	}
	return MatchDeterminations(mappings, context_, asOf)
}

// MatchDeterminations is the pure core of ResolveAccount, so tests can
// resolve hypothetical mappings without storing them.
func MatchDeterminations(mappings []Determination, context_ map[string]string, asOf string) (Determination, error) {
	if err := checkAsOf(asOf); err != nil {
		return Determination{}, err
	}
	var winners []Determination
	best := -1
	for _, mapping := range mappings {
		if mapping.Disabled {
			continue
		}
		if err := checkEffectiveRange(mapping.EffectiveFrom, mapping.EffectiveTo); err != nil {
			return Determination{}, fmt.Errorf("mapping %q: %v", mapping.Label, err)
		}
		if !effectiveOn(mapping.EffectiveFrom, mapping.EffectiveTo, asOf) {
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
