package lists

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

func testLogoPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 32), G: uint8(y * 32), B: 128, A: 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func goldenDocument(t *testing.T) ListDocument {
	t.Helper()
	rows := make([][]string, 0, 112)
	rows = append(rows, []string{"Wanjirũ Kamau", "Vegetarian, no nuts; please confirm the birthday cake is eggless and the frosting dairy-free", "2"})
	diets := []string{"None", "Vegan", "Halal", "Gluten-free"}
	for i := 1; i <= 111; i++ {
		rows = append(rows, []string{
			fmt.Sprintf("Guest %03d", i),
			diets[i%len(diets)],
			fmt.Sprintf("%d", i%4),
		})
	}
	return ListDocument{
		WorkspaceName: "Circle Space",
		LogoPNG:       testLogoPNG(t),
		Title:         "RSVP yes but missing diet",
		FilterText:    "response is yes",
		RenderedAt:    time.Date(2026, 11, 24, 7, 0, 0, 0, time.UTC),
		Columns:       []string{"Guest", "Dietary needs", "Plus-ones"},
		Rows:          rows,
	}
}

var (
	creationDate = regexp.MustCompile(`/CreationDate \(D:[^)]*\)`)
	modDate      = regexp.MustCompile(`/ModDate \(D:[^)]*\)`)
	docIDs       = regexp.MustCompile(`/ID \[<[0-9A-Fa-f]*><[0-9A-Fa-f]*>\]`)
)

func normalizePDF(raw []byte) []byte {
	out := creationDate.ReplaceAll(raw, []byte(`/CreationDate (D:00000000000000Z)`))
	out = modDate.ReplaceAll(out, []byte(`/ModDate (D:00000000000000Z)`))
	return docIDs.ReplaceAll(out, []byte(`/ID [<00000000000000000000000000000000><00000000000000000000000000000000>]`))
}

// TestRenderListPDFGolden renders a 3-page list: the header repeats per
// page, wrapped text stays inside cells, Inter is embedded (Wanjirũ's ũ
// maps through the ToUnicode CMap), and the normalized bytes match the
// committed golden file. Regenerate deliberately with UPDATE_GOLDEN=1 and
// re-verify the structural asserts below by eye.
func TestRenderListPDFGolden(t *testing.T) {
	got, err := RenderListPDF(goldenDocument(t))
	if err != nil {
		t.Fatal(err)
	}
	// gopdf writes "/Type /Page" per page and "/Type /Pages" once.
	pages := bytes.Count(got, []byte("/Type /Page")) - bytes.Count(got, []byte("/Type /Pages"))
	if pages != 3 {
		t.Fatalf("pages = %d, want 3", pages)
	}
	if !bytes.Contains(got, []byte("FontFile2")) {
		t.Fatal("no embedded TrueType font (FontFile2)")
	}
	if !bytes.Contains(got, []byte("0169")) {
		t.Fatal("ToUnicode map has no U+0169 (ũ)")
	}
	golden := filepath.Join("testdata", "lists-3page.pdf")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(golden, normalizePDF(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("golden rewritten: %s (%d bytes)", golden, len(got))
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden: %v (generate with UPDATE_GOLDEN=1)", err)
	}
	if !bytes.Equal(normalizePDF(got), want) {
		t.Fatalf("PDF differs from golden (%d vs %d bytes); inspect and regenerate with UPDATE_GOLDEN=1", len(got), len(want))
	}
}

func TestRenderListPDFRequiresColumns(t *testing.T) {
	if _, err := RenderListPDF(ListDocument{Title: "x"}); err == nil {
		t.Fatal("columnless list rendered")
	}
}
