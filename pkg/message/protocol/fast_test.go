package protocol

import (
	"bufio"
	"bytes"
	"testing"
)

// TestFastResponseCountTotalRoundTrip verifies that count/total are written
// and parsed even when the response also carries a Result map.
// Regression test for pagination: search responses must deliver count/total
// to the client so H5/GTK can compute page counts.
func TestFastResponseCountTotalRoundTrip(t *testing.T) {
	resp := &Response{
		ID:    1,
		Count: 42,
		Total: 1147,
		Paths: []string{"/a", "/b"},
		Result: map[string]any{
			"status": "ok",
		},
	}

	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	proto := NewFastProtocol()
	if err := proto.WriteResponse(writer, resp); err != nil {
		t.Fatalf("WriteResponse: %v", err)
	}
	writer.Flush()

	reader := bufio.NewReader(&buf)
	parsed, err := proto.ParseResponse(reader)
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}

	if parsed.Count != 42 {
		t.Errorf("Count: expected 42, got %d", parsed.Count)
	}
	if parsed.Total != 1147 {
		t.Errorf("Total: expected 1147, got %d", parsed.Total)
	}
	if len(parsed.Paths) != 2 {
		t.Errorf("Paths: expected 2, got %d", len(parsed.Paths))
	}
	if parsed.Result == nil {
		t.Error("Result: expected non-nil map to survive round trip")
	}
}
