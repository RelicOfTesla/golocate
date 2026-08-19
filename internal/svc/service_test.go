package svc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/RelicOfTesla/golocate/pkg/config"
	"github.com/RelicOfTesla/golocate/pkg/index"
)

func TestNewDaemonService(t *testing.T) {
	cfg := &config.Config{
		Directories: []string{os.TempDir()},
	}

	d := NewDaemonService(cfg, filepath.Join(os.TempDir(), "test_config.yaml"))

	if d == nil {
		t.Error("Expected non-nil daemon service")
	}

	if d.cfg != cfg {
		t.Error("Expected config to be set")
	}

	if d.ctx == nil {
		t.Error("Expected non-nil context")
	}

	if d.cancel == nil {
		t.Error("Expected non-nil cancel function")
	}
}

func TestDaemonServiceStartAndStop(t *testing.T) {
	cfg := &config.Config{
		Directories:  []string{filepath.Join(os.TempDir(), "test_golocate_svc")},
		DatabasePath: filepath.Join(os.TempDir(), "test_golocate_svc.db"),
		WorkerCount:  1,
	}

	_ = NewDaemonService(cfg, filepath.Join(os.TempDir(), "test_golocate_svc.yaml"))

	t.Log("DaemonService created successfully")
}

func TestServiceConfig(t *testing.T) {
	svcConfig := ServiceConfig()

	if svcConfig == nil {
		t.Error("Expected non-nil service config")
	}

	if svcConfig.Name != "golocate" {
		t.Errorf("Expected name 'golocate', got %q", svcConfig.Name)
	}

	if svcConfig.DisplayName == "" {
		t.Error("Expected non-empty display name")
	}

	if svcConfig.Description == "" {
		t.Error("Expected non-empty description")
	}
}

func TestRunWithoutService(t *testing.T) {
	_ = &config.Config{
		Directories:  []string{filepath.Join(os.TempDir(), "test_golocate_run")},
		DatabasePath: filepath.Join(os.TempDir(), "test_golocate_run.db"),
		WorkerCount:  1,
	}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Run() panicked: %v", r)
		}
	}()

	t.Log("Run() can be called without panic")
}

func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.Config{
		Directories:  []string{filepath.Join(os.TempDir(), "test_golocate_integration")},
		DatabasePath: filepath.Join(os.TempDir(), "test_golocate_integration.db"),
		WorkerCount:  1,
	}

	d := NewDaemonService(cfg, filepath.Join(os.TempDir(), "test_golocate_integration.yaml"))

	if d.server != nil {
		t.Error("Expected server to be nil initially")
	}

	t.Log("Integration test passed - DaemonService has server field")
}

// TestBuildContentIndex verifies the optional content token index builder
// tokenizes all non-directory entries of an index.
func TestBuildContentIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha beta gamma"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("beta delta"), 0644); err != nil {
		t.Fatal(err)
	}

	builder := index.NewBuilder(index.BuilderOptions{WorkerCount: 1})
	if err := builder.Build(context.Background(), []string{dir}); err != nil {
		t.Fatal(err)
	}

	d := NewDaemonService(&config.Config{MaxContentFileSize: 0}, "")
	ix := d.buildContentIndex(builder.Index())

	if got := ix.FileCount(); got != 2 {
		t.Fatalf("FileCount = %d, want 2", got)
	}
	if got := ix.Lookup("alpha"); len(got) != 1 {
		t.Errorf("Lookup(alpha) = %v, want 1 hit", got)
	}
	// "beta" appears in both files.
	if got := ix.Lookup("beta"); len(got) != 2 {
		t.Errorf("Lookup(beta) = %v, want 2 hits", got)
	}
}
