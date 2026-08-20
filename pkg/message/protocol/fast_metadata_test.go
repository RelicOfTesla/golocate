package protocol

import (
	"bufio"
	"bytes"
	"testing"
)

// TestFastRequestMetadataFieldsRoundTrip verifies that the newer search
// parameter fields (types, size/mtime filters, exclude_hidden, dedupe)
// survive a fast-protocol write/parse round trip.
func TestFastRequestMetadataFieldsRoundTrip(t *testing.T) {
	req := &Request{
		Method:        "search",
		Pattern:       "main",
		Types:         []string{"go", "md"},
		MinSize:       1024,
		MaxSize:       1 << 20,
		MtimeAfter:    1700000000,
		MtimeBefore:   1800000000,
		ExcludeHidden: true,
		Dedupe:        true,
	}

	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	proto := NewFastProtocol()
	if err := proto.WriteRequest(writer, req); err != nil {
		t.Fatalf("WriteRequest: %v", err)
	}
	writer.Flush()

	reader := bufio.NewReader(&buf)
	parsed, err := proto.ParseRequest(reader)
	if err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}

	if parsed.Method != "search" || parsed.Pattern != "main" {
		t.Errorf("method/pattern mismatch: %q %q", parsed.Method, parsed.Pattern)
	}
	if len(parsed.Types) != 2 || parsed.Types[0] != "go" || parsed.Types[1] != "md" {
		t.Errorf("types mismatch: %v", parsed.Types)
	}
	if parsed.MinSize != 1024 || parsed.MaxSize != 1<<20 {
		t.Errorf("size filters mismatch: %d %d", parsed.MinSize, parsed.MaxSize)
	}
	if parsed.MtimeAfter != 1700000000 || parsed.MtimeBefore != 1800000000 {
		t.Errorf("mtime filters mismatch: %d %d", parsed.MtimeAfter, parsed.MtimeBefore)
	}
	if !parsed.ExcludeHidden {
		t.Error("exclude_hidden should survive the round trip")
	}
	if !parsed.Dedupe {
		t.Error("dedupe should survive the round trip")
	}
}

// TestFastRequestDedupeAbsent verifies dedupe defaults to false when the
// field is not present in the request.
func TestFastRequestDedupeAbsent(t *testing.T) {
	raw := "method=search\npattern=main\n\n"
	reader := bufio.NewReader(bytes.NewBufferString(raw))
	parsed, err := NewFastProtocol().ParseRequest(reader)
	if err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}
	if parsed.Dedupe {
		t.Error("dedupe must default to false")
	}
}
