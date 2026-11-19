package types

import (
	"errors"
	"time"
)

// ErrDate is returned for text that is not a valid date or datetime.
var ErrDate = errors.New("types: invalid date or datetime")

const dateLayout = "2006-01-02"

// ParseDate parses a calendar date written exactly as YYYY-MM-DD. The result
// is midnight UTC; it carries no time zone meaning.
func ParseDate(s string) (time.Time, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil || t.Format(dateLayout) != s {
		return time.Time{}, ErrDate
	}
	return t, nil
}

// FormatDate writes t as YYYY-MM-DD using its UTC calendar day.
func FormatDate(t time.Time) string { return t.UTC().Format(dateLayout) }

// ParseDateTime parses an RFC 3339 datetime that states its offset ("Z" or
// "+03:00") and returns it in UTC. Text without an offset is rejected so a
// stored instant never depends on the server's zone.
func ParseDateTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, ErrDate
	}
	return t.UTC(), nil
}

// FormatDateTime writes t in UTC as RFC 3339 with the fraction digits it needs.
func FormatDateTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
