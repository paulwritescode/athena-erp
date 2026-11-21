package brand

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Elements dropped with their whole subtree. filter is dropped rather than
// kept because its feImage child can load externally; an element that
// pointed at a dropped filter only loses the effect.
var dropElements = map[string]bool{
	"script": true, "style": true, "foreignobject": true, "filter": true,
}

// feElements are the filter primitives, dropped with filter.
var feElements = map[string]bool{
	"feblend": true, "fecolormatrix": true, "fecomponenttransfer": true,
	"fecomposite": true, "feconvolvematrix": true, "fediffuselighting": true,
	"fedisplacementmap": true, "fedistantlight": true, "fedropshadow": true,
	"feflood": true, "fefunca": true, "fefuncb": true, "fefuncg": true,
	"fefuncr": true, "fegaussianblur": true, "feimage": true, "femerge": true,
	"femergenode": true, "femorphology": true, "feoffset": true,
	"fepointlight": true, "fespecularlighting": true, "fespotlight": true,
	"fetile": true, "feturbulence": true,
}

// unwrapElements are dropped while their children stay: a link that cannot
// point anywhere still shows its content.
var unwrapElements = map[string]bool{"a": true}

// keepElements is what a logo may draw with. Anything else is dropped with
// its subtree.
var keepElements = map[string]bool{
	"svg": true, "g": true, "defs": true, "symbol": true, "use": true,
	"path": true, "rect": true, "circle": true, "ellipse": true, "line": true,
	"polyline": true, "polygon": true, "text": true, "tspan": true,
	"textpath": true, "tref": true, "altglyph": true,
	"clippath": true, "mask": true, "pattern": true, "marker": true,
	"lineargradient": true, "radialgradient": true, "stop": true,
	"title": true, "desc": true, "switch": true, "view": true,
	"font": true, "font-face": true, "glyph": true, "missing-glyph": true,
	"hkern": true, "vkern": true, "image": true,
	// No animate, animateTransform, animateMotion, mpath or set: a logo has
	// no business animating, and their attributeName/to/from/by/values can
	// rewrite href or event handlers at render time.
}

// urlAttrs may carry url() references, which must stay local.
var urlAttrs = map[string]bool{
	"fill": true, "stroke": true, "filter": true, "clip-path": true,
	"mask": true, "marker": true, "marker-start": true, "marker-mid": true,
	"marker-end": true, "color-profile": true, "cursor": true,
}

var urlRef = regexp.MustCompile(`(?i)url\(\s*(?:"([^"]*)"|'([^']*)'|([^)'"]*))\s*\)`)

// styleProps are the CSS declarations kept in a style attribute.
var styleProps = map[string]bool{
	"fill": true, "fill-opacity": true, "fill-rule": true, "clip-rule": true,
	"stroke": true, "stroke-width": true, "stroke-opacity": true,
	"stroke-linecap": true, "stroke-linejoin": true, "stroke-dasharray": true,
	"stroke-dashoffset": true, "stroke-miterlimit": true, "opacity": true,
	"font-family": true, "font-size": true, "font-weight": true,
	"font-style": true, "text-anchor": true, "letter-spacing": true,
	"word-spacing": true, "display": true, "visibility": true, "overflow": true,
	"color": true, "stop-color": true, "stop-opacity": true,
	"transform": true, "transform-box": true, "transform-origin": true,
}

// SanitizeSVG removes scripts, event handlers, external references,
// stylesheets and foreignObject from an SVG logo and returns the clean
// document. Comments, doctypes and processing instructions (including
// xml-stylesheet, which loads externally) are dropped too. The root must be
// <svg>; anything else is refused.
func SanitizeSVG(content []byte) ([]byte, error) {
	dec := xml.NewDecoder(bytes.NewReader(content))
	dec.Strict = true
	dec.Entity = xml.HTMLEntity
	var out bytes.Buffer
	enc := xml.NewEncoder(&out)
	depth := 0
	seenRoot := false
	// skip counts nested levels inside a dropped element.
	skip := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: the SVG does not parse: %v", ErrInvalid, err)
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(tok.Name.Local)
			if skip > 0 {
				skip++
				continue
			}
			if depth == 0 {
				if seenRoot || name != "svg" {
					return nil, fmt.Errorf("%w: the SVG root must be <svg>", ErrInvalid)
				}
				seenRoot = true
			}
			depth++
			// Elements outside the SVG namespace are editor metadata
			// (sodipodi, inkscape); dropping them cannot change rendering.
			if tok.Name.Space != "" && tok.Name.Space != svgNS {
				skip = 1
				depth--
				continue
			}
			switch {
			case dropElements[name] || feElements[name]:
				skip = 1
				depth--
			case unwrapElements[name]:
				depth--
			case !keepElements[name]:
				skip = 1
				depth--
			default:
				tok.Name.Space = ""
				tok.Attr = cleanAttrs(tok.Attr)
				if depth == 1 {
					tok.Attr = ensureXMLNS(tok.Attr)
				}
				if err := enc.EncodeToken(tok); err != nil {
					return nil, fmt.Errorf("%w: cannot rewrite the SVG: %v", ErrInvalid, err)
				}
			}
		case xml.EndElement:
			if skip > 0 {
				skip--
				continue
			}
			if unwrapElements[strings.ToLower(tok.Name.Local)] {
				continue
			}
			depth--
			tok.Name.Space = ""
			if err := enc.EncodeToken(tok); err != nil {
				return nil, fmt.Errorf("%w: cannot rewrite the SVG: %v", ErrInvalid, err)
			}
		case xml.CharData:
			if skip == 0 {
				if err := enc.EncodeToken(tok); err != nil {
					return nil, fmt.Errorf("%w: cannot rewrite the SVG: %v", ErrInvalid, err)
				}
			}
		case xml.Comment, xml.Directive, xml.ProcInst:
			// Dropped: no comments, doctypes, entities or stylesheets.
		}
	}
	if err := enc.Flush(); err != nil {
		return nil, fmt.Errorf("%w: cannot rewrite the SVG: %v", ErrInvalid, err)
	}
	if !seenRoot {
		return nil, fmt.Errorf("%w: the SVG has no root element", ErrInvalid)
	}
	return out.Bytes(), nil
}

const (
	svgNS   = "http://www.w3.org/2000/svg"
	xlinkNS = "http://www.w3.org/1999/xlink"
	xmlNS   = "http://www.w3.org/XML/1998/namespace"
)

// normalizeAttr maps a decoder-resolved attribute name back to a writable
// prefix. Editor-namespace attributes (inkscape, sodipodi) are dropped.
func normalizeAttr(attr xml.Attr) (xml.Attr, bool) {
	switch attr.Name.Space {
	case "", "xmlns":
		return attr, true
	case svgNS:
		attr.Name.Space = ""
		return attr, true
	case xlinkNS:
		attr.Name.Space = "xlink"
		return attr, true
	case xmlNS:
		attr.Name.Space = "xml"
		return attr, true
	default:
		return xml.Attr{}, false
	}
}

// ensureXMLNS gives the root a default namespace when it lacks one, so the
// document stands alone.
func ensureXMLNS(attrs []xml.Attr) []xml.Attr {
	for _, attr := range attrs {
		if attr.Name.Space == "" && attr.Name.Local == "xmlns" {
			return attrs
		}
	}
	return append([]xml.Attr{{Name: xml.Name{Local: "xmlns"}, Value: svgNS}}, attrs...)
}

// cleanAttrs drops event handlers and unsafe references from one element.
func cleanAttrs(attrs []xml.Attr) []xml.Attr {
	kept := attrs[:0]
	seen := map[xml.Name]bool{}
	for _, attr := range attrs {
		var ok bool
		if attr, ok = normalizeAttr(attr); !ok {
			continue
		}
		if seen[attr.Name] {
			continue
		}
		seen[attr.Name] = true
		local := strings.ToLower(attr.Name.Local)
		switch {
		case strings.HasPrefix(local, "on"):
			// Event handler.
		case local == "href" || local == "src":
			if localRef(attr.Value) {
				kept = append(kept, attr)
			}
		case local == "style":
			if clean := cleanStyle(attr.Value); clean != "" {
				attr.Value = clean
				kept = append(kept, attr)
			}
		case urlAttrs[local]:
			if localURLs(attr.Value) {
				kept = append(kept, attr)
			}
		default:
			kept = append(kept, attr)
		}
	}
	return kept
}

// localRef reports whether an href/src value stays inside the document: a
// fragment, or an embedded image. Control characters are stripped before the
// check so smuggled schemes cannot pass.
func localRef(value string) bool {
	return isLocalRef(stripControls(value))
}

func isLocalRef(value string) bool {
	trimmed := strings.TrimLeft(value, " \t\n\r\f\v")
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(trimmed, "#") || strings.HasPrefix(lower, "data:image/")
}

func stripControls(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value)
}

// localURLs reports whether every url() in value points at a fragment or an
// embedded image. Values without url() pass.
func localURLs(value string) bool {
	for _, match := range urlRef.FindAllStringSubmatch(value, -1) {
		target := match[1]
		if target == "" {
			target = match[2]
		}
		if target == "" {
			target = match[3]
		}
		if !isLocalRef(target) {
			return false
		}
	}
	// An unbalanced url( that the pattern missed is refused.
	lower := strings.ToLower(value)
	if strings.Contains(lower, "url(") && !strings.Contains(value, ")") {
		return false
	}
	return true
}

// cleanStyle keeps only safe declarations from a style attribute.
func cleanStyle(value string) string {
	var kept []string
	for _, decl := range strings.Split(value, ";") {
		prop, val, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		prop = strings.ToLower(strings.TrimSpace(prop))
		val = strings.TrimSpace(val)
		if prop == "" || val == "" || !styleProps[prop] {
			continue
		}
		lower := strings.ToLower(val)
		switch {
		case strings.ContainsAny(val, "<>"),
			strings.Contains(lower, "expression"),
			strings.Contains(lower, "javascript:"),
			strings.Contains(lower, "@import"),
			strings.Contains(lower, "behavior:"),
			strings.Contains(lower, "var("):
			continue
		case !localURLs(val):
			continue
		}
		kept = append(kept, prop+":"+val)
	}
	return strings.Join(kept, ";")
}

// dimensions returns the pixel size of a validated logo.
func dimensions(ext string, content []byte) (int, int, error) {
	if ext == "png" {
		cfg, err := pngConfig(content)
		if err != nil {
			return 0, 0, err
		}
		return cfg.Width, cfg.Height, nil
	}
	return svgDimensions(content)
}

// svgDimensions reads the width and height, or the viewBox, of sanitized SVG.
func svgDimensions(content []byte) (int, int, error) {
	dec := xml.NewDecoder(bytes.NewReader(content))
	for {
		tok, err := dec.Token()
		if err != nil {
			return 0, 0, fmt.Errorf("%w: the SVG logo needs a width and height or a viewBox", ErrInvalid)
		}
		start, ok := tok.(xml.StartElement)
		if !ok || strings.ToLower(start.Name.Local) != "svg" {
			continue
		}
		attrs := map[string]string{}
		for _, attr := range start.Attr {
			attrs[strings.ToLower(attr.Name.Local)] = attr.Value
		}
		if w, h, ok := svgSize(attrs["width"], attrs["height"]); ok {
			return w, h, nil
		}
		if w, h, ok := svgViewBox(attrs["viewbox"]); ok {
			return w, h, nil
		}
		return 0, 0, fmt.Errorf("%w: the SVG logo needs a width and height or a viewBox", ErrInvalid)
	}
}

func svgSize(width, height string) (int, int, bool) {
	w, ok := svgLength(width)
	if !ok {
		return 0, 0, false
	}
	h, ok := svgLength(height)
	if !ok {
		return 0, 0, false
	}
	return w, h, true
}

// svgLength parses a unitless or px length. Percentages and physical units
// say nothing about pixels, so they fall back to the viewBox.
func svgLength(value string) (int, bool) {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(strings.ToLower(value), "px")
	n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || n <= 0 || n != float64(int(n)) {
		return 0, false
	}
	return int(n), true
}

func svgViewBox(value string) (int, int, bool) {
	fields := strings.Fields(strings.ReplaceAll(value, ",", " "))
	if len(fields) != 4 {
		return 0, 0, false
	}
	var nums [4]float64
	for i, field := range fields {
		n, err := strconv.ParseFloat(field, 64)
		if err != nil {
			return 0, 0, false
		}
		nums[i] = n
	}
	w, h := nums[2], nums[3]
	if w <= 0 || h <= 0 || w != float64(int(w)) || h != float64(int(h)) {
		return 0, 0, false
	}
	return int(w), int(h), true
}
