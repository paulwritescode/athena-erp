package appearance

import (
	"errors"
	"math"
	"testing"

	v1 "athena-erp/gen/athena/v1"
)

func TestAppearanceContrastAndNearestLightness(t *testing.T) {
	// For neutral OKLCH, linear sRGB luminance is L cubed. This gives an
	// independent boundary for the light theme.
	want := math.Cbrt((math.Pow(.985, 3)+.05)/4.5 - .05)
	_, err := Resolve(&v1.Appearance{Preset: "custom", Accent: "oklch(90% 0 0)"})
	var fail *ContrastError
	if !errors.As(err, &fail) {
		t.Fatalf("wanted a contrast failure: %v", err)
	}
	c, err := parse(fail.Suggested)
	if err != nil || c.l > want || want-c.l > 1e-6 {
		t.Fatalf("suggestion %s, expected %.8f", fail.Suggested, want)
	}
	for _, input := range []string{fail.Suggested, "oklch(0.42 0.13 22)", "oklch(0.21 0.006 285.885)", "oklch(0.1 0.4 85)"} {
		p, err := Resolve(&v1.Appearance{Preset: "custom", Accent: input})
		if err != nil {
			t.Fatal(err)
		}
		light, e1 := parse(p.Light)
		dark, e2 := parse(p.Dark)
		if e1 != nil || e2 != nil || (lightText.luminance()+.05)/(light.luminance()+.05) < 4.5 || (dark.luminance()+.05)/(darkText.luminance()+.05) < 4.5 {
			t.Fatalf("invalid theme pair for %s: %v", input, p)
		}
	}
}

func TestAppearanceInput(t *testing.T) {
	for _, a := range []*v1.Appearance{nil, {Preset: "unknown"}, {Preset: "red", Accent: "oklch(0.2 0 0)"}, {Preset: "blue", ChartPalette: "unknown"}, {Preset: "blue", ChartPalette: "ocean", ChartColors: []string{"oklch(0.2 0 0)"}}, {Preset: "custom", Accent: "oklch(0.2 0 0); color:red"}, {Preset: "custom", Accent: "oklch(0.2 0 0)", ChartColors: []string{"red"}}, {Preset: "custom", Accent: "oklch(0.2 0 0)", ChartColors: make([]string, 6)}} {
		if _, err := Resolve(a); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted invalid appearance %v: %v", a, err)
		}
	}
}

func TestAppearancePresetAndPaletteLists(t *testing.T) {
	if len(Presets) != 10 || Presets[0] != "ink" || Presets[len(Presets)-1] != "custom" {
		t.Fatalf("accent list drifted: %v", Presets)
	}
	if len(ChartPalettes) != 6 || ChartPalettes[0] != "match" || ChartPalettes[len(ChartPalettes)-1] != "custom" {
		t.Fatalf("chart list drifted: %v", ChartPalettes)
	}
	for _, preset := range Presets[:9] {
		for _, palette := range ChartPalettes[:5] {
			if _, err := Resolve(&v1.Appearance{Preset: preset, ChartPalette: palette}); err != nil {
				t.Fatalf("%s/%s: %v", preset, palette, err)
			}
		}
	}
	// Appearances saved before palettes existed read back with their charts.
	p, err := Resolve(&v1.Appearance{Preset: "red", ChartColors: []string{"oklch(0.2 0 0)"}})
	if err != nil || p != (Palette{}) {
		t.Fatalf("legacy custom charts: %v %v", p, err)
	}
	if _, err := Resolve(&v1.Appearance{Preset: "custom", Accent: "oklch(0.2 0 0)", ChartPalette: "custom", ChartColors: []string{"oklch(0.3 0 0)"}}); err != nil {
		t.Fatalf("custom charts: %v", err)
	}
}
