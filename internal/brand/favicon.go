package brand

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

// mark32 and mark180 are the Athena black mark rendered once with Favicon
// from web/src/assets/brand/athena-black.png. The suite regenerates them
// from that file, so they cannot drift from the Desk mark.
//
//go:embed mark32.png
var mark32 []byte

//go:embed mark180.png
var mark180 []byte

// MarkFavicon returns the Athena mark favicon at 32 or 180 pixels, the
// fallback when the Workspace has no PNG light logo.
func MarkFavicon(size int) ([]byte, error) {
	switch size {
	case FaviconSmall:
		return mark32, nil
	case FaviconLarge:
		return mark180, nil
	default:
		return nil, fmt.Errorf("unknown favicon size %d", size)
	}
}

func hash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func pngConfig(content []byte) (image.Config, error) {
	if len(content) < 8 || string(content[:8]) != "\x89PNG\r\n\x1a\n" {
		return image.Config{}, fmt.Errorf("%w: not a PNG file", ErrInvalid)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return image.Config{}, fmt.Errorf("%w: the PNG does not decode: %v", ErrInvalid, err)
	}
	return cfg, nil
}

// Favicon renders PNG bytes at size by size pixels with an area-average
// downscale, so edges stay smooth. The output is deterministic in its input.
func Favicon(content []byte, size int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("%w: the PNG does not decode: %v", ErrInvalid, err)
	}
	bounds := src.Bounds()
	sw, sh := bounds.Dx(), bounds.Dy()
	if sw <= 0 || sh <= 0 {
		return nil, fmt.Errorf("%w: the PNG has no pixels", ErrInvalid)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		y0 := bounds.Min.Y + y*sh/size
		y1 := bounds.Min.Y + (y+1)*sh/size
		for x := 0; x < size; x++ {
			x0 := bounds.Min.X + x*sw/size
			x1 := bounds.Min.X + (x+1)*sw/size
			var r, g, b, a uint64
			var n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					pr, pg, pb, pa := src.At(sx, sy).RGBA()
					r, g, b, a, n = r+uint64(pr), g+uint64(pg), b+uint64(pb), a+uint64(pa), n+1
				}
			}
			if n == 0 {
				pr, pg, pb, pa := src.At(x0, y0).RGBA()
				r, g, b, a, n = uint64(pr), uint64(pg), uint64(pb), uint64(pa), 1
			}
			dst.SetNRGBA(x, y, color.NRGBA{R: uint8(r / n >> 8), G: uint8(g / n >> 8), B: uint8(b / n >> 8), A: uint8(a / n >> 8)})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, fmt.Errorf("render the favicon: %w", err)
	}
	return out.Bytes(), nil
}
