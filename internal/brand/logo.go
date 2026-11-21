// Package brand validates Workspace logos, sanitizes SVGs and renders
// favicons. A logo is a PNG or an SVG of at most 512 KB (R5.4) with checked
// dimensions. SVG passes through SanitizeSVG, which removes scripts, event
// handlers, external references and foreignObject; serving adds a sandboxed
// content security policy on top.
package brand

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"path"
	"strings"
)

const (
	// MaxLogoBytes is the largest logo Athena stores.
	MaxLogoBytes = 512 * 1024
	// MaxDimension is the largest logo side in pixels.
	MaxDimension = 2048
	// FaviconSmall and FaviconLarge are the generated favicon sizes.
	FaviconSmall = 32
	FaviconLarge = 180
)

// ErrInvalid is a logo that fails validation, wrapped with the reason.
var ErrInvalid = errors.New("invalid logo")

// Logo is a validated logo, ready to store under brand/.
type Logo struct {
	// Ext is "png" or "svg".
	Ext string
	// Bytes is the PNG as given, or the sanitized SVG.
	Bytes []byte
	// Hash is the SHA-256 of Bytes as lowercase hex.
	Hash string
}

// ValidateLogo checks filename and content and returns the storable logo.
// The file type comes from the filename extension. A PNG must decode with
// sides from 1 to MaxDimension pixels; an SVG must sanitize and carry a
// width and height or a viewBox within the same bounds.
func ValidateLogo(filename string, content []byte) (Logo, error) {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(filename), "."))
	switch ext {
	case "png", "svg":
	default:
		return Logo{}, fmt.Errorf("%w: %q is not a logo; use a .png or .svg file", ErrInvalid, filename)
	}
	if len(content) > MaxLogoBytes {
		return Logo{}, fmt.Errorf("%w: %q is larger than %d bytes", ErrInvalid, filename, MaxLogoBytes)
	}
	if len(content) == 0 {
		return Logo{}, fmt.Errorf("%w: %q is empty", ErrInvalid, filename)
	}
	clean := content
	if ext == "svg" {
		var err error
		clean, err = SanitizeSVG(content)
		if err != nil {
			return Logo{}, err
		}
	}
	width, height, err := dimensions(ext, clean)
	if err != nil {
		return Logo{}, err
	}
	if width < 1 || height < 1 || width > MaxDimension || height > MaxDimension {
		return Logo{}, fmt.Errorf("%w: dimensions %dx%d are outside 1 to %d pixels", ErrInvalid, width, height, MaxDimension)
	}
	if ext == "png" {
		// Decode fully so a corrupt file fails here, not when a favicon is
		// rendered from it.
		if _, _, err := image.Decode(bytes.NewReader(clean)); err != nil {
			return Logo{}, fmt.Errorf("%w: the PNG does not decode: %v", ErrInvalid, err)
		}
	}
	return Logo{Ext: ext, Bytes: clean, Hash: hash(clean)}, nil
}
