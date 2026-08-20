package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestDedupeHardlinks verifies that Dedupe collapses two paths that hard link
// to the same underlying file, while ordinary files are unaffected.
func TestDedupeHardlinks(t *testing.T) {
	dir := t.TempDir()

	// One real file with a hard link alias + one ordinary file.
	if err := os.WriteFile(filepath.Join(dir, "real.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(dir, "real.txt"), filepath.Join(dir, "alias.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	builder := NewBuilder(BuilderOptions{WorkerCount: 1})
	if err := builder.Build(context.Background(), []string{dir}); err != nil {
		t.Fatal(err)
	}
	idx := builder.Index()

	// Without dedupe: alias.txt and real.txt are two separate results.
	all := idx.Search(SearchOptions{Pattern: "txt", Basename: true})
	if len(all) != 3 {
		t.Fatalf("expected 3 entries before dedupe, got %d", len(all))
	}

	// With dedupe: the hard-linked pair collapses to one, other.txt stays.
	deduped := idx.Search(SearchOptions{Pattern: "txt", Basename: true, Dedupe: true})
	if len(deduped) != 2 {
		t.Fatalf("expected 2 entries after dedupe, got %d", len(deduped))
	}
	paths := map[string]bool{}
	for _, e := range deduped {
		paths[filepath.Base(e.Path)] = true
	}
	if !paths["other.txt"] {
		t.Error("other.txt must survive dedupe")
	}
	if paths["alias.txt"] && paths["real.txt"] {
		t.Error("hard-linked alias.txt and real.txt must collapse to one")
	}
}

// TestDedupeIdentifiesHardlinks verifies the identity uses device/inode when
// available (Unix), so distinct files with identical size+mtime are NOT merged.
func TestDedupeIdentifiesHardlinks(t *testing.T) {
	dir := t.TempDir()
	content := []byte("same content and same size")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	builder := NewBuilder(BuilderOptions{WorkerCount: 1})
	if err := builder.Build(context.Background(), []string{dir}); err != nil {
		t.Fatal(err)
	}
	idx := builder.Index()

	// Same size/time but different files: dedupe must keep both on Unix
	// (where device/inode identity is available).
	deduped := idx.Search(SearchOptions{Pattern: "txt", Basename: true, Dedupe: true})
	if len(deduped) != 2 {
		t.Fatalf("expected 2 distinct entries (same size+mtime), got %d", len(deduped))
	}

	// Sanity: the builder recorded real identities.
	var seenIno uint64
	for _, e := range idx.GetAllEntries() {
		if e.Ino > 0 {
			seenIno = e.Ino
		}
	}
	if seenIno == 0 {
		t.Log("note: platform provides no inode info; size+mtime fallback in effect")
	}
}
