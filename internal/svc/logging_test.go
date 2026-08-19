package svc

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRotatingFile_WritesAndRotates verifies that the rotating log writer
// writes appends and rotates once the size limit is crossed.
func TestRotatingFile_WritesAndRotates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.log")
	backup := path + ".1"

	r, err := openLogFile(path, 20) // tiny limit: 20 bytes
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	// Each write appends 10 bytes: "0123456789\n".
	write := func() {
		if _, err := r.Write([]byte("0123456789\n")); err != nil {
			t.Fatal(err)
		}
	}

	write() // size 11
	write() // 11+11 > 20 -> rotates before writing; size 11 in fresh file
	write() // 11+11 > 20 again -> rotates again; fresh file keeps newest write

	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("backup log %s should exist after rotation: %v", backup, err)
	}
	cur, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(cur) != "0123456789\n" {
		t.Errorf("fresh log should contain only the newest write, got %q", cur)
	}
	old, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	// Rotation happens before the write that would exceed the limit, so the
	// backup holds the previous single write (11 bytes).
	if len(old) != 11 {
		t.Errorf("backup should hold the pre-rotation write (11 bytes), got %d", len(old))
	}
}

// TestRotatingFile_Appends verifies normal append behavior under the limit.
func TestRotatingFile_Appends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.log")

	r, err := openLogFile(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if _, err := r.Write([]byte("first\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Write([]byte("second\n")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "first\nsecond\n" {
		t.Errorf("expected two appended lines, got %q", data)
	}
}
