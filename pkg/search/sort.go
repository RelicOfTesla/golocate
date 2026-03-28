// Package search provides search result sorting functionality.
package search

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/RelicOfTesla/golocate/pkg/index"
)

// SortField defines the field to sort by.
type SortField int

const (
	SortNone SortField = iota
	SortByName
	SortByNameCaseSensitive
	SortBySize
	SortByModTime
	SortByPath
)

func (s SortField) String() string {
	switch s {
	case SortByName:
		return "name"
	case SortByNameCaseSensitive:
		return "name-case"
	case SortBySize:
		return "size"
	case SortByModTime:
		return "time"
	case SortByPath:
		return "path"
	default:
		return "none"
	}
}

// ParseSortField parses a string into SortField.
func ParseSortField(s string) SortField {
	switch strings.ToLower(s) {
	case "name", "n":
		return SortByName
	case "name-case", "nc":
		return SortByNameCaseSensitive
	case "size", "s":
		return SortBySize
	case "time", "t", "mtime", "date":
		return SortByModTime
	case "path", "p":
		return SortByPath
	default:
		return SortNone
	}
}

// SortOrder defines the sort order.
type SortOrder int

const (
	OrderAsc SortOrder = iota
	OrderDesc
)

func (o SortOrder) String() string {
	if o == OrderDesc {
		return "desc"
	}
	return "asc"
}

// SortOptions contains sorting options.
type SortOptions struct {
	Field SortField
	Order SortOrder
}

// Sort sorts the entries according to the given options.
func Sort(entries []*index.Entry, opts SortOptions) {
	if opts.Field == SortNone || len(entries) == 0 {
		return
	}

	switch opts.Field {
	case SortByName:
		sort.Slice(entries, func(i, j int) bool {
			cmp := strings.Compare(
				strings.ToLower(entries[i].Name),
				strings.ToLower(entries[j].Name),
			)
			if opts.Order == OrderDesc {
				return cmp > 0
			}
			return cmp < 0
		})

	case SortByNameCaseSensitive:
		sort.Slice(entries, func(i, j int) bool {
			cmp := strings.Compare(entries[i].Name, entries[j].Name)
			if opts.Order == OrderDesc {
				return cmp > 0
			}
			return cmp < 0
		})

	case SortBySize:
		sort.Slice(entries, func(i, j int) bool {
			if opts.Order == OrderDesc {
				return entries[i].Size > entries[j].Size
			}
			return entries[i].Size < entries[j].Size
		})

	case SortByModTime:
		sort.Slice(entries, func(i, j int) bool {
			if opts.Order == OrderDesc {
				return entries[i].ModTime.After(entries[j].ModTime)
			}
			return entries[i].ModTime.Before(entries[j].ModTime)
		})

	case SortByPath:
		sort.Slice(entries, func(i, j int) bool {
			cmp := strings.Compare(
				strings.ToLower(entries[i].Path),
				strings.ToLower(entries[j].Path),
			)
			if opts.Order == OrderDesc {
				return cmp > 0
			}
			return cmp < 0
		})
	}
}

// SortByMultiple sorts by multiple fields.
// Fields are applied in order; if the first field is equal, the second is used, etc.
func SortByMultiple(entries []*index.Entry, fields []SortOptions) {
	if len(fields) == 0 || len(entries) == 0 {
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		for _, opts := range fields {
			cmp := compareByField(entries[i], entries[j], opts.Field)
			if cmp != 0 {
				if opts.Order == OrderDesc {
					return cmp > 0
				}
				return cmp < 0
			}
		}
		return false
	})
}

// compareByField compares two entries by a specific field.
// Returns -1, 0, or 1.
func compareByField(a, b *index.Entry, field SortField) int {
	switch field {
	case SortByName:
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	case SortByNameCaseSensitive:
		return strings.Compare(a.Name, b.Name)
	case SortBySize:
		if a.Size < b.Size {
			return -1
		} else if a.Size > b.Size {
			return 1
		}
		return 0
	case SortByModTime:
		if a.ModTime.Before(b.ModTime) {
			return -1
		} else if a.ModTime.After(b.ModTime) {
			return 1
		}
		return 0
	case SortByPath:
		return strings.Compare(strings.ToLower(a.Path), strings.ToLower(b.Path))
	default:
		return 0
	}
}

// DefaultSortOptions returns the default sort options.
func DefaultSortOptions() SortOptions {
	return SortOptions{
		Field: SortByName,
		Order: OrderAsc,
	}
}

// ParseSort parses a sort string like "name:asc" or "size:desc".
func ParseSort(s string) SortOptions {
	if s == "" {
		return DefaultSortOptions()
	}

	parts := strings.Split(s, ":")
	field := ParseSortField(parts[0])

	order := OrderAsc
	if len(parts) > 1 {
		if strings.ToLower(parts[1]) == "desc" {
			order = OrderDesc
		}
	}

	return SortOptions{Field: field, Order: order}
}

// FormatModTime formats a modification time for display.
func FormatModTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// FormatSize formats a file size for display.
func FormatSize(size int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case size >= GB:
		return fmt.Sprintf("%.1f GB", float64(size)/GB)
	case size >= MB:
		return fmt.Sprintf("%.1f MB", float64(size)/MB)
	case size >= KB:
		return fmt.Sprintf("%.1f KB", float64(size)/KB)
	default:
		return fmt.Sprintf("%d B", size)
	}
}
