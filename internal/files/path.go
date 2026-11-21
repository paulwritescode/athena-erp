// Package files manages the source files of one Workspace: Record Type
// definitions, Rule sources, Pack layout and generated source. Every path is
// checked against a fixed rule set before it touches the disk, every file
// operation goes through an os.Root confined to the Workspace directory, and
// every change is recorded as a file version and an Audit Log entry that
// holds the path, hash and actor but never the content (ADR-0015).
package files

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Limits on what Athena will manage.
const (
	// MaxFileBytes is the largest file Athena reads or writes.
	MaxFileBytes = 1 << 20
	// MaxImageBytes is the largest image Athena uploads.
	MaxImageBytes = 5 << 20
	// MaxFiles is the most files a Workspace may hold under the managed folders.
	MaxFiles = 1000

	maxPathBytes    = 240
	maxSegmentBytes = 100
	maxDepth        = 8
)

// Folders are the only top-level folders of a Workspace that Athena manages.
// brand/ holds the Workspace logos, written only through the Workspace
// identity; the file service refuses to change it.
var Folders = []string{"brand", "generated", "images", "packs", "record_types", "rules"}

// Extensions are the file types Athena stores. Everything is text except
// images: PNG, JPEG and WebP under images/, and a PNG logo under brand/.
var Extensions = []string{".go", ".jpeg", ".jpg", ".json", ".md", ".png", ".star", ".svg", ".txt", ".webp", ".yaml", ".yml"}

// AllowBinary reports whether p may hold non-text bytes: a PNG logo under
// brand/. An SVG logo is text and follows the text rules.
func AllowBinary(p string) bool {
	return strings.HasPrefix(p, "brand/") && path.Ext(p) == ".png"
}

// ImageExtensions are the image file types. Images live under images/ and
// images/ holds only images.
var ImageExtensions = []string{".jpeg", ".jpg", ".png", ".webp"}

// IsImagePath reports whether p is a valid path to an image file: cleaned,
// under images/, with an image extension.
func IsImagePath(p string) bool {
	rel, err := CleanPath(p)
	if err != nil {
		return false
	}
	folder, _, _ := strings.Cut(rel, "/")
	return folder == "images" && slices.Contains(ImageExtensions, path.Ext(rel))
}

// segmentPattern is one path segment: lowercase letters, digits, underscore,
// dash and dot, not starting with a dot. Uppercase is refused so that no two
// paths name one file on a case-insensitive disk.
var segmentPattern = regexp.MustCompile(`^[a-z0-9_][a-z0-9_.-]*$`)

// Errors a caller can map to a response. Each is wrapped with detail.
var (
	// ErrInvalidPath is a path that is not allowed, whatever the disk holds.
	ErrInvalidPath = errors.New("invalid path")
	ErrNotFound    = errors.New("file not found")
	ErrExists      = errors.New("file already exists")
	// ErrStale is a change made against a file that is no longer what the
	// caller read, or a change that lost a race with another one.
	ErrStale = errors.New("file changed")
	// ErrTooLarge is a file over MaxFileBytes.
	ErrTooLarge = errors.New("file too large")
	// ErrNotText is content that is not UTF-8 text.
	ErrNotText = errors.New("file is not text")
	// ErrNotImage is content that is not a PNG, JPEG or WebP image.
	ErrNotImage = errors.New("file is not an image")
	// ErrLimit is more files than MaxFiles.
	ErrLimit = errors.New("too many files")
	// ErrInvalidFile is content that fails its file format. See ValidationError.
	ErrInvalidFile = errors.New("invalid file")
)

func invalidPath(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidPath, fmt.Sprintf(format, args...))
}

// CleanPath returns p when it is a path Athena may manage and an error
// wrapping ErrInvalidPath otherwise. A valid path is relative, uses forward
// slashes, lies in one of Folders, has no empty, dot, dot-dot, hidden or
// non-lowercase segment, and ends in one of Extensions. The check is purely
// textual, so it holds on every operating system. Nothing is decoded, so an
// encoded separator such as %2f fails the character rule.
func CleanPath(p string) (string, error) {
	switch {
	case p == "":
		return "", invalidPath("the path is empty")
	case len(p) > maxPathBytes:
		return "", invalidPath("the path is longer than %d bytes", maxPathBytes)
	case !utf8.ValidString(p):
		return "", invalidPath("the path is not valid UTF-8")
	case strings.HasPrefix(p, "/"):
		return "", invalidPath("%q is absolute; use a path relative to the Workspace", p)
	case strings.ContainsRune(p, '\\'):
		return "", invalidPath("use / as the separator")
	}
	segments := strings.Split(p, "/")
	if len(segments) > maxDepth {
		return "", invalidPath("the path is deeper than %d folders", maxDepth)
	}
	for _, segment := range segments {
		switch {
		case segment == "":
			return "", invalidPath("%q has an empty segment", p)
		case segment == "." || segment == "..":
			return "", invalidPath("%q uses %q, which leaves the folder", p, segment)
		case strings.HasPrefix(segment, "."):
			return "", invalidPath("%q: names starting with a dot are not managed", p)
		case len(segment) > maxSegmentBytes:
			return "", invalidPath("a segment is longer than %d bytes", maxSegmentBytes)
		case !segmentPattern.MatchString(segment) || strings.HasSuffix(segment, "."):
			return "", invalidPath("%q: use lowercase letters, digits, _, - and . only", p)
		}
	}
	if len(segments) < 2 {
		return "", invalidPath("%q is not inside a folder; use one of %s", p, strings.Join(Folders, ", "))
	}
	if !slices.Contains(Folders, segments[0]) {
		return "", invalidPath("%q is not in a managed folder; use one of %s", p, strings.Join(Folders, ", "))
	}
	if !slices.Contains(Extensions, path.Ext(p)) {
		return "", invalidPath("%q: the file type is not allowed; use one of %s", p, strings.Join(Extensions, " "))
	}
	if folder, _, _ := strings.Cut(p, "/"); slices.Contains(ImageExtensions, path.Ext(p)) != (folder == "images" || (folder == "brand" && path.Ext(p) == ".png")) {
		if folder == "images" {
			return "", invalidPath("%q: images/ holds only PNG, JPEG and WebP images", p)
		}
		return "", invalidPath("%q: images live under images/", p)
	}
	return p, nil
}
