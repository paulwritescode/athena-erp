package brand

import "fmt"

// URL builders for the brand routes served by the Go server. Every URL
// carries a content hash (or the mark fallback), so responses cache for a
// year and a changed logo gets a new address.

// LogoURL is the address of a stored logo.
func LogoURL(hash, ext string) string {
	return fmt.Sprintf("/brand/logo-%s.%s", hash, ext)
}

// FaviconURL is the address of a favicon. Hash is the light PNG logo's hash,
// or MarkHash when the favicon renders from the Athena mark.
func FaviconURL(hash string, size int) string {
	return fmt.Sprintf("/brand/favicon-%s-%d.png", hash, size)
}

// MarkHash stands in for a content hash when the Athena mark is the source.
const MarkHash = "mark"
