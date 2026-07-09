package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestStripJPEGDropsMetadataSegments(t *testing.T) {
	input := []byte{
		0xff, 0xd8,
		0xff, 0xe1, 0x00, 0x08, 'E', 'x', 'i', 'f', 0x00, 0x00,
		0xff, 0xfe, 0x00, 0x05, 'c', 'm', 't',
		0xff, 0xdb, 0x00, 0x04, 0x01, 0x02,
		0xff, 0xda, 0x00, 0x04, 0x03, 0x04,
		0x11, 0x22, 0xff, 0x00, 0x33,
		0xff, 0xd9,
	}

	out, err := stripJPEG(input)
	if err != nil {
		t.Fatal(err)
	}
	mustNotContain(t, out, []byte("Exif"))
	mustNotContain(t, out, []byte("cmt"))
	mustContain(t, out, []byte{0xff, 0xdb})
	mustContain(t, out, []byte{0x11, 0x22, 0xff, 0x00, 0x33})
}

func TestStripPNGKeepsCriticalAndTransparencyChunks(t *testing.T) {
	var input []byte
	input = append(input, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}...)
	input = append(input, pngChunk("IHDR", []byte{0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0})...)
	input = append(input, pngChunk("tEXt", []byte("camera=secret"))...)
	input = append(input, pngChunk("tRNS", []byte{0})...)
	input = append(input, pngChunk("IDAT", []byte{1, 2, 3})...)
	input = append(input, pngChunk("IEND", nil)...)

	out, err := stripPNG(input)
	if err != nil {
		t.Fatal(err)
	}
	mustNotContain(t, out, []byte("camera=secret"))
	mustContain(t, out, []byte("tRNS"))
	mustContain(t, out, []byte("IDAT"))
}

func TestStripWebPDropsMetadataChunksAndClearsFlags(t *testing.T) {
	vp8x := append([]byte{0x2c, 0, 0, 0}, []byte{1, 0, 0, 1, 0, 0}...)
	input := riffWebP(
		webpChunk("VP8X", vp8x),
		webpChunk("EXIF", []byte("Exif")),
		webpChunk("XMP ", []byte("xmp")),
		webpChunk("VP8 ", []byte{1, 2, 3, 4}),
	)

	out, err := stripWebP(input)
	if err != nil {
		t.Fatal(err)
	}
	mustNotContain(t, out, []byte("EXIF"))
	mustNotContain(t, out, []byte("XMP "))
	mustContain(t, out, []byte("VP8 "))
	if got := out[20]; got != 0 {
		t.Fatalf("VP8X metadata flags were not cleared: %#x", got)
	}
}

func TestStripGIFDropsCommentsAndKeepsImage(t *testing.T) {
	input := []byte{
		'G', 'I', 'F', '8', '9', 'a',
		1, 0, 1, 0, 0, 0, 0,
		0x21, 0xfe, 5, 's', 'e', 'c', 'r', 't', 0,
		0x2c, 0, 0, 0, 0, 1, 0, 1, 0, 0, 2, 1, 0, 0,
		0x3b,
	}

	out, err := stripGIF(input)
	if err != nil {
		t.Fatal(err)
	}
	mustNotContain(t, out, []byte("secrt"))
	mustContain(t, out, []byte{0x2c, 0, 0, 0, 0})
}

func TestScrubFileWritesPrefixedCopy(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "IMG_1234.jpg")
	original := []byte{
		0xff, 0xd8,
		0xff, 0xe1, 0x00, 0x08, 'E', 'x', 'i', 'f', 0x00, 0x00,
		0xff, 0xda, 0x00, 0x04, 0x03, 0x04,
		0x11,
		0xff, 0xd9,
	}
	if err := os.WriteFile(input, original, 0o666); err != nil {
		t.Fatal(err)
	}

	output, err := scrubFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(output) != "Scrub-IMG_1234.jpg" {
		t.Fatalf("unexpected output name: %s", output)
	}
	after, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("original file changed")
	}
	cleaned, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	mustNotContain(t, cleaned, []byte("Exif"))
}

func pngChunk(kind string, data []byte) []byte {
	out := make([]byte, 8+len(data)+4)
	binary.BigEndian.PutUint32(out[:4], uint32(len(data)))
	copy(out[4:8], kind)
	copy(out[8:], data)
	return out
}

func webpChunk(kind string, data []byte) []byte {
	padded := len(data) + len(data)%2
	out := make([]byte, 8+padded)
	copy(out[:4], kind)
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(data)))
	copy(out[8:], data)
	return out
}

func riffWebP(chunks ...[]byte) []byte {
	out := []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P'}
	for _, chunk := range chunks {
		out = append(out, chunk...)
	}
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	return out
}

func mustContain(t *testing.T, data, needle []byte) {
	t.Helper()
	if !contains(data, needle) {
		t.Fatalf("missing %q in %#v", needle, data)
	}
}

func mustNotContain(t *testing.T, data, needle []byte) {
	t.Helper()
	if contains(data, needle) {
		t.Fatalf("unexpected %q in %#v", needle, data)
	}
}

func contains(data, needle []byte) bool {
	for i := 0; i+len(needle) <= len(data); i++ {
		match := true
		for j := range needle {
			if data[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return len(needle) == 0
}
