package autostart

import (
	"net"
	"os"
	"testing"
)

func TestParseMode(t *testing.T) {
	cases := []struct {
		in   string
		want Mode
		err  bool
	}{
		{"child", Child, false},
		{"", Child, false},
		{"background", Background, false},
		{"none", None, false},
		{"bogus", None, true},
	}
	for _, c := range cases {
		got, err := ParseMode(c.in)
		if (err != nil) != c.err {
			t.Fatalf("ParseMode(%q) err=%v, want err=%v", c.in, err, c.err)
		}
		if !c.err && got != c.want {
			t.Fatalf("ParseMode(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

// TestEnsureNoopWhenUp: with the socket already up, Ensure returns without
// spawning (even in child mode).
func TestEnsureNoopWhenUp(t *testing.T) {
	dir := t.TempDir()
	sock := dir + "/up.sock"
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	l := &Launcher{SocketPath: sock, Mode: Child}
	if child, err := l.Ensure(); err != nil || child != nil {
		t.Fatalf("Ensure on a live socket should no-op, got child=%v err=%v", child != nil, err)
	}
}

// TestEnsureNone: in None mode we never spawn even when the socket is absent.
func TestEnsureNone(t *testing.T) {
	dir := t.TempDir()
	l := &Launcher{SocketPath: dir + "/x.sock", Mode: None}
	if child, err := l.Ensure(); err != nil || child != nil {
		t.Fatalf("Ensure(none) should no-op, got child=%v err=%v", child != nil, err)
	}
}

func TestDefaultMode(t *testing.T) {
	if DefaultMode() != Child {
		t.Fatal("default mode must be child")
	}
}

func TestFindGolocatedFromPATH(t *testing.T) {
	dir := t.TempDir()
	fake := dir + "/golocated"
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	old := os.Getenv("PATH")
	t.Setenv("PATH", dir+":"+old)
	p, err := findGolocated()
	if err != nil {
		t.Fatalf("findGolocated via PATH failed: %v", err)
	}
	if p != fake {
		t.Fatalf("findGolocated = %q, want %q", p, fake)
	}
}
