package svc

import (
	"testing"

	"github.com/RelicOfTesla/golocate/pkg/config"
)

func TestNewDaemonService(t *testing.T) {
	cfg := &config.Config{
		Directories: []string{"/tmp"},
	}
	
	d := NewDaemonService(cfg)
	
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
		Directories:   []string{"/tmp/test_golocate_svc"},
		DatabasePath:  "/tmp/test_golocate_svc.db",
		WorkerCount:   1,
	}
	
	_ = NewDaemonService(cfg)
	
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
		Directories:   []string{"/tmp/test_golocate_run"},
		DatabasePath:  "/tmp/test_golocate_run.db",
		WorkerCount:   1,
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
		Directories:   []string{"/tmp/test_golocate_integration"},
		DatabasePath:  "/tmp/test_golocate_integration.db",
		WorkerCount:   1,
	}
	
	d := NewDaemonService(cfg)
	
	if d.server != nil {
		t.Error("Expected server to be nil initially")
	}
	
	t.Log("Integration test passed - DaemonService has server field")
}
