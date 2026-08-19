package api

import (
	"testing"

	"github.com/RelicOfTesla/golocate/pkg/index"
)

func TestSearchOptions_Passthrough(t *testing.T) {
	opts := searchOptions(SearchParams{
		IgnoreCase: true,
		Basename:   true,
		Dedupe:     true,
		Limit:      42,
		Offset:     7,
		SortField:  "size",
		SortOrder:  "desc",
	})
	if !opts.IgnoreCase || !opts.Basename || !opts.Dedupe {
		t.Errorf("bool passthrough failed: %+v", opts)
	}
	if opts.Limit != 42 || opts.Offset != 7 {
		t.Errorf("paging passthrough failed: %+v", opts)
	}
	if opts.SortField != "size" || opts.SortOrder != "desc" {
		t.Errorf("sort passthrough failed: %+v", opts)
	}
}

func TestSearchOptions_PatternModes(t *testing.T) {
	cases := []struct {
		regex       bool
		patternMode string
		want        index.PatternMode
	}{
		{false, "", index.PatternMode("")},
		{true, "", index.PatternModeExtendedRegex},
		{false, "regex", index.PatternModeExtendedRegex},
		{false, "wildcard", index.PatternModeWildcard},
		{true, "wildcard", index.PatternModeWildcard}, // explicit mode wins
	}
	for _, c := range cases {
		opts := searchOptions(SearchParams{Regex: c.regex, PatternMode: c.patternMode})
		if opts.PatternMode != c.want {
			t.Errorf("regex=%v mode=%q: PatternMode = %q, want %q",
				c.regex, c.patternMode, opts.PatternMode, c.want)
		}
	}
}
