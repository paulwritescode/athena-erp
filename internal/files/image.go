// Package files manages the source files of one Workspace, including the
// images under images/. This file is the image pipeline: sniffing,
// EXIF-aware cleaning and thumbnails. PNG and JPEG use the standard image
// packages; WebP decode uses golang.org/x/image, which is pure Go and keeps
// the single binary free of cgo.
package files

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"strings"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	// ThumbEdge is the long edge of a thumbnail in pixels. Smaller images
	// are served as they are, never upscaled.
	ThumbEdge = 256
	// JPEGReencodeQuality keeps re-encoded uploads near-lossless.
	JPEGReencodeQuality = 95
	// JPEGThumbQuality trades thumbnail bytes for size.
	JPEGThumbQuality = 85

	// maxImageDimension and maxImagePixels bound a decode before it runs,
	// so a tiny file claiming huge dimensions cannot exhaust memory.
	maxImageDimension = 16384
	maxImagePixels    = 25_000_000
)

// Sniff reports the image format of content: png, jpeg or webp. Anything
// else, including text and executables wearing an image name, is "".
func Sniff(content []byte) string {
	switch http.DetectContentType(content) {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpeg"
	case "image/webp":
		return "webp"
	}
	return ""
}

// DecodeImage decodes content after Sniff has accepted it. Bounds come from
// DecodeConfig first so corrupt dimensions fail before allocating pixels.
func DecodeImage(content []byte) (image.Image, string, error) {
	format := Sniff(content)
	if format == "" {
		return nil, "", fmt.Errorf("%w: not a PNG, JPEG or WebP image", ErrNotImage)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return nil, "", fmt.Errorf("%w: cannot read the image: %v", ErrNotImage, err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxImageDimension || config.Height > maxImageDimension || int64(config.Width)*int64(config.Height) > maxImagePixels {
		return nil, "", fmt.Errorf("%w: dimensions %dx%d are not supported", ErrNotImage, config.Width, config.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return nil, "", fmt.Errorf("%w: cannot read the image: %v", ErrNotImage, err)
	}
	return img, format, nil
}

// CleanedImage is an upload ready to store: metadata-free bytes in the
// source format with the dimensions the pixels now have.
type CleanedImage struct {
	Content []byte
	Format  string
	Width   int
	Height  int
}

// CleanImage prepares uploaded bytes for storage: it verifies the format,
// applies EXIF orientation, strips metadata and reports the result.
// Unchanged files keep their exact bytes; only files with metadata or a
// rotated orientation are re-encoded.
func CleanImage(content []byte) (CleanedImage, error) {
	img, format, err := DecodeImage(content)
	if err != nil {
		return CleanedImage{}, err
	}
	switch format {
	case "png":
		return cleanPNG(content, img)
	case "jpeg":
		return cleanJPEG(content, img)
	case "webp":
		return cleanWebP(content)
	}
	return CleanedImage{}, fmt.Errorf("%w: unreachable format %q", ErrNotImage, format)
}

// pngSignature is the 8 magic bytes of every PNG file.
var pngSignature = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}

// pngDropped holds ancillary chunk types that carry metadata rather than
// pixels or color intent: text, EXIF and timestamps. Color chunks (gAMA,
// cHRM, sRGB, iCCP) and everything else stay, so rendering never changes.
var pngDropped = map[string]bool{"tEXt": true, "zTXt": true, "iTXt": true, "eXIf": true, "tIME": true}

// cleanPNG drops metadata chunks and returns the original bytes when there
// is nothing to drop.
func cleanPNG(content []byte, img image.Image) (CleanedImage, error) {
	cleaned, dropped, err := filterPNGChunks(content)
	if err != nil {
		var out bytes.Buffer
		if err := png.Encode(&out, img); err != nil {
			return CleanedImage{}, fmt.Errorf("%w: cannot rewrite the image: %v", ErrNotImage, err)
		}
		return CleanedImage{Content: out.Bytes(), Format: "png", Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}, nil
	}
	if !dropped {
		return CleanedImage{Content: content, Format: "png", Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}, nil
	}
	return CleanedImage{Content: cleaned, Format: "png", Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}, nil
}

// filterPNGChunks copies a PNG without its metadata chunks. Kept chunks,
// CRCs included, pass through untouched.
func filterPNGChunks(content []byte) (cleaned []byte, dropped bool, err error) {
	if !bytes.HasPrefix(content, pngSignature) {
		return nil, false, fmt.Errorf("not a PNG file")
	}
	out := append([]byte(nil), pngSignature...)
	rest := content[len(pngSignature):]
	for len(rest) > 0 {
		if len(rest) < 12 {
			return nil, false, fmt.Errorf("truncated chunk")
		}
		length := binary.BigEndian.Uint32(rest)
		chunkType := string(rest[4:8])
		if uint64(length) > uint64(len(rest)-12) {
			return nil, false, fmt.Errorf("truncated chunk")
		}
		end := 8 + int(length) + 4
		if pngDropped[chunkType] {
			dropped = true
		} else {
			out = append(out, rest[:end]...)
		}
		rest = rest[end:]
		if chunkType == "IEND" {
			break
		}
	}
	return out, dropped, nil
}

// cleanJPEG strips metadata and applies EXIF orientation. An oriented photo
// is decoded, rotated and re-encoded, because dropping the orientation tag
// without rotating would lay the photo sideways. Anything else keeps its
// exact bytes apart from dropped metadata segments.
func cleanJPEG(content []byte, img image.Image) (CleanedImage, error) {
	// jpegOrientation only returns 1-8; anything unreadable reports 1.
	if orientation := jpegOrientation(content); orientation != 1 {
		return reencodeJPEG(orientImage(img, orientation))
	}
	cleaned, err := filterJPEGSegments(content)
	if err != nil {
		return reencodeJPEG(img)
	}
	return CleanedImage{Content: cleaned, Format: "jpeg", Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}, nil
}

// reencodeJPEG writes pixels without any metadata.
func reencodeJPEG(img image.Image) (CleanedImage, error) {
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: JPEGReencodeQuality}); err != nil {
		return CleanedImage{}, fmt.Errorf("%w: cannot rewrite the image: %v", ErrNotImage, err)
	}
	return CleanedImage{Content: out.Bytes(), Format: "jpeg", Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}, nil
}

// jpegOrientation reads the EXIF orientation of JPEG bytes: 1 means upright
// or absent, 2-8 a rotation. Anything unreadable reports 1; the metadata is
// still stripped by the caller.
func jpegOrientation(content []byte) int {
	payload, ok := jpegAPP1(content, "Exif\x00\x00")
	if !ok {
		return 1
	}
	tiff := payload[len("Exif\x00\x00"):]
	if len(tiff) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	if order.Uint16(tiff[2:]) != 42 {
		return 1
	}
	at := int(order.Uint32(tiff[4:]))
	if at < 8 || at+2 > len(tiff) {
		return 1
	}
	count := int(order.Uint16(tiff[at:]))
	at += 2
	for i := 0; i < count; i++ {
		if at+12 > len(tiff) {
			return 1
		}
		if tag := order.Uint16(tiff[at:]); tag == 0x0112 {
			if order.Uint16(tiff[at+2:]) != 3 || order.Uint32(tiff[at+4:]) != 1 {
				return 1
			}
			orientation := int(order.Uint16(tiff[at+8:]))
			if orientation >= 1 && orientation <= 8 {
				return orientation
			}
			return 1
		}
		at += 12
	}
	return 1
}

// jpegAPP1 returns the payload of the first APP1 segment with the given
// header. JPEG structure errors report ok as false.
func jpegAPP1(content []byte, header string) ([]byte, bool) {
	pos, ok := jpegScanStart(content)
	if !ok {
		return nil, false
	}
	for pos+4 <= len(content) {
		if content[pos] != 0xff {
			return nil, false
		}
		for pos+1 < len(content) && content[pos+1] == 0xff {
			pos++
		}
		if pos+1 >= len(content) {
			return nil, false
		}
		marker := content[pos+1]
		pos += 2
		if marker == 0xda {
			return nil, false
		}
		if marker == 0xd9 || marker == 0x01 || (marker >= 0xd0 && marker <= 0xd7) {
			continue
		}
		if pos+2 > len(content) {
			return nil, false
		}
		length := int(content[pos])<<8 | int(content[pos+1])
		if length < 2 || pos+length > len(content) {
			return nil, false
		}
		if marker == 0xe1 && strings.HasPrefix(string(content[pos+2:pos+length]), header) {
			return content[pos+2 : pos+length], true
		}
		pos += length
	}
	return nil, false
}

// jpegScanStart skips the SOI marker and padding, returning the first
// segment. JPEG files start with FFD8 by definition.
func jpegScanStart(content []byte) (int, bool) {
	if len(content) < 2 || content[0] != 0xff || content[1] != 0xd8 {
		return 0, false
	}
	return 2, true
}

// filterJPEGSegments copies a JPEG without its Exif and XMP metadata
// segments. Color profiles and everything else pass through untouched.
func filterJPEGSegments(content []byte) ([]byte, error) {
	pos, ok := jpegScanStart(content)
	if !ok {
		return nil, fmt.Errorf("not a JPEG file")
	}
	var out bytes.Buffer
	out.Write(content[:pos])
	for pos+2 <= len(content) {
		for pos < len(content) && content[pos] != 0xff {
			return nil, fmt.Errorf("expected a marker")
		}
		for pos+1 < len(content) && content[pos] == 0xff && content[pos+1] == 0xff {
			pos++
		}
		if pos+2 > len(content) {
			return nil, fmt.Errorf("truncated marker")
		}
		marker := content[pos+1]
		if marker == 0xda {
			out.Write(content[pos:])
			return out.Bytes(), nil
		}
		if marker == 0xd9 || marker == 0x01 || (marker >= 0xd0 && marker <= 0xd7) {
			out.Write(content[pos : pos+2])
			pos += 2
			continue
		}
		if pos+4 > len(content) {
			return nil, fmt.Errorf("truncated segment")
		}
		length := int(content[pos+2])<<8 | int(content[pos+3])
		if length < 2 || pos+2+length > len(content) {
			return nil, fmt.Errorf("truncated segment")
		}
		payload := string(content[pos+4 : pos+2+length])
		if marker == 0xe1 && (strings.HasPrefix(payload, "Exif\x00\x00") || strings.HasPrefix(payload, "http://ns.adobe.com/xap/")) {
			pos += 2 + length
			continue
		}
		out.Write(content[pos : pos+2+length])
		pos += 2 + length
	}
	return nil, fmt.Errorf("truncated file")
}

// orientImage applies an EXIF orientation to decoded pixels, returning an
// upright image. Orientations 5-8 swap width and height.
func orientImage(img image.Image, orientation int) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	swap := orientation >= 5
	outW, outH := w, h
	if swap {
		outW, outH = h, w
	}
	out := image.NewRGBA(image.Rect(0, 0, outW, outH))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			px, py := orientPoint(x, y, w, h, orientation)
			out.Set(px, py, img.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return out
}

// orientPoint maps a source pixel to its upright position for each EXIF
// orientation: 2 mirrors left-right, 3 turns halfway round, 4 mirrors
// up-down, 5-8 transpose and turn.
func orientPoint(x, y, w, h, orientation int) (int, int) {
	switch orientation {
	case 2:
		return w - 1 - x, y
	case 3:
		return w - 1 - x, h - 1 - y
	case 4:
		return x, h - 1 - y
	case 5:
		return y, x
	case 6:
		return h - 1 - y, x
	case 7:
		return h - 1 - y, w - 1 - x
	case 8:
		return y, w - 1 - x
	default:
		return x, y
	}
}

// WebP chunk types that carry metadata rather than pixels.
var webpDropped = map[string]bool{"EXIF": true, "XMP ": true, "ICCP": true}

// WebP VP8X feature bits, per the WebP container spec: byte 0 holds
// reserved, reserved, ICC, alpha, EXIF, XMP, animation, reserved.
const (
	webpFlagICC  = 0x20
	webpFlagEXIF = 0x08
	webpFlagXMP  = 0x04
)

// cleanWebP strips metadata chunks from a WebP file without touching pixels.
// Orientation is not applied: WebP encoders bake rotation into the pixels,
// so an orientation tag is not expected, and re-encoding would change the
// file for no visible gain.
func cleanWebP(content []byte) (CleanedImage, error) {
	cleaned, dropped, err := filterWebPChunks(content)
	if err != nil {
		return CleanedImage{}, fmt.Errorf("%w: cannot read the image container: %v", ErrNotImage, err)
	}
	if !dropped {
		config, _, err := image.DecodeConfig(bytes.NewReader(content))
		if err != nil {
			return CleanedImage{}, fmt.Errorf("%w: cannot read the image: %v", ErrNotImage, err)
		}
		return CleanedImage{Content: content, Format: "webp", Width: config.Width, Height: config.Height}, nil
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(cleaned))
	if err != nil {
		return CleanedImage{}, fmt.Errorf("%w: cannot read the image: %v", ErrNotImage, err)
	}
	return CleanedImage{Content: cleaned, Format: "webp", Width: config.Width, Height: config.Height}, nil
}

// filterWebPChunks copies a WebP file without its metadata chunks and clears
// the matching VP8X feature bits. Payload chunks pass through untouched;
// the RIFF size is recomputed.
func filterWebPChunks(content []byte) (cleaned []byte, dropped bool, err error) {
	if len(content) < 12 || string(content[:4]) != "RIFF" || string(content[8:12]) != "WEBP" {
		return nil, false, fmt.Errorf("not a WebP file")
	}
	var out bytes.Buffer
	out.Write(content[:12])
	rest := content[12:]
	for len(rest) > 0 {
		if len(rest) < 8 {
			return nil, false, fmt.Errorf("truncated chunk")
		}
		fourCC := string(rest[:4])
		size := int(binary.LittleEndian.Uint32(rest[4:]))
		padded := size + size%2
		if padded > len(rest)-8 {
			return nil, false, fmt.Errorf("truncated chunk")
		}
		payload := rest[8 : 8+size]
		if webpDropped[fourCC] {
			dropped = true
		} else if fourCC == "VP8X" {
			if len(payload) < 10 {
				return nil, false, fmt.Errorf("truncated VP8X chunk")
			}
			flags := payload[0] &^ (webpFlagICC | webpFlagEXIF | webpFlagXMP)
			chunk := append([]byte{}, rest[:8]...)
			chunk = append(chunk, flags)
			chunk = append(chunk, payload[1:]...)
			if size%2 == 1 {
				chunk = append(chunk, 0)
			}
			out.Write(chunk)
		} else {
			out.Write(rest[:8+padded])
		}
		rest = rest[8+padded:]
	}
	result := out.Bytes()
	binary.LittleEndian.PutUint32(result[4:], uint32(len(result)-8))
	return result, dropped, nil
}

// Thumbnail returns a 256 px thumbnail of image bytes: decoded, fitted
// inside ThumbEdge on the long edge, and encoded back. Sources already at or
// below the edge pass through unchanged. PNG and JPEG keep their format;
// WebP has no Go encoder, so WebP thumbnails are PNG.
func Thumbnail(content []byte) ([]byte, error) {
	img, format, err := DecodeImage(content)
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	longest := max(w, h)
	if longest <= ThumbEdge {
		return content, nil
	}
	scaledW, scaledH := w*ThumbEdge/longest, h*ThumbEdge/longest
	if scaledW < 1 {
		scaledW = 1
	}
	if scaledH < 1 {
		scaledH = 1
	}
	out := image.NewRGBA(image.Rect(0, 0, scaledW, scaledH))
	xdraw.CatmullRom.Scale(out, out.Bounds(), img, bounds, xdraw.Over, nil)
	var encoded bytes.Buffer
	switch format {
	case "jpeg":
		err = jpeg.Encode(&encoded, out, &jpeg.Options{Quality: JPEGThumbQuality})
	default:
		err = png.Encode(&encoded, out)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: cannot write the thumbnail: %v", ErrNotImage, err)
	}
	return encoded.Bytes(), nil
}
