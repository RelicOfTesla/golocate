package index

import (
	"testing"
	"time"
)

func mkEntry(path string, t time.Time) *Entry {
	return &Entry{Name: path, Path: path, ModTime: t}
}

// TestRecentEntries_NewestFirst verifies the bounded candidate set returns
// entries sorted by ModTime desc, capped at the requested size, and only when
// the index has enough distinct real-mtime entries.
func TestRecentEntries_NewestFirst(t *testing.T) {
	idx := NewIndex()
	base := time.Unix(1700000000, 0)
	for i := 0; i < 5; i++ {
		idx.Add(mkEntry("old-"+string(rune('a'+i)), base))
	}
	for i := 0; i < 5; i++ {
		idx.Add(mkEntry("new-"+string(rune('a'+i)), base.Add(time.Duration(i+1)*time.Minute)))
	}

	got := idx.RecentEntries(3)
	if len(got) != 3 {
		t.Fatalf("RecentEntries(3) = %d, want 3", len(got))
	}
	// newest first
	if !got[0].ModTime.After(got[1].ModTime) || !got[1].ModTime.After(got[2].ModTime) {
		t.Errorf("not newest-first: %v %v %v", got[0].ModTime, got[1].ModTime, got[2].ModTime)
	}
	if got[0].Path != "new-e" {
		t.Errorf("first = %q, want new-e (newest)", got[0].Path)
	}
}

// TestRecentEntries_DupAndRemoved verifies duplicate paths keep their newest
// version and removed entries are filtered out.
func TestRecentEntries_DupAndRemoved(t *testing.T) {
	idx := NewIndex()
	base := time.Unix(1700000000, 0)
	idx.Add(mkEntry("/a", base))
	idx.Add(mkEntry("/b", base.Add(time.Minute)))
	// Update /a with a newer mtime (Write event upsert).
	idx.Add(mkEntry("/a", base.Add(2*time.Minute)))
	idx.Remove("/b")

	got := idx.RecentEntries(10)
	if len(got) != 1 || got[0].Path != "/a" {
		t.Fatalf("RecentEntries = %v, want only /a", got)
	}
	if !got[0].ModTime.Equal(base.Add(2 * time.Minute)) {
		t.Errorf("/a should keep the newest version")
	}
}

// TestRecentEntries_Cap verifies the heap is bounded (memory stays small).
func TestRecentEntries_Cap(t *testing.T) {
	idx := NewIndex()
	base := time.Unix(1700000000, 0)
	junk := 0
	for i := 0; i < maxRecentEntries+50; i++ {
		idx.Add(mkEntry("/f"+itoa(i), base.Add(time.Duration(i)*time.Second)))
	}
	if idx.recent == nil || idx.recent.Len() > maxRecentEntries {
		t.Fatalf("recent heap exceeded cap: %d", idx.recent.Len())
	}
	got := idx.RecentEntries(100)
	if len(got) != 100 {
		t.Fatalf("RecentEntries(100) = %d, want 100", len(got))
	}
	// The heap keeps the newest maxRecentEntries (drops the oldest 50 here),
	// so the newest overall file (highest index) must be present.
	if got[0].Path != "/f"+itoa(maxRecentEntries+49) {
		t.Errorf("newest = %q, want /f%d", got[0].Path, maxRecentEntries+49)
	}
	junk++ // keep param used
	_ = junk
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
