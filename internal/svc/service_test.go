package svc

import (
	"path/filepath"
	"testing"

	"github.com/RelicOfTesla/golocate/pkg/config"
	"github.com/RelicOfTesla/golocate/pkg/ignore"
)

// TestDaemonOwnPatterns verifies the daemon's own outputs are excluded from
// watching, so writing the db/log/socket/pid inside a monitored directory can
// never feed a self-triggering event loop (CPU).
func TestDaemonOwnPatterns(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "idx.db")
	cfg := &config.Config{
		DatabasePath: db,
		LogFile:      filepath.Join(dir, "golocate.log"),
		SocketPath:   filepath.Join(dir, "golocate.sock"),
	}
	m := ignore.NewMatcher(buildWatchPatterns(cfg))

	for _, p := range []string{
		filepath.Join(dir, "idx.db"),
		filepath.Join(dir, "idx.db.tmp"), // persist temp sibling
		filepath.Join(dir, "golocate.log"),
		filepath.Join(dir, "golocate.sock"),
	} {
		if !m.MatchPath(p) {
			t.Errorf("expected daemon own output ignored: %s", p)
		}
	}

	// A real user file next to them must NOT be ignored.
	real := filepath.Join(dir, "report.txt")
	if m.MatchPath(real) {
		t.Errorf("real file should not be ignored: %s", real)
	}
}
