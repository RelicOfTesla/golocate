package content

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestIndex_AddLookup verifies basic token -> path indexing with
// case-insensitive lookup.
func TestIndex_AddLookup(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	writeFile(t, a, "hello world unique-a\nsecond line\n")
	writeFile(t, b, "hello golocate unique-b\n")

	ix := NewIndex(0)
	ix.AddFile(a)
	ix.AddFile(b)

	if got := ix.FileCount(); got != 2 {
		t.Fatalf("FileCount = %d, want 2", got)
	}

	// Shared token "hello" -> both files.
	got := ix.Lookup("hello")
	sort.Strings(got)
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Errorf("Lookup(hello) = %v, want [%s %s]", got, a, b)
	}

	// Case-insensitive: "UNIQUEA" matches the lowercase token "uniquea".
	writeFile(t, filepath.Join(dir, "c.txt"), "UNIQUEA\n")
	ix.AddFile(filepath.Join(dir, "c.txt"))
	got = ix.Lookup("UNIQUEA")
	if len(got) != 1 {
		t.Errorf("Lookup(UNIQUEA) = %v, want 1 hit", got)
	}

	// Missing token -> nil (caller falls back to the scan).
	if got := ix.Lookup("absent"); got != nil {
		t.Errorf("Lookup(absent) = %v, want nil", got)
	}
}

// TestIndex_Tokenize verifies token extraction rules.
func TestIndex_Tokenize(t *testing.T) {
	got := tokenize("Hello, World! foo_bar 你好 abc xyz")
	// 你好 is a 2-rune CJK word -> filtered by MinTokenLen.
	want := []string{"hello", "world", "foo_bar", "abc", "xyz"}
	if len(got) != len(want) {
		t.Fatalf("tokenize = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tokenize = %v, want %v", got, want)
		}
	}
}

// TestIndex_ShortTokensSkipped verifies MinTokenLen filtering.
func TestIndex_ShortTokensSkipped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "f.txt"), "a bb cc dd go to\n")

	ix := NewIndex(0)
	ix.AddFile(filepath.Join(dir, "f.txt"))

	for _, kw := range []string{"a", "bb", "go"} {
		if got := ix.Lookup(kw); got != nil {
			t.Errorf("Lookup(%q) should be empty (short token), got %v", kw, got)
		}
	}
}

// TestIndex_RemoveFile verifies path removal drops all its tokens.
func TestIndex_RemoveFile(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	writeFile(t, a, "shared unique-a\n")
	writeFile(t, b, "shared unique-b\n")

	ix := NewIndex(0)
	ix.AddFile(a)
	ix.AddFile(b)

	ix.RemoveFile(a)
	if got := ix.Lookup("unique-a"); got != nil {
		t.Errorf("Lookup(unique-a) after remove = %v, want nil", got)
	}
	got := ix.Lookup("shared")
	if len(got) != 1 || got[0] != b {
		t.Errorf("Lookup(shared) = %v, want [%s]", got, b)
	}
	if ix.FileCount() != 1 {
		t.Errorf("FileCount after remove = %d, want 1", ix.FileCount())
	}
}

// TestIndex_GBKDecoded verifies the content index decodes non-UTF-8 files
// (GBK) before tokenizing, reusing the content decoder.
func TestIndex_GBKDecoded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.txt")
	writeGBK(t, path, "你好世界 golocate keyword\n")

	ix := NewIndex(0)
	ix.AddFile(path)

	// The English token is indexed; the Chinese word (2 runes) is filtered.
	if got := ix.Lookup("keyword"); len(got) != 1 {
		t.Errorf("Lookup(keyword) = %v, want 1 hit", got)
	}
}

// TestIndex_BinarySkipped verifies binary files are not indexed.
func TestIndex_BinarySkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bin.dat")
	if err := os.WriteFile(path, []byte{0x00, 0x01, 0x02, 'h', 'e', 'l', 'l', 'o'}, 0o644); err != nil {
		t.Fatal(err)
	}

	ix := NewIndex(0)
	ix.AddFile(path)
	if ix.FileCount() != 0 {
		t.Errorf("FileCount = %d, want 0 (binary skipped)", ix.FileCount())
	}
}
