// Package appearance resolves a Workspace's accent tokens for both themes.
package appearance

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	v1 "athena-erp/gen/athena/v1"
)

var ErrInvalid = errors.New("invalid appearance")

type ContrastError struct{ Suggested, Dark string }

func (e *ContrastError) Error() string {
	return "Text is hard to read on this accent. Try " + e.Suggested + "."
}

type color struct{ l, c, h float64 }

const number = `([0-9]+(?:\.[0-9]+)?|\.[0-9]+)`

var syntax = regexp.MustCompile(`^oklch\(\s*` + number + `(%)?\s+` + number + `\s+(-?(?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+))(?:deg)?\s*\)$`)

func parse(s string) (color, error) {
	invalid := fmt.Errorf("%w: use oklch(lightness chroma hue)", ErrInvalid)
	if len(s) > 100 {
		return color{}, invalid
	}
	m := syntax.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return color{}, invalid
	}
	l, e1 := strconv.ParseFloat(m[1], 64)
	c, e2 := strconv.ParseFloat(m[3], 64)
	h, e3 := strconv.ParseFloat(m[4], 64)
	if m[2] == "%" {
		l /= 100
	}
	if e1 != nil || e2 != nil || e3 != nil || l > 1 || c > 0.4 {
		return color{}, invalid
	}
	return color{l, c, math.Mod(h, 360)}, nil
}
func (c color) String() string {
	format := func(x float64) string { return strconv.FormatFloat(x, 'f', -1, 64) }
	return "oklch(" + format(c.l) + " " + format(c.c) + " " + format(c.h) + ")"
}

// Linear sRGB channels from the inverse OKLab transform. The WCAG relative
// luminance weights operate on linear light, without an sRGB gamma step.
func (c color) rgb() (float64, float64, float64) {
	h := c.h * math.Pi / 180
	a, b := c.c*math.Cos(h), c.c*math.Sin(h)
	cube := func(x float64) float64 { return x * x * x }
	l, m, s := cube(c.l+0.3963377774*a+0.2158037573*b), cube(c.l-0.1055613458*a-0.0638541728*b), cube(c.l-0.0894841775*a-1.2914855480*b)
	return 4.0767416621*l - 3.3077115913*m + 0.2309699292*s, -1.2684380046*l + 2.6097574011*m - 0.3413193965*s, -0.0041960863*l - 0.7034186147*m + 1.7076147010*s
}

// fitSRGB keeps hue and lightness, reducing chroma only when necessary.
// Returning the fitted token makes contrast independent of the browser's
// choice of gamut mapping on different displays.
func (c color) fitSRGB() color {
	inside := func(x color) bool {
		r, g, b := x.rgb()
		return min(r, g, b) >= -1e-10 && max(r, g, b) <= 1+1e-10
	}
	if inside(c) {
		return c
	}
	lo, hi := 0., c.c
	for range 32 {
		c.c = (lo + hi) / 2
		if inside(c) {
			lo = c.c
		} else {
			hi = c.c
		}
	}
	c.c = lo
	return c
}

func (c color) luminance() float64 {
	r, g, b := c.rgb()
	clip := func(v float64) float64 { return math.Max(0, math.Min(1, v)) }
	return .2126*clip(r) + .7152*clip(g) + .0722*clip(b)
}

var lightText = color{.985, 0, 0}
var darkText = color{.21, .006, 285.885}

// nearest searches the lightness boundary at fixed chroma and hue. Round
// toward the passing side so the color returned by the API still passes.
func nearest(c color, dark bool) color {
	pass := func(x color) bool {
		x = x.fitSRGB()
		if dark {
			return (x.luminance()+.05)/(darkText.luminance()+.05) >= 4.5
		}
		return (lightText.luminance()+.05)/(x.luminance()+.05) >= 4.5
	}
	if pass(c) {
		return c
	}
	lo, hi := 0, 1000000
	for lo < hi {
		mid := (lo + hi) / 2
		x := c
		x.l = float64(mid) / 1e6
		if !dark {
			x.l = float64(1000000-mid) / 1e6
		}
		if pass(x) {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	if !dark {
		lo = 1000000 - lo
	}
	c.l = float64(lo) / 1e6
	return c
}

type Palette struct{ Light, Dark string }

// Resolve validates the saved choice and derives displayable custom tokens.
func Resolve(a *v1.Appearance) (Palette, error) {
	switch a.GetPreset() {
	case "ink", "red":
		if a.Accent != "" || len(a.ChartColors) > 0 {
			return Palette{}, fmt.Errorf("%w: choose Custom to set colors", ErrInvalid)
		}
		return Palette{}, nil
	case "custom":
	default:
		return Palette{}, fmt.Errorf("%w: choose ink, red or custom", ErrInvalid)
	}
	if len(a.ChartColors) > 5 {
		return Palette{}, fmt.Errorf("%w: choose up to five chart colors", ErrInvalid)
	}
	for i, s := range a.ChartColors {
		if s != "" {
			if _, err := parse(s); err != nil {
				return Palette{}, fmt.Errorf("chart %d: %w", i+1, err)
			}
		}
	}
	c, err := parse(a.Accent)
	if err != nil {
		return Palette{}, err
	}
	light := nearest(c, false)
	dark := nearest(light, true).fitSRGB()
	if c != light {
		return Palette{}, &ContrastError{light.String(), dark.String()}
	}
	return Palette{c.fitSRGB().String(), dark.String()}, nil
}
