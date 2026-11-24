package secrets

// Redacted marks a value withheld from logs, spans and errors. Callers
// reaching for a secret value in an observable place use RedactValue or
// RedactString instead of the value.
const Redacted = "[redacted]"

// RedactValue withholds secret bytes from an observable place.
func RedactValue(_ []byte) string {
	return Redacted
}

// RedactString withholds a secret string from an observable place.
func RedactString(_ string) string {
	return Redacted
}
