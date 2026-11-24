// Package lists serves saved lists (ADR-0034): named condition trees over one
// Record Type with live lists, PDF export and outbox-queued messages.
package lists

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	_ "image/png"
	"time"

	"github.com/signintech/gopdf"
)

// Inter Regular and Bold, instantiated (wght 400/700, opsz 14) from the
// variable Inter at github.com/google/fonts (ofl/inter) on 2026-11-24 and
// vendored so PDF output never depends on the network. SIL OFL 1.1, see
// fonts/OFL.txt. Full coverage including Latin Extended (Wanjirũ's ũ).

//go:embed fonts/inter-regular.ttf
var interRegular []byte

//go:embed fonts/inter-bold.ttf
var interBold []byte

// ListDocument is everything RenderListPDF needs: the header facts, the
// columns, and the rows as display text (already formatted by the caller).
type ListDocument struct {
	WorkspaceName string
	LogoPNG       []byte // optional workspace logo; skipped when nil or undecodable
	Title         string
	FilterText    string // human description of the applied filter; "" hides the line
	RenderedAt    time.Time
	Columns       []string
	Rows          [][]string
}

const (
	pageW     = 595.0 // A4 portrait, points
	pageH     = 842.0
	margin    = 40.0
	headerGap = 12.0
	bodySize  = 9.0
	smallSize = 8.0
	lineGap   = 2.5
)

// RenderListPDF renders a list to PDF bytes: an A4 portrait with the
// workspace name, logo, list title, filter and render time in a header
// repeated on every page, and a table whose text wraps inside its cells.
// Deterministic in its input except the producer trailer, which tests
// normalize away.
func RenderListPDF(doc ListDocument) ([]byte, error) {
	if len(doc.Columns) == 0 {
		return nil, fmt.Errorf("a list needs at least one column")
	}
	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: gopdf.Rect{W: pageW, H: pageH}})
	if err := pdf.AddTTFFontData("inter", interRegular); err != nil {
		return nil, fmt.Errorf("load Inter: %w", err)
	}
	if err := pdf.AddTTFFontData("interb", interBold); err != nil {
		return nil, fmt.Errorf("load Inter bold: %w", err)
	}
	var logo image.Image
	if len(doc.LogoPNG) > 0 {
		if img, _, err := image.Decode(bytes.NewReader(doc.LogoPNG)); err == nil {
			logo = img
		}
	}
	widths := columnWidths(len(doc.Columns))
	r := renderer{pdf: &pdf, doc: doc, logo: logo, widths: widths}
	pdf.AddPage()
	r.header()
	for _, row := range doc.Rows {
		r.row(row)
	}
	return pdf.GetBytesPdf(), nil
}

func columnWidths(n int) []float64 {
	usable := pageW - 2*margin
	each := usable / float64(n)
	widths := make([]float64, n)
	for i := range widths {
		widths[i] = each
	}
	// Last column takes the rounding remainder so rules align.
	widths[n-1] = usable - each*float64(n-1)
	return widths
}

type renderer struct {
	pdf    *gopdf.GoPdf
	doc    ListDocument
	logo   image.Image
	widths []float64
}

func (r *renderer) textFamily(bold bool) string {
	if bold {
		return "interb"
	}
	return "inter"
}

func (r *renderer) header() {
	pdf := r.pdf
	y := margin
	if r.logo != nil {
		_ = pdf.ImageFrom(r.logo, margin, y, &gopdf.Rect{W: 28, H: 28})
	}
	textX := margin + 36
	if r.logo == nil {
		textX = margin
	}
	_ = pdf.SetFont(r.textFamily(false), "", smallSize)
	pdf.SetXY(textX, y)
	_ = pdf.Cell(&gopdf.Rect{W: pageW - textX - margin, H: 10}, r.doc.WorkspaceName)
	when := r.doc.RenderedAt.UTC().Format("2006-01-02 15:04 UTC")
	pdf.SetXY(textX, y+11)
	pdf.SetTextColor(110, 110, 110)
	_ = pdf.Cell(&gopdf.Rect{W: pageW - textX - margin, H: 10}, when)
	pdf.SetTextColor(0, 0, 0)
	_ = pdf.SetFont(r.textFamily(true), "", 14)
	pdf.SetXY(margin, y+26)
	_ = pdf.Cell(&gopdf.Rect{W: pageW - 2*margin, H: 16}, r.doc.Title)
	nextY := y + 44
	if r.doc.FilterText != "" {
		_ = pdf.SetFont(r.textFamily(false), "", smallSize)
		pdf.SetTextColor(110, 110, 110)
		pdf.SetXY(margin, nextY)
		_ = pdf.Cell(&gopdf.Rect{W: pageW - 2*margin, H: 10}, "Filter: "+r.doc.FilterText)
		pdf.SetTextColor(0, 0, 0)
		nextY += 12
	}
	pdf.SetXY(margin, nextY+headerGap)
	r.headerRow()
}

func (r *renderer) headerRow() {
	pdf := r.pdf
	_ = pdf.SetFont(r.textFamily(true), "", bodySize)
	y := pdf.GetY()
	h := bodySize + 2*lineGap
	x := margin
	for i, col := range r.doc.Columns {
		_ = pdf.Rectangle(x, y, x+r.widths[i], y+h, "D", 0, 0)
		pdf.SetXY(x+4, y+lineGap)
		_ = pdf.Cell(&gopdf.Rect{W: r.widths[i] - 8, H: bodySize + lineGap}, col)
		x += r.widths[i]
	}
	pdf.SetY(y + h)
}

func (r *renderer) row(cells []string) {
	pdf := r.pdf
	_ = pdf.SetFont(r.textFamily(false), "", bodySize)
	lineH := bodySize + lineGap
	wrapped := make([][]string, len(r.doc.Columns))
	lines := 1
	for i := range r.doc.Columns {
		text := ""
		if i < len(cells) {
			text = cells[i]
		}
		parts, err := pdf.SplitTextWithWordWrap(text, r.widths[i]-8)
		if err != nil || len(parts) == 0 {
			parts = []string{text}
		}
		wrapped[i] = parts
		if len(parts) > lines {
			lines = len(parts)
		}
	}
	h := float64(lines)*lineH + lineGap
	if pdf.GetY()+h > pageH-margin {
		pdf.AddPage()
		r.header()
	}
	y := pdf.GetY()
	x := margin
	for i := range r.doc.Columns {
		_ = pdf.Rectangle(x, y, x+r.widths[i], y+h, "D", 0, 0)
		for j, line := range wrapped[i] {
			pdf.SetXY(x+4, y+lineGap+float64(j)*lineH)
			_ = pdf.Cell(&gopdf.Rect{W: r.widths[i] - 8, H: lineH}, line)
		}
		x += r.widths[i]
	}
	pdf.SetY(y + h)
}
