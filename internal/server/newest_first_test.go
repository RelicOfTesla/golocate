package server

import (
	"testing"
	"time"

	"github.com/RelicOfTesla/golocate/pkg/index"
)

// TestNewestFirst verifies newestFirst orders entries by ModTime descending,
// with zero-time entries last.
func TestNewestFirst(t *testing.T) {
	base := time.Unix(1700000000, 0)
	entries := []*index.Entry{
		{Path: "/old", ModTime: base.Add(-time.Hour)},
		{Path: "/new", ModTime: base.Add(time.Hour)},
		{Path: "/zero", ModTime: time.Time{}},
		{Path: "/mid", ModTime: base},
	}

	newestFirst(entries)

	got := []string{entries[0].Path, entries[1].Path, entries[2].Path, entries[3].Path}
	want := []string{"/new", "/mid", "/old", "/zero"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}
