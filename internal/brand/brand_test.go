package brand

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"
)

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestValidateLogoPNG(t *testing.T) {
	logo, err := ValidateLogo("logo.png", testPNG(t, 8, 8))
	if err != nil {
		t.Fatal(err)
	}
	if logo.Ext != "png" || len(logo.Bytes) == 0 || len(logo.Hash) != 64 {
		t.Fatalf("unexpected logo: %+v", logo)
	}
	if _, err := ValidateLogo("LOGO.PNG", testPNG(t, 8, 8)); err != nil {
		t.Fatalf("uppercase extension rejected: %v", err)
	}
}

func TestValidateLogoRejects(t *testing.T) {
	png := testPNG(t, 8, 8)
	for _, tc := range []struct {
		name    string
		file    string
		content []byte
		want    string
	}{
		{"bad extension", "logo.jpg", png, "not a logo"},
		{"no extension", "logo", png, "not a logo"},
		{"empty", "logo.png", nil, "empty"},
		{"too large", "logo.png", bytes.Repeat([]byte{0}, MaxLogoBytes+1), "larger than"},
		{"corrupt png", "logo.png", []byte("\x89PNG\r\n\x1a\njunk"), "does not decode"},
		{"text as png", "logo.png", []byte("hello"), "not a PNG"},
		{"too wide", "logo.png", testPNG(t, MaxDimension+1, 1), "outside"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ValidateLogo(tc.file, tc.content); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func TestSanitizeSVG(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      string
		keep    []string
		drop    []string
		wantErr bool
	}{
		{
			name: "script removed",
			in:   `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><script>alert(1)</script><rect width="8" height="8"/></svg>`,
			keep: []string{"<rect"},
			drop: []string{"script", "alert"},
		},
		{
			name: "event handlers removed",
			in:   `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><rect width="8" height="8" onload="evil()" ONCLICK="evil()"/></svg>`,
			keep: []string{"<rect"},
			drop: []string{"onload", "ONCLICK", "evil"},
		},
		{
			name: "http href removed",
			in:   `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="8" height="8"><image xlink:href="http://evil/x.png" width="8" height="8"/></svg>`,
			keep: []string{"<image"},
			drop: []string{"http://evil"},
		},
		{
			name: "javascript href removed",
			in:   `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><a href="java&#x09;script:alert(1)"><rect width="8" height="8"/></a></svg>`,
			keep: []string{"<rect"},
			drop: []string{"<a", "script:alert"},
		},
		{
			name: "foreignObject removed",
			in:   `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><foreignObject><body xmlns="http://www.w3.org/1999/xhtml">hi</body></foreignObject><rect width="8" height="8"/></svg>`,
			keep: []string{"<rect"},
			drop: []string{"foreignObject", ">hi<"},
		},
		{
			name: "fragment and data refs kept",
			in:   `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><defs><linearGradient id="g"><stop offset="0"/></linearGradient></defs><rect width="8" height="8" fill="url(#g)"/><use href="#g"/></svg>`,
			keep: []string{"url(#g)", `href="#g"`},
		},
		{
			name: "external paint dropped",
			in:   `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><rect width="8" height="8" fill="url(http://evil/g)"/></svg>`,
			keep: []string{"<rect"},
			drop: []string{"http://evil"},
		},
		{
			name: "style element dropped, style attribute filtered",
			in:   `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><style>.a{fill:red}</style><rect width="8" height="8" style="fill:red;width:expression(evil)"/></svg>`,
			keep: []string{"fill:red"},
			drop: []string{"<style", "expression"},
		},
		{
			name: "processing instructions dropped",
			in:   `<?xml version="1.0"?><?xml-stylesheet href="http://evil/x.css"?><svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><rect width="8" height="8"/></svg>`,
			keep: []string{"<rect"},
			drop: []string{"xml-stylesheet", "http://evil"},
		},
		{
			name: "link unwrapped",
			in:   `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><a href="#x"><rect width="8" height="8"/></a></svg>`,
			keep: []string{"<rect"},
			drop: []string{"<a"},
		},
		{
			name: "unknown elements dropped",
			in:   `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><blink><rect width="8" height="8"/></blink></svg>`,
			drop: []string{"blink", "<rect"},
		},
		{
			name: "namespaces round-trip without duplication",
			in:   `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="8" height="8"><defs><linearGradient id="g"><stop offset="0"/></linearGradient></defs><use xlink:href="#g"/><rect width="8" height="8"/></svg>`,
			keep: []string{`xlink:href="#g"`},
			drop: []string{`xmlns="http://www.w3.org/2000/svg" xmlns=`},
		},
		{
			name: "editor namespaces dropped",
			in:   `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="8" height="8"><sodipodi:namedview xmlns:sodipodi="http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd"/><g inkscape:label="x"><rect width="8" height="8"/></g></svg>`,
			keep: []string{"<rect"},
			drop: []string{"sodipodi", "inkscape:label", "namedview"},
		},
		{
			name: "root gains a namespace when missing",
			in:   `<svg width="8" height="8"><rect width="8" height="8"/></svg>`,
			keep: []string{`xmlns="http://www.w3.org/2000/svg"`},
		},
		{
			name:    "non-svg root refused",
			in:      `<html><rect width="8" height="8"/></html>`,
			wantErr: true,
		},
		{
			name:    "unparseable refused",
			in:      `<svg><rect`,
			wantErr: true,
		},
		{
			name:    "empty refused",
			in:      ``,
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := SanitizeSVG([]byte(tc.in))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.keep {
				if !strings.Contains(string(out), want) {
					t.Fatalf("output lost %q:\n%s", want, out)
				}
			}
			for _, drop := range tc.drop {
				if strings.Contains(string(out), drop) {
					t.Fatalf("output kept %q:\n%s", drop, out)
				}
			}
		})
	}
}

func TestValidateLogoSVGDimensions(t *testing.T) {
	sized := `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="8"><rect width="16" height="8"/></svg>`
	if _, err := ValidateLogo("logo.svg", []byte(sized)); err != nil {
		t.Fatalf("sized SVG rejected: %v", err)
	}
	viewBox := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 8"><rect width="16" height="8"/></svg>`
	if _, err := ValidateLogo("logo.svg", []byte(viewBox)); err != nil {
		t.Fatalf("viewBox SVG rejected: %v", err)
	}
	unsized := `<svg xmlns="http://www.w3.org/2000/svg"><rect width="16" height="8"/></svg>`
	if _, err := ValidateLogo("logo.svg", []byte(unsized)); err == nil {
		t.Fatal("unsized SVG accepted")
	}
}

func TestFavicon(t *testing.T) {
	src := testPNG(t, 64, 32)
	first, err := Favicon(src, FaviconSmall)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Favicon(src, FaviconSmall)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("favicon output is not deterministic")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != FaviconSmall || cfg.Height != FaviconSmall {
		t.Fatalf("favicon is %dx%d", cfg.Width, cfg.Height)
	}
	large, err := Favicon(src, FaviconLarge)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = png.DecodeConfig(bytes.NewReader(large))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != FaviconLarge || cfg.Height != FaviconLarge {
		t.Fatalf("favicon is %dx%d", cfg.Width, cfg.Height)
	}
	if _, err := Favicon([]byte("nope"), FaviconSmall); err == nil {
		t.Fatal("corrupt source accepted")
	}
}

// The embedded mark favicons must match what the Desk mark renders to, so
// they cannot drift from web/src/assets/brand.
func TestMarkFaviconsMatchDeskMark(t *testing.T) {
	src, err := os.ReadFile("../../web/src/assets/brand/athena-black.png")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{FaviconSmall, FaviconLarge} {
		want, err := Favicon(src, size)
		if err != nil {
			t.Fatal(err)
		}
		got, err := MarkFavicon(size)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("mark%d.png drifted from the Desk mark; re-render it", size)
		}
	}
	if _, err := MarkFavicon(16); err == nil {
		t.Fatal("unknown favicon size accepted")
	}
}
