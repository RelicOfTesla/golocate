package content

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// writeGBK writes content encoded as GBK (the common non-UTF-8 Chinese
// encoding) so search behavior on legacy files can be verified.
func writeGBK(t *testing.T, path, line string) {
	t.Helper()
	enc := simplifiedchinese.GBK.NewEncoder()
	out, err := enc.Bytes([]byte(line))
	if err != nil {
		t.Fatalf("GBK encode failed: %v", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeUTF16LE writes content as UTF-16LE with an optional BOM.
func writeUTF16LE(t *testing.T, path, text string, withBOM bool) {
	t.Helper()
	units := utf16.Encode([]rune(text))
	var b []byte
	if withBOM {
		b = append(b, 0xFF, 0xFE)
	}
	for _, u := range units {
		b = append(b, byte(u), byte(u>>8))
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func searchOne(t *testing.T, dir, pattern string, ignoreCase bool) []*SearchResult {
	t.Helper()
	s, err := NewSearcher(SearchOptions{Pattern: pattern, IgnoreCase: ignoreCase, MaxFileSize: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.SearchInDirectory(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// TestSearch_GBK verifies Chinese text in GBK-encoded files (no BOM, not
// valid UTF-8) is decoded and matched correctly.
func TestSearch_GBK(t *testing.T) {
	dir := t.TempDir()
	line := "这是第一行你好世界"
	writeGBK(t, filepath.Join(dir, "legacy.txt"), line)

	results := searchOne(t, dir, "你好", false)
	if len(results) != 1 {
		t.Fatalf("expected 1 GBK match, got %d", len(results))
	}
	if got := results[0].Line; got != line {
		t.Errorf("decoded line mismatch: got %q", got)
	}
	if results[0].Match != "你好" {
		t.Errorf("match = %q, want 你好", results[0].Match)
	}
	if results[0].LineNum != 1 {
		t.Errorf("line number = %d, want 1", results[0].LineNum)
	}
}

// TestSearch_UTF16LE_BOM verifies UTF-16LE text with a BOM is searched.
// UTF-16 text contains NUL bytes, so this also proves such files are no
// longer misclassified as binary.
func TestSearch_UTF16LE_BOM(t *testing.T) {
	dir := t.TempDir()
	text := "hello\n世界你好\n"
	writeUTF16LE(t, filepath.Join(dir, "utf16.txt"), text, true)

	results := searchOne(t, dir, "世界", false)
	if len(results) != 1 {
		t.Fatalf("expected 1 UTF-16 match, got %d", len(results))
	}
	if got := results[0].Line; got != "世界你好" {
		t.Errorf("decoded line = %q, want 世界你好", got)
	}
	if results[0].LineNum != 2 {
		t.Errorf("line number = %d, want 2", results[0].LineNum)
	}
}

// TestSearch_UTF16LE_NoBOM verifies ASCII-only UTF-16LE without a BOM is
// still searchable (its NUL bytes previously made isBinaryFile skip it).
func TestSearch_UTF16LE_NoBOM(t *testing.T) {
	dir := t.TempDir()
	text := "hello world\nplain english\n"
	writeUTF16LE(t, filepath.Join(dir, "utf16nobom.txt"), text, false)

	results := searchOne(t, dir, "hello", false)
	if len(results) != 1 {
		t.Fatalf("expected 1 match in BOM-less UTF-16 file, got %d", len(results))
	}
	if got := results[0].Line; got != "hello world" {
		t.Errorf("decoded line = %q, want 'hello world'", got)
	}
}

// TestSearch_UTF8_StillWorks is a regression guard: UTF-8 files keep working
// after the decode path was added.
func TestSearch_UTF8_StillWorks(t *testing.T) {
	dir := t.TempDir()
	line := "plain ascii line\n中文 UTF-8 行\n"
	if err := os.WriteFile(filepath.Join(dir, "utf8.txt"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	results := searchOne(t, dir, "UTF-8", true)
	if len(results) != 1 {
		t.Fatalf("expected 1 UTF-8 match, got %d", len(results))
	}
	if results[0].LineNum != 2 {
		t.Errorf("line number = %d, want 2", results[0].LineNum)
	}
}

// TestSearch_CRLF verifies CRLF line endings produce clean match lines.
func TestSearch_CRLF(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "crlf.txt"), []byte("aaa bbb\r\nccc\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	results := searchOne(t, dir, "bbb", false)
	if len(results) != 1 {
		t.Fatalf("expected 1 CRLF match, got %d", len(results))
	}
	if results[0].Line != "aaa bbb" {
		t.Errorf("line = %q, want 'aaa bbb' (CR stripped)", results[0].Line)
	}
}

// TestDecodeText_UTF16BE verifies big-endian UTF-16 with BOM decodes.
func TestDecodeText_UTF16BE(t *testing.T) {
	// "AB" in UTF-16BE with BOM: FE FF 00 41 00 42
	data := []byte{0xFE, 0xFF, 0x00, 0x41, 0x00, 0x42}
	if got := decodeText(data); got != "AB" {
		t.Errorf("decodeText(UTF-16BE) = %q, want AB", got)
	}
}

// TestDecodeText_FallbackRaw verifies decodeText never panics on odd input.
func TestDecodeText_FallbackRaw(t *testing.T) {
	// Invalid UTF-8 and invalid-for-GBK-UTF8-result bytes still return raws.
	data := []byte{0x80, 0x81, 0xFF}
	out := decodeText(data)
	if out == "" {
		t.Error("decodeText should return something")
	}
}
