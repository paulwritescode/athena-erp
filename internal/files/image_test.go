package files

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// testPNG encodes a w-by-h red-green gradient. Pixel (x, y) is unique, so
// transforms are verifiable.
func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 255 / max(w-1, 1)), uint8(y * 255 / max(h-1, 1)), 0, 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 255 / max(w-1, 1)), uint8(y * 255 / max(h-1, 1)), 0, 255})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// withPNGText splices a tEXt chunk after IHDR.
func withPNGText(t *testing.T, content []byte, keyword, text string) []byte {
	t.Helper()
	if len(content) < 33 || string(content[12:16]) != "IHDR" {
		t.Fatal("not a PNG file")
	}
	end := 8 + 4 + 4 + 13 + 4
	payload := append([]byte(keyword+"\x00"+text), []byte{}...)
	chunk := make([]byte, 8)
	binary.BigEndian.PutUint32(chunk, uint32(len(payload)))
	copy(chunk[4:], "tEXt")
	chunk = append(chunk, payload...)
	sum := make([]byte, 4)
	binary.BigEndian.PutUint32(sum, crc32.ChecksumIEEE(append([]byte("tEXt"), payload...)))
	chunk = append(chunk, sum...)
	out := append([]byte(nil), content[:end]...)
	out = append(out, chunk...)
	return append(out, content[end:]...)
}

// withJPEGEXIF splices an APP1 Exif segment carrying orientation right after SOI.
func withJPEGEXIF(t *testing.T, content []byte, orientation int) []byte {
	t.Helper()
	if len(content) < 2 || content[0] != 0xff || content[1] != 0xd8 {
		t.Fatal("not a JPEG file")
	}
	tiff := []byte("II*\x00\x08\x00\x00\x00\x01\x00")
	entry := make([]byte, 12)
	binary.LittleEndian.PutUint16(entry, 0x0112)
	binary.LittleEndian.PutUint16(entry[2:], 3)
	binary.LittleEndian.PutUint32(entry[4:], 1)
	binary.LittleEndian.PutUint16(entry[8:], uint16(orientation))
	tiff = append(tiff, entry...)
	tiff = append(tiff, 0, 0, 0, 0)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	segment := []byte{0xff, 0xe1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}
	segment = append(segment, payload...)
	out := append([]byte{0xff, 0xd8}, segment...)
	return append(out, content[2:]...)
}

func TestSniffDetection(t *testing.T) {
	pngBytes := testPNG(t, 4, 4)
	jpegBytes := testJPEG(t, 4, 4)
	for _, tc := range []struct {
		name    string
		content []byte
		want    string
	}{
		{"png", pngBytes, "png"},
		{"jpeg", jpegBytes, "jpeg"},
		{"text", []byte("hello\n"), ""},
		{"executable", []byte("\x7fELF\x02\x01\x01\x00" + strings.Repeat("\x00", 512)), ""},
		{"empty", nil, ""},
		// Sniffing reads magic bytes only; truncated files fail at decode.
		{"truncated", pngBytes[:20], "png"},
	} {
		if got := Sniff(tc.content); got != tc.want {
			t.Errorf("Sniff(%s) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestValidateImageTable(t *testing.T) {
	pngBytes := testPNG(t, 4, 4)
	jpegBytes := testJPEG(t, 4, 4)
	big := make([]byte, MaxImageBytes+1)
	for i := range big {
		big[i] = byte(i)
	}
	for _, tc := range []struct {
		name    string
		path    string
		content []byte
		want    string
	}{
		{"png ok", "images/a.png", pngBytes, "png"},
		{"jpeg ok", "images/a.jpeg", jpegBytes, "jpeg"},
		{"jpg ok", "images/a.jpg", jpegBytes, "jpeg"},
		{"text refused", "images/a.png", []byte("hello\n"), ""},
		{"renamed executable refused", "images/a.png", append([]byte("\x7fELF\x02\x01\x01\x00"), make([]byte, 600)...), ""},
		{"too big refused", "images/a.png", big, ""},
		{"name must match bytes", "images/a.png", jpegBytes, ""},
		{"truncated refused", "images/a.png", pngBytes[:20], ""},
		{"text in images refused", "images/a.txt", []byte("hello\n"), ""},
	} {
		format, err := ValidateImage(tc.path, tc.content)
		if tc.want == "" {
			if err == nil {
				t.Errorf("%s: accepted, want an error", tc.name)
			}
			continue
		}
		if err != nil || format != tc.want {
			t.Errorf("%s: format = %q, %v, want %q", tc.name, format, err, tc.want)
		}
	}
}

func TestCleanPNGDropsTextKeepsPixels(t *testing.T) {
	original := withPNGText(t, testPNG(t, 8, 6), "Comment", "location: home")
	cleaned, err := CleanImage(original)
	if err != nil {
		t.Fatal(err)
	}
	if cleaned.Format != "png" || cleaned.Width != 8 || cleaned.Height != 6 {
		t.Fatalf("cleaned = %+v", cleaned)
	}
	if bytes.Contains(cleaned.Content, []byte("location: home")) {
		t.Fatal("the text chunk survived cleaning")
	}
	first, _, err := image.Decode(bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := image.Decode(bytes.NewReader(cleaned.Content))
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 6; y++ {
		for x := 0; x < 8; x++ {
			if first.At(x, y) != second.At(x, y) {
				t.Fatalf("pixel (%d,%d) changed", x, y)
			}
		}
	}
}

func TestCleanPNGKeepsCleanBytes(t *testing.T) {
	original := testPNG(t, 8, 6)
	cleaned, err := CleanImage(original)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cleaned.Content, original) {
		t.Fatal("a clean PNG must keep its exact bytes")
	}
}

func TestCleanJPEGAppliesOrientation(t *testing.T) {
	original := withJPEGEXIF(t, testJPEG(t, 6, 4), 6)
	cleaned, err := CleanImage(original)
	if err != nil {
		t.Fatal(err)
	}
	if cleaned.Width != 4 || cleaned.Height != 6 {
		t.Fatalf("oriented dims = %dx%d, want 4x6", cleaned.Width, cleaned.Height)
	}
	if bytes.Contains(cleaned.Content, []byte("Exif\x00\x00")) {
		t.Fatal("the EXIF segment survived cleaning")
	}
	img, _, err := image.Decode(bytes.NewReader(cleaned.Content))
	if err != nil {
		t.Fatal(err)
	}
	// Orientation 6 turns 90 degrees clockwise: the source's bottom-left
	// green corner lands top-left.
	_, green, _, _ := img.At(0, 0).RGBA()
	if green < 0x8000 {
		t.Fatalf("top-left pixel = %04x, want green from the rotation", green)
	}
}

func TestCleanJPEGWithoutEXIFKeepsBytes(t *testing.T) {
	original := testJPEG(t, 6, 4)
	cleaned, err := CleanImage(original)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cleaned.Content, original) {
		t.Fatal("a clean JPEG must keep its exact bytes")
	}
}

func TestFilterWebPChunksDropsMetadata(t *testing.T) {
	riff := []byte("RIFF\x00\x00\x00\x00WEBP")
	vp8x := []byte("VP8X\x0a\x00\x00\x00\x2c\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	exif := []byte("EXIF\x04\x00\x00\x00abcd")
	vp8 := []byte("VP8 \x04\x00\x00\x00\x9d\x01\x2a\x01")
	content := append(append(append(riff, vp8x...), exif...), vp8...)
	binary.LittleEndian.PutUint32(content[4:], uint32(len(content)-8))
	cleaned, dropped, err := filterWebPChunks(content)
	if err != nil || !dropped {
		t.Fatalf("filter = %v, %v", dropped, err)
	}
	if bytes.Contains(cleaned, []byte("abcd")) {
		t.Fatal("the EXIF chunk survived filtering")
	}
	if !bytes.Contains(cleaned, []byte("\x9d\x01\x2a\x01")) {
		t.Fatal("the payload chunk did not survive filtering")
	}
	if cleaned[20] != 0x00 {
		t.Fatalf("VP8X flags = %02x, want the metadata bits cleared", cleaned[20])
	}
	if binary.LittleEndian.Uint32(cleaned[4:]) != uint32(len(cleaned)-8) {
		t.Fatal("the RIFF size was not recomputed")
	}
	if _, _, err := filterWebPChunks([]byte("RIFF\x08\x00\x00\x00WEBPnope")); err == nil {
		t.Fatal("a truncated container must fail closed")
	}
}

func TestThumbnailDimensions(t *testing.T) {
	wide := testPNG(t, 400, 200)
	thumb, err := Thumbnail(wide)
	if err != nil {
		t.Fatal(err)
	}
	img, format, err := image.Decode(bytes.NewReader(thumb))
	if err != nil {
		t.Fatal(err)
	}
	if format != "png" || img.Bounds().Dx() != 256 || img.Bounds().Dy() != 128 {
		t.Fatalf("thumbnail = %s %dx%d", format, img.Bounds().Dx(), img.Bounds().Dy())
	}
	small := testPNG(t, 100, 50)
	if thumb, err := Thumbnail(small); err != nil || !bytes.Equal(thumb, small) {
		t.Fatalf("a small image must pass through unchanged: %v", err)
	}
	photo := testJPEG(t, 300, 600)
	thumb, err = Thumbnail(photo)
	if err != nil {
		t.Fatal(err)
	}
	img, format, err = image.Decode(bytes.NewReader(thumb))
	if err != nil {
		t.Fatal(err)
	}
	if format != "jpeg" || img.Bounds().Dx() != 128 || img.Bounds().Dy() != 256 {
		t.Fatalf("thumbnail = %s %dx%d", format, img.Bounds().Dx(), img.Bounds().Dy())
	}
	if _, err := Thumbnail([]byte("not an image")); err == nil {
		t.Fatal("a thumbnail of text must fail")
	}
}

func TestOrientPointCoversAllEight(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 0, 255})
		}
	}
	at := func(img image.Image, x, y int) [2]uint8 {
		r, g, _, _ := img.At(x, y).RGBA()
		return [2]uint8{uint8(r >> 8), uint8(g >> 8)}
	}
	for orientation := 1; orientation <= 8; orientation++ {
		out := orientImage(img, orientation)
		// The source top-left (0,0) must land somewhere exact per orientation.
		var wantX, wantY, wantW, wantH int
		switch orientation {
		case 1:
			wantX, wantY, wantW, wantH = 0, 0, 2, 3
		case 2:
			wantX, wantY, wantW, wantH = 1, 0, 2, 3
		case 3:
			wantX, wantY, wantW, wantH = 1, 2, 2, 3
		case 4:
			wantX, wantY, wantW, wantH = 0, 2, 2, 3
		case 5:
			wantX, wantY, wantW, wantH = 0, 0, 3, 2
		case 6:
			wantX, wantY, wantW, wantH = 2, 0, 3, 2
		case 7:
			wantX, wantY, wantW, wantH = 2, 1, 3, 2
		case 8:
			wantX, wantY, wantW, wantH = 0, 1, 3, 2
		}
		if out.Bounds().Dx() != wantW || out.Bounds().Dy() != wantH {
			t.Fatalf("orientation %d dims = %dx%d, want %dx%d", orientation, out.Bounds().Dx(), out.Bounds().Dy(), wantW, wantH)
		}
		if got := at(out, wantX, wantY); got != [2]uint8{0, 0} {
			t.Fatalf("orientation %d moved (0,0) to %v at (%d,%d)", orientation, got, wantX, wantY)
		}
	}
}
