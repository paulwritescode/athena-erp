// The Athena ASCII logo generator (task F6). It reads the brand artwork
// (web/src/assets/brand) and writes draw-in frames to
// web/src/art/logo-frames.ts, one set per theme. Each cell composites the
// ink over the theme background, takes the relative luminance, applies the
// theme gamma and maps it through a two-stop density ramp. The ramp holds
// no letters, so the art can never spell the name.
//
// Regenerate with:
//
//	go run ./tools/asciilogo
//
// Flags set the size, step and gammas; the defaults below are the shipped
// values, documented in docs/design/ascii-logo.md. The CLI banner in
// cmd/athena/main.go is a copy of the light final frame; the art tests
// keep the two in step.
package main

import (
	"flag"
	"fmt"
	"image"
	_ "image/png"
	"math"
	"os"
	"strings"
)

const (
	alphaCut = 8 // content bbox keeps pixels more opaque than this
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "asciilogo: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cols := flag.Int("cols", 40, "frame width in characters")
	rows := flag.Int("rows", 8, "frame height in lines (design 9.6 allows at most 8)")
	lightGamma := flag.Float64("light-gamma", 1.0, "gamma for the light-theme variant")
	darkGamma := flag.Float64("dark-gamma", 1.0, "gamma for the dark-theme variant")
	stepMs := flag.Int("step-ms", 180, "draw-in step in milliseconds (120 to 240)")
	lightSrc := flag.String("light-src", "web/src/assets/brand/athena-black.png", "black mark source")
	darkSrc := flag.String("dark-src", "web/src/assets/brand/athena-white.png", "white mark source")
	out := flag.String("out", "web/src/art/logo-frames.ts", "generated frames file")
	ramp := flag.String("ramp", " #", "density ramp, background first")
	flag.Parse()

	if *rows < 1 || *rows > 8 {
		return fmt.Errorf("rows = %d, want 1 to 8", *rows)
	}
	if *cols < 1 {
		return fmt.Errorf("cols = %d, want at least 1", *cols)
	}
	if *stepMs < 120 || *stepMs > 240 {
		return fmt.Errorf("step-ms = %d, want 120 to 240", *stepMs)
	}
	for _, r := range *ramp {
		if r < 0x20 || r > 0x7e {
			return fmt.Errorf("ramp %q is not plain ASCII", *ramp)
		}
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			return fmt.Errorf("ramp %q holds a letter", *ramp)
		}
	}

	light, err := load(*lightSrc)
	if err != nil {
		return err
	}
	dark, err := load(*darkSrc)
	if err != nil {
		return err
	}
	// Each variant is cropped to its own content box so the mark fills the
	// frame in both themes; only one variant shows at a time, so the two
	// sources are never overlaid.
	lightBox, darkBox := contentBox(light), contentBox(dark)
	if lightBox.Empty() {
		return fmt.Errorf("no content above the alpha cut in %s", *lightSrc)
	}
	if darkBox.Empty() {
		return fmt.Errorf("no content above the alpha cut in %s", *darkSrc)
	}

	// The background each theme composites over: white in the light theme,
	// black in the dark theme. Ink value is darkness on light, brightness
	// on dark, so dense cells always mean "the mark is here".
	lightArt := render(light, lightBox, *cols, *rows, *ramp, *lightGamma, [3]float64{1, 1, 1}, false)
	darkArt := render(dark, darkBox, *cols, *rows, *ramp, *darkGamma, [3]float64{0, 0, 0}, true)
	for _, line := range append(append([]string{}, lightArt...), darkArt...) {
		if len(line) > *cols {
			return fmt.Errorf("line %q is wider than %d columns", line, *cols)
		}
	}

	body := emit(*lightSrc, *darkSrc, *cols, *rows, *ramp, *lightGamma, *darkGamma, *stepMs, lightArt, darkArt)
	if err := os.WriteFile(*out, []byte(body), 0o644); err != nil {
		return err
	}
	diff := 0
	for r := range lightArt {
		a, b := lightArt[r], darkArt[r]
		for c := 0; c < len(a) || c < len(b); c++ {
			var ca, cb byte = ' ', ' '
			if c < len(a) {
				ca = a[c]
			}
			if c < len(b) {
				cb = b[c]
			}
			if ca != cb {
				diff++
			}
		}
	}
	fmt.Printf("wrote %s (%dx%d, %d frames per theme, variants differ in %d cells)\n", *out, *cols, *rows, *rows+1, diff)
	fmt.Println("--- light final ---")
	fmt.Println(strings.Join(lightArt, "\n"))
	fmt.Println("--- dark final ---")
	fmt.Println(strings.Join(darkArt, "\n"))
	return nil
}

func load(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return img, nil
}

// contentBox is the smallest rectangle holding every pixel more opaque
// than the alpha cut.
func contentBox(img image.Image) image.Rectangle {
	b := img.Bounds()
	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X-1, b.Min.Y-1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a>>8 > alphaCut {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if maxX < minX {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX+1, maxY+1)
}

// render maps the box to cols by rows cells. Every source pixel counts:
// each cell composites its pixels over the theme background, averages the
// relative luminance and maps the ink value through the ramp.
func render(img image.Image, box image.Rectangle, cols, rows int, ramp string, gamma float64, bg [3]float64, brightInk bool) []string {
	stops := len(ramp) - 1
	out := make([]string, rows)
	for r := 0; r < rows; r++ {
		y0 := box.Min.Y + (box.Dy()*r)/rows
		y1 := box.Min.Y + (box.Dy()*(r+1))/rows
		var line strings.Builder
		for c := 0; c < cols; c++ {
			x0 := box.Min.X + (box.Dx()*c)/cols
			x1 := box.Min.X + (box.Dx()*(c+1))/cols
			var lum float64
			var n float64
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					// RGBA() is alpha-premultiplied: straighten the ink
					// before compositing so edges are not darkened twice.
					rr, gg, bb, aa := img.At(x, y).RGBA()
					t := float64(aa>>8) / 255
					var ir, ig, ib float64
					if aa > 0 {
						ir = float64(rr) / float64(aa)
						ig = float64(gg) / float64(aa)
						ib = float64(bb) / float64(aa)
					}
					cr := ir*t + bg[0]*(1-t)
					cg := ig*t + bg[1]*(1-t)
					cb := ib*t + bg[2]*(1-t)
					lum += 0.2126*cr + 0.7152*cg + 0.0722*cb
					n++
				}
			}
			v := lum / n
			if !brightInk {
				v = 1 - v
			}
			v = math.Pow(v, gamma)
			idx := int(v*float64(stops) + 0.5)
			if idx < 0 {
				idx = 0
			}
			if idx > stops {
				idx = stops
			}
			line.WriteByte(ramp[idx])
		}
		out[r] = strings.TrimRight(line.String(), " ")
	}
	return out
}

// frames builds the draw-in sequence: frame k shows the first k rows and
// leaves the rest blank, so every frame has the full height and the logo
// never moves the page as it draws.
func frames(art []string) [][]string {
	out := make([][]string, 0, len(art)+1)
	for k := 0; k <= len(art); k++ {
		frame := make([]string, len(art))
		copy(frame, art[:k])
		out = append(out, frame)
	}
	return out
}

func emit(lightSrc, darkSrc string, cols, rows int, ramp string, lightGamma, darkGamma float64, stepMs int, lightArt, darkArt []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `// Code generated from %s and %s by tools/asciilogo; DO NOT EDIT.
// Regenerate with: go run ./tools/asciilogo
// Geometry: %d columns by %d rows over each source's own content box
// (alpha above %d). Ramp %q (%d stops): a cell takes the stop
// nearest its ink value, which is darkness composited over white for the
// light theme and brightness composited over black for the dark theme,
// after gamma (light %.2f, dark %.2f). Frames draw the mark top to bottom
// at %d ms a row; frame 0 is blank and later frames keep the full height
// with empty rows so the draw never moves the page. The CLI banner in
// cmd/athena/main.go copies the light final frame.
export const LOGO_COLS = %d
export const LOGO_ROWS = %d
export const LOGO_STEP_MS = %d
export const LOGO_RAMP = %s
`, lightSrc, darkSrc, cols, rows, alphaCut, ramp, len(ramp), lightGamma, darkGamma, stepMs, cols, rows, stepMs, quote(ramp))
	writeFrames(&b, "LOGO_LIGHT_FRAMES", frames(lightArt))
	writeFrames(&b, "LOGO_DARK_FRAMES", frames(darkArt))
	writeBlock(&b, "LOGO_LIGHT_BLOCK", lightArt)
	writeBlock(&b, "LOGO_DARK_BLOCK", darkArt)
	return b.String()
}

func writeFrames(b *strings.Builder, name string, fr [][]string) {
	fmt.Fprintf(b, "export const %s: readonly (readonly string[])[] = [\n", name)
	for _, frame := range fr {
		b.WriteString("  [")
		for i, line := range frame {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(quote(line))
		}
		b.WriteString("],\n")
	}
	b.WriteString("]\n")
}

func writeBlock(b *strings.Builder, name string, art []string) {
	fmt.Fprintf(b, "export const %s = [\n", name)
	for _, line := range art {
		fmt.Fprintf(b, "  %s,\n", quote(line))
	}
	b.WriteString("].join('\\n')\n")
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "\\'") + "'"
}
