package protocol

import (
	"bufio"
	"bytes"
	"testing"
)

// TestJSONDecoder_TopLevelFields verifies that the plain JSON protocol form
// (search fields at the top level, no "params" wrapper) is parsed correctly.
func TestJSONDecoder_TopLevelFields(t *testing.T) {
	raw := []byte(`{"method":"search","content":"hello","pattern":"main.go","ignore_case":true,"limit":10,"dedupe":true,"exclude_hidden":true}`)
	req, err := DecodeRequest(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if req.Method != "search" {
		t.Errorf("Method = %q, want search", req.Method)
	}
	if req.Content != "hello" {
		t.Errorf("Content = %q, want hello (top-level JSON field)", req.Content)
	}
	if req.Pattern != "main.go" {
		t.Errorf("Pattern = %q, want main.go", req.Pattern)
	}
	if !req.IgnoreCase || req.Limit != 10 {
		t.Errorf("flags not parsed: ignore_case=%v limit=%d", req.IgnoreCase, req.Limit)
	}
	if !req.Dedupe || !req.ExcludeHidden {
		t.Error("dedupe/exclude_hidden should be parsed from top level")
	}
}

// TestJSONDecoder_ParamsWins verifies that an explicit JSON-RPC "params"
// object overrides top-level fields when both are present.
func TestJSONDecoder_ParamsWins(t *testing.T) {
	raw := []byte(`{"jsonrpc":"2.0","method":"search","content":"top","params":{"content":"params"}}`)
	req, err := DecodeRequest(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if req.Content != "params" {
		t.Errorf("Content = %q, want params (params object wins)", req.Content)
	}
}

// TestJSONDecoder_JSONRPCStyle verifies the documented JSON-RPC form.
func TestJSONDecoder_JSONRPCStyle(t *testing.T) {
	raw := []byte(`{"jsonrpc":"2.0","method":"search","params":{"pattern":"x","basename":true},"id":1}`)
	req, err := DecodeRequest(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if req.Pattern != "x" || !req.Basename {
		t.Errorf("params fields lost: pattern=%q basename=%v", req.Pattern, req.Basename)
	}
}
