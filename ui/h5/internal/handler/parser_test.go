package handler

import "testing"

func TestParseSearchQuery_Basic(t *testing.T) {
	p := ParseSearchQuery("test123")
	if p.Pattern != "test123" {
		t.Errorf("Pattern = %q, want test123", p.Pattern)
	}
	if p.Content != "" || p.IgnoreCase || p.Regex || p.Basename {
		t.Errorf("unexpected flags: %+v", p)
	}
	if p.Limit != 100 {
		t.Errorf("default Limit = %d, want 100", p.Limit)
	}
}

func TestParseSearchQuery_ContentFlag(t *testing.T) {
	p := ParseSearchQuery("main.go --content:hello")
	if p.Pattern != "main.go" {
		t.Errorf("Pattern = %q, want main.go", p.Pattern)
	}
	if p.Content != "hello" {
		t.Errorf("Content = %q, want hello", p.Content)
	}

	// Extra words after the flag stay in the pattern (multi-word pattern).
	p2 := ParseSearchQuery("main.go --content:hello world")
	if p2.Content != "hello" {
		t.Errorf("Content = %q, want hello", p2.Content)
	}
	if p2.Pattern != "main.go  world" && p2.Pattern != "main.go world" {
		t.Errorf("Pattern = %q, want trailing words preserved", p2.Pattern)
	}
}

func TestParseSearchQuery_QuotedContent(t *testing.T) {
	p := ParseSearchQuery(`--content:"two words" path`)
	if p.Content != "two words" {
		t.Errorf("Content = %q, want 'two words'", p.Content)
	}
	if p.Pattern != "path" {
		t.Errorf("Pattern = %q, want path", p.Pattern)
	}
}

func TestParseSearchQuery_Flags(t *testing.T) {
	p := ParseSearchQuery("foo --ignore-case --regex --basename --limit:50")
	if !p.IgnoreCase {
		t.Error("ignore-case should be set")
	}
	if !p.Regex {
		t.Error("regex should be set")
	}
	if !p.Basename {
		t.Error("basename should be set")
	}
	if p.Limit != 50 {
		t.Errorf("Limit = %d, want 50", p.Limit)
	}
	if p.Pattern != "foo" {
		t.Errorf("Pattern = %q, want foo", p.Pattern)
	}
}

func TestParseSearchQuery_NoFlags(t *testing.T) {
	p := ParseSearchQuery("   ")
	if p.Pattern != "" {
		t.Errorf("Pattern = %q, want empty", p.Pattern)
	}
}
