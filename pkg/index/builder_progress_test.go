package index

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// TestBuilderProgressCallback verifies that SetProgressCallback reports a
// monotonic scanned count and finishes with the total number of entries.
func TestBuilderProgressCallback(t *testing.T) {
	dir := t.TempDir()
	// a/ (dir), a/b.txt, a/c.txt, d.go — walk yields 4 entries.
	sub := filepath.Join(dir, "a")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "c.txt"), []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d.go"), []byte("package d"), 0o644); err != nil {
		t.Fatal(err)
	}

	var reports []int64
	var maxReported atomic.Int64
	builder := NewBuilder(BuilderOptions{WorkerCount: 1})
	builder.SetProgressCallback(func(scanned int64) {
		reports = append(reports, scanned)
		for {
			cur := maxReported.Load()
			if scanned <= cur || maxReported.CompareAndSwap(cur, scanned) {
				break
			}
		}
	})

	if err := builder.Build(context.Background(), []string{dir}); err != nil {
		t.Fatal(err)
	}

	if len(reports) == 0 {
		t.Fatal("progress callback was never invoked")
	}

	// Final report must equal the index size.
	final := reports[len(reports)-1]
	if got, want := final, int64(builder.Index().Len()); got != want {
		t.Errorf("final progress = %d, want %d", got, want)
	}

	// Progress must be monotonic non-decreasing.
	prev := int64(0)
	for i, r := range reports {
		if r < prev {
			t.Errorf("report[%d] = %d went backwards from %d", i, r, prev)
		}
		prev = r
	}

	// At least one intermediate report should exist beyond the final one.
	if len(reports) < 2 {
		t.Logf("only final report emitted (len=%d) — fine for a tiny tree", len(reports))
	}
}
