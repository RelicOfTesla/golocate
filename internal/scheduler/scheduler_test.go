package scheduler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RelicOfTesla/golocate/pkg/config"
	"github.com/RelicOfTesla/golocate/pkg/index"
)

// TestRebuildHooks verifies that the build lifecycle hooks (start/end), the
// progress callback, and the on-index-built hook fire around a rebuild.
func TestRebuildHooks(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := &config.Config{Directories: []string{dir}}
	sch := NewScheduler(cfg, nil, &SchedulerConfig{
		Interval:         24 * 3600 * 1e9, // 24h in ns
		Throttle:         false,
		WorkerCount:      1,
		SkipInitialBuild: true,
	})

	var startCount, endCount int
	var lastProgress int64 = -1
	var builtEntries int = -1
	sch.SetOnBuildStart(func() { startCount++ })
	sch.SetOnBuildEnd(func() { endCount++ })
	sch.SetOnProgress(func(scanned int64) { lastProgress = scanned })
	sch.SetOnIndexBuilt(func(idx *index.Index) { builtEntries = idx.Len() })

	// TriggerBuild runs rebuild synchronously; verify all hooks fired.
	sch.TriggerBuild(false)

	if startCount != 1 {
		t.Errorf("onBuildStart called %d times, want 1", startCount)
	}
	if endCount != 1 {
		t.Errorf("onBuildEnd called %d times, want 1", endCount)
	}
	if builtEntries != 4 { // 3 files + 1 dir
		t.Errorf("onIndexBuilt entries = %d, want 4", builtEntries)
	}
	if lastProgress < 0 {
		t.Error("onProgress was never called; want at least a final report")
	}
	if lastProgress != 4 {
		t.Errorf("final progress = %d, want 4 entries", lastProgress)
	}
	if got := sch.GetLastBuild(); got.IsZero() {
		t.Error("GetLastBuild should be set after a rebuild")
	}
}
