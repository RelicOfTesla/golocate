package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/RelicOfTesla/golocate/pkg/security"
)

// fakeMessage implements the parts of message.Message the handlers use:
// only Payload() is needed here, but the interface also requires the other
// accessor methods, so they are stubbed.
type fakeMessage struct {
	payload []byte
}

func (f *fakeMessage) Payload() []byte          { return f.payload }
func (f *fakeMessage) ID() string               { return "" }
func (f *fakeMessage) Method() string           { return "open" }
func (f *fakeMessage) Context() context.Context { return context.Background() }
func (f *fakeMessage) Reply(any) error          { return nil }
func (f *fakeMessage) SetOnComplete(func())     {}

// TestHandleOpenHandler_RejectsOutOfScope tests that the open RPC refuses
// paths outside the configured allowed directories.
func TestHandleOpenHandler_RejectsOutOfScope(t *testing.T) {
	s := New(nil)
	s.pathValidator = security.NewPathValidator([]string{"/allowed"})

	payload, _ := json.Marshal(map[string]string{"content": "/etc/passwd"})
	_, err := s.handleOpenHandler(context.Background(), &fakeMessage{payload: payload})
	if err == nil {
		t.Fatal("expected error for out-of-scope path, got nil")
	}
	if got := err.Error(); got != "path not allowed: /etc/passwd" {
		t.Errorf("unexpected error: %v", got)
	}
}

// TestHandleOpenHandler_MissingPath tests that an empty path is rejected.
func TestHandleOpenHandler_MissingPath(t *testing.T) {
	s := New(nil)

	payload, _ := json.Marshal(map[string]string{"content": ""})
	_, err := s.handleOpenHandler(context.Background(), &fakeMessage{payload: payload})
	if err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
}

// TestHandleOpenHandler_NonexistentPath tests that a missing file is rejected
// even when it is inside the allowed directories.
func TestHandleOpenHandler_NonexistentPath(t *testing.T) {
	s := New(nil)
	s.pathValidator = security.NewPathValidator([]string{"/allowed"})

	payload, _ := json.Marshal(map[string]string{"content": "/allowed/does-not-exist"})
	_, err := s.handleOpenHandler(context.Background(), &fakeMessage{payload: payload})
	if err == nil {
		t.Fatal("expected error for nonexistent path, got nil")
	}
}
