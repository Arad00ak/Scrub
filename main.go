package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	menuText = "Remove Metadata with Scrub"
	verbKey  = `HKCU\Software\Classes\SystemFileAssociations\image\shell\Scrub`
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Scrub:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}

	switch strings.ToLower(args[0]) {
	case "--install", "install":
		return install()
	case "--uninstall", "uninstall":
		return uninstall()
	case "--help", "-h", "/?":
		usage()
		return nil
	}

	failures := 0
	for _, name := range args {
		out, err := scrubFile(name)
		if err != nil {
			failures++
			fmt.Fprintf(os.Stderr, "Failed: %s (%v)\n", name, err)
			continue
		}
		fmt.Printf("Scrubbed: %s -> %s\n", name, out)
	}
	if failures > 0 {
		return fmt.Errorf("%d file(s) failed", failures)
	}
	return nil
}

func usage() {
	fmt.Println("Scrub - remove image metadata without touching originals")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  scrub.exe --install      Add Explorer context menu entry")
	fmt.Println("  scrub.exe --uninstall    Remove Explorer context menu entry")
	fmt.Println("  scrub.exe <image> [...]  Write Scrub-<name> copies beside inputs")
}

func scrubFile(input string) (string, error) {
	info, err := os.Stat(input)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("not a file")
	}

	data, err := os.ReadFile(input)
	if err != nil {
		return "", err
	}

	output := outputPath(input)
	var cleaned []byte
	switch {
	case isJPEG(data):
		cleaned, err = stripJPEG(data)
	case isPNG(data):
		cleaned, err = stripPNG(data)
	case isWebP(data):
		cleaned, err = stripWebP(data)
	case isGIF(data):
		cleaned, err = stripGIF(data)
	default:
		if guid, ok := wicContainerGUID(input); ok {
			if err := transcodeWithWIC(input, output, guid); err != nil {
				return "", err
			}
			return output, nil
		}
		return "", errors.New("unsupported image format")
	}
	if err != nil {
		return "", err
	}

	if err := writeAtomic(output, cleaned); err != nil {
		return "", err
	}
	return output, nil
}

func outputPath(input string) string {
	dir := filepath.Dir(input)
	name := filepath.Base(input)
	return filepath.Join(dir, "Scrub-"+name)
}

func writeAtomic(output string, data []byte) error {
	tmp := output + ".tmp"
	if err := os.WriteFile(tmp, data, 0o666); err != nil {
		return err
	}
	return replaceFile(tmp, output)
}

func replaceFile(tmp, output string) error {
	if err := os.Rename(tmp, output); err == nil {
		return nil
	}
	_ = os.Remove(output)
	return os.Rename(tmp, output)
}

func wicContainerGUID(path string) (string, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".bmp", ".dib":
		return "0af1d87e-fcfe-4188-bdeb-a7906471cbe3", true
	case ".tif", ".tiff":
		return "163bcc30-e2e9-4f0b-961d-a3e9fdb788a3", true
	case ".heic", ".heif", ".hif":
		return "e1e62521-6787-405b-a339-500715b5763f", true
	default:
		return "", false
	}
}

func transcodeWithWIC(input, output, containerGUID string) error {
	tmp := output + ".tmp"
	_ = os.Remove(tmp)

	input64 := base64.StdEncoding.EncodeToString([]byte(input))
	tmp64 := base64.StdEncoding.EncodeToString([]byte(tmp))
	guid64 := base64.StdEncoding.EncodeToString([]byte(containerGUID))

	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$inPath = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s'))
$outPath = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s'))
$containerText = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s'))
$container = [Guid]::Parse($containerText)
Add-Type -AssemblyName PresentationCore
$resolved = (Resolve-Path -LiteralPath $inPath).ProviderPath
$uri = [Uri]::new($resolved)
$decoder = [System.Windows.Media.Imaging.BitmapDecoder]::Create(
    $uri,
    [System.Windows.Media.Imaging.BitmapCreateOptions]::PreservePixelFormat,
    [System.Windows.Media.Imaging.BitmapCacheOption]::OnLoad
)
$encoder = [System.Windows.Media.Imaging.BitmapEncoder]::Create($container)
foreach ($frame in $decoder.Frames) {
    $source = [System.Windows.Media.Imaging.BitmapSource]$frame
    $encoder.Frames.Add([System.Windows.Media.Imaging.BitmapFrame]::Create($source))
}
$stream = [System.IO.File]::Open($outPath, [System.IO.FileMode]::Create, [System.IO.FileAccess]::Write, [System.IO.FileShare]::None)
try {
    $encoder.Save($stream)
} finally {
    $stream.Dispose()
}
`, input64, tmp64, guid64)
	cmd := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy",
		"Bypass",
		"-Command",
		script,
	)
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("WIC transcode failed: %v: %s", err, strings.TrimSpace(combined.String()))
	}
	return replaceFile(tmp, output)
}

func isJPEG(data []byte) bool {
	return len(data) >= 2 && data[0] == 0xff && data[1] == 0xd8
}

func isPNG(data []byte) bool {
	return bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
}

func isWebP(data []byte) bool {
	return len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP"
}

func isGIF(data []byte) bool {
	return bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a"))
}

func isBMP(data []byte) bool {
	return len(data) >= 2 && data[0] == 'B' && data[1] == 'M'
}

func stripJPEG(data []byte) ([]byte, error) {
	if !isJPEG(data) {
		return nil, errors.New("not a JPEG")
	}

	out := make([]byte, 0, len(data))
	out = append(out, data[:2]...)
	pos := 2

	for pos < len(data) {
		markerStart := findJPEGMarker(data, pos)
		if markerStart < 0 || markerStart+1 >= len(data) {
			return nil, errors.New("invalid JPEG marker")
		}
		marker := data[markerStart+1]
		pos = markerStart + 2

		if marker == 0xd9 {
			out = append(out, data[markerStart:markerStart+2]...)
			break
		}
		if marker == 0x01 || (marker >= 0xd0 && marker <= 0xd7) {
			out = append(out, data[markerStart:markerStart+2]...)
			continue
		}
		if pos+2 > len(data) {
			return nil, errors.New("truncated JPEG segment")
		}

		segLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
		if segLen < 2 || pos+segLen > len(data) {
			return nil, errors.New("truncated JPEG segment")
		}
		segEnd := pos + segLen
		remove := (marker >= 0xe0 && marker <= 0xef) || marker == 0xfe

		if marker == 0xda {
			out = append(out, data[markerStart:segEnd]...)
			pos = copyJPEGScan(data, segEnd, &out)
			continue
		}
		if !remove {
			out = append(out, data[markerStart:segEnd]...)
		}
		pos = segEnd
	}

	return out, nil
}

func findJPEGMarker(data []byte, pos int) int {
	for pos+1 < len(data) {
		if data[pos] == 0xff && data[pos+1] != 0x00 && data[pos+1] != 0xff {
			return pos
		}
		pos++
	}
	return -1
}

func copyJPEGScan(data []byte, pos int, out *[]byte) int {
	for pos < len(data) {
		if data[pos] != 0xff {
			*out = append(*out, data[pos])
			pos++
			continue
		}
		if pos+1 >= len(data) {
			*out = append(*out, data[pos])
			return len(data)
		}

		next := data[pos+1]
		if next == 0x00 || next == 0xff || (next >= 0xd0 && next <= 0xd7) {
			*out = append(*out, data[pos], data[pos+1])
			pos += 2
			continue
		}
		return pos
	}
	return pos
}

func stripPNG(data []byte) ([]byte, error) {
	if !isPNG(data) {
		return nil, errors.New("not a PNG")
	}

	out := make([]byte, 0, len(data))
	out = append(out, data[:8]...)
	pos := 8

	for pos+12 <= len(data) {
		chunkLen := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		chunkEnd := pos + 12 + chunkLen
		if chunkEnd < pos || chunkEnd > len(data) {
			return nil, errors.New("truncated PNG chunk")
		}

		kind := string(data[pos+4 : pos+8])
		if keepPNGChunk(kind) {
			out = append(out, data[pos:chunkEnd]...)
		}
		pos = chunkEnd
		if kind == "IEND" {
			break
		}
	}

	return out, nil
}

func keepPNGChunk(kind string) bool {
	if len(kind) != 4 {
		return false
	}
	critical := kind[0] >= 'A' && kind[0] <= 'Z'
	if critical {
		return true
	}
	switch kind {
	case "tRNS", "acTL", "fcTL", "fdAT":
		return true
	default:
		return false
	}
}

func stripWebP(data []byte) ([]byte, error) {
	if !isWebP(data) {
		return nil, errors.New("not a WebP")
	}

	out := make([]byte, 0, len(data))
	out = append(out, data[:12]...)
	pos := 12

	for pos+8 <= len(data) {
		chunkType := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		padded := size + size%2
		chunkEnd := pos + 8 + padded
		if chunkEnd < pos || chunkEnd > len(data) {
			return nil, errors.New("truncated WebP chunk")
		}

		if chunkType != "EXIF" && chunkType != "XMP " && chunkType != "ICCP" {
			start := len(out)
			out = append(out, data[pos:chunkEnd]...)
			if chunkType == "VP8X" && size >= 10 {
				out[start+8] &^= 0x20 | 0x08 | 0x04
			}
		}
		pos = chunkEnd
	}

	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	return out, nil
}

func stripGIF(data []byte) ([]byte, error) {
	if !isGIF(data) || len(data) < 13 {
		return nil, errors.New("not a GIF")
	}

	gctLen := 0
	if data[10]&0x80 != 0 {
		gctLen = 3 * (1 << ((data[10] & 0x07) + 1))
	}
	headerEnd := 13 + gctLen
	if headerEnd > len(data) {
		return nil, errors.New("truncated GIF header")
	}

	out := make([]byte, 0, len(data))
	out = append(out, data[:headerEnd]...)
	pos := headerEnd

	for pos < len(data) {
		switch data[pos] {
		case 0x3b:
			out = append(out, 0x3b)
			return out, nil
		case 0x2c:
			end, err := gifImageEnd(data, pos)
			if err != nil {
				return nil, err
			}
			out = append(out, data[pos:end]...)
			pos = end
		case 0x21:
			end, err := gifExtensionEnd(data, pos)
			if err != nil {
				return nil, err
			}
			label := data[pos+1]
			isComment := label == 0xfe
			isXMP := label == 0xff && pos+14 <= len(data) && bytes.HasPrefix(data[pos+3:pos+14], []byte("XMP DataXMP"))
			if !isComment && !isXMP {
				out = append(out, data[pos:end]...)
			}
			pos = end
		default:
			return nil, errors.New("invalid GIF block")
		}
	}

	return out, nil
}

func gifImageEnd(data []byte, pos int) (int, error) {
	if pos+10 > len(data) {
		return 0, errors.New("truncated GIF image descriptor")
	}
	lctLen := 0
	if data[pos+9]&0x80 != 0 {
		lctLen = 3 * (1 << ((data[pos+9] & 0x07) + 1))
	}
	lzwPos := pos + 10 + lctLen
	if lzwPos >= len(data) {
		return 0, errors.New("truncated GIF image data")
	}
	return gifSubBlocksEnd(data, lzwPos+1)
}

func gifExtensionEnd(data []byte, pos int) (int, error) {
	if pos+2 > len(data) {
		return 0, errors.New("truncated GIF extension")
	}
	if data[pos+1] == 0xf9 {
		if pos+8 > len(data) {
			return 0, errors.New("truncated GIF graphics control extension")
		}
		return pos + 8, nil
	}
	return gifSubBlocksEnd(data, pos+2)
}

func gifSubBlocksEnd(data []byte, pos int) (int, error) {
	for {
		if pos >= len(data) {
			return 0, errors.New("truncated GIF sub-block")
		}
		n := int(data[pos])
		pos++
		if n == 0 {
			return pos, nil
		}
		pos += n
		if pos > len(data) {
			return 0, errors.New("truncated GIF sub-block")
		}
	}
}

func install() error {
	exe, err := installExecutable()
	if err != nil {
		return err
	}
	command := fmt.Sprintf(`"%s" "%%1"`, exe)

	if err := regAdd(verbKey, "", menuText); err != nil {
		return err
	}
	if err := regAdd(verbKey, "Icon", exe); err != nil {
		return err
	}
	if err := regAdd(verbKey, "MultiSelectModel", "Player"); err != nil {
		return err
	}
	if err := regAdd(verbKey+`\command`, "", command); err != nil {
		return err
	}

	fmt.Println("Installed Explorer menu:", menuText)
	fmt.Println("Executable:", exe)
	return nil
}

func uninstall() error {
	cmd := exec.Command("reg.exe", "delete", verbKey, "/f")
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	if err := cmd.Run(); err != nil {
		if strings.Contains(combined.String(), "unable to find") || strings.Contains(combined.String(), "cannot find") {
			fmt.Println("Explorer menu was not installed.")
			return nil
		}
		return fmt.Errorf("registry delete failed: %v: %s", err, strings.TrimSpace(combined.String()))
	}
	fmt.Println("Removed Explorer menu:", menuText)
	return nil
}

func installExecutable() (string, error) {
	current, err := os.Executable()
	if err != nil {
		return "", err
	}
	current, _ = filepath.Abs(current)

	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return "", errors.New("LOCALAPPDATA is not set")
	}
	targetDir := filepath.Join(base, "Scrub")
	if err := os.MkdirAll(targetDir, 0o777); err != nil {
		return "", err
	}
	target := filepath.Join(targetDir, "scrub.exe")

	if !samePath(current, target) {
		if err := copyFile(current, target); err != nil {
			return "", err
		}
	}
	return target, nil
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return strings.EqualFold(a, b)
	}
	return strings.EqualFold(aa, bb)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return replaceFile(tmp, dst)
}

func regAdd(key, name, value string) error {
	args := []string{"add", key, "/f", "/t", "REG_SZ", "/d", value}
	if name == "" {
		args = append(args, "/ve")
	} else {
		args = append(args, "/v", name)
	}

	cmd := exec.Command("reg.exe", args...)
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("registry update failed: %v: %s", err, strings.TrimSpace(combined.String()))
	}
	return nil
}
