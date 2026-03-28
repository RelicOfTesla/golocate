package pool

import (
	"testing"
)

func TestStringPoolGet(t *testing.T) {
	pool := NewStringPool()

	s1 := pool.Get("test")
	s2 := pool.Get("test")

	// Both should return the same string pointer
	if s1 != s2 {
		t.Errorf("Expected same string instance, got different")
	}

	// Check stats
	entries, hits, misses := pool.Stats()
	if entries != 1 {
		t.Errorf("Expected 1 entry, got %d", entries)
	}
	if hits != 1 {
		t.Errorf("Expected 1 hit, got %d", hits)
	}
	if misses != 1 {
		t.Errorf("Expected 1 miss, got %d", misses)
	}
}

func TestStringPoolDifferentStrings(t *testing.T) {
	pool := NewStringPool()

	pool.Get("test1")
	pool.Get("test2")
	pool.Get("test3")

	entries, _, misses := pool.Stats()
	if entries != 3 {
		t.Errorf("Expected 3 entries, got %d", entries)
	}
	if misses != 3 {
		t.Errorf("Expected 3 misses, got %d", misses)
	}
}

func TestStringPoolClear(t *testing.T) {
	pool := NewStringPool()

	pool.Get("test1")
	pool.Get("test2")

	entries, _, _ := pool.Stats()
	if entries != 2 {
		t.Errorf("Expected 2 entries before clear, got %d", entries)
	}

	pool.Clear()

	entries, hits, misses := pool.Stats()
	if entries != 0 {
		t.Errorf("Expected 0 entries after clear, got %d", entries)
	}
	if hits != 0 {
		t.Errorf("Expected 0 hits after clear, got %d", hits)
	}
	if misses != 0 {
		t.Errorf("Expected 0 misses after clear, got %d", misses)
	}
}

func TestByteBufferPool(t *testing.T) {
	pool := NewByteBufferPool()

	buf1 := pool.Get()
	if len(buf1) != 0 {
		t.Errorf("Expected empty buffer, got length %d", len(buf1))
	}
	if cap(buf1) < 1024 {
		t.Errorf("Expected capacity >= 1024, got %d", cap(buf1))
	}

	// Write some data
	buf1 = append(buf1, []byte("test")...)

	// Return to pool
	pool.Put(buf1)

	// Get again
	buf2 := pool.Get()
	if len(buf2) != 0 {
		t.Errorf("Expected empty buffer after Put, got length %d", len(buf2))
	}
}

func TestStringBuilderPool(t *testing.T) {
	pool := NewStringBuilderPool()

	sb := pool.Get()
	sb.WriteString("hello")
	sb.WriteString(" ")
	sb.WriteString("world")

	result := sb.String()
	if result != "hello world" {
		t.Errorf("Expected 'hello world', got %q", result)
	}

	// Return to pool
	pool.Put(sb)

	// Get again
	sb2 := pool.Get()
	if sb2.String() != "" {
		t.Errorf("Expected empty string builder after Put, got %q", sb2.String())
	}
}

func TestPathPool(t *testing.T) {
	pool := NewPathPool()

	// Test segment deduplication
	seg1 := pool.InternSegment("home")
	seg2 := pool.InternSegment("home")

	if seg1 != seg2 {
		t.Errorf("Expected same segment instance, got different")
	}

	// Test path deduplication
	path1 := pool.InternPath("/home/user/test.txt")
	path2 := pool.InternPath("/home/user/test.txt")

	if path1 != path2 {
		t.Errorf("Expected same path instance, got different")
	}

	// Check stats
	segs, paths, _, _, _, _ := pool.Stats()
	if segs != 1 {
		t.Errorf("Expected 1 segment entry, got %d", segs)
	}
	if paths != 1 {
		t.Errorf("Expected 1 path entry, got %d", paths)
	}
}

func TestPathPoolStats(t *testing.T) {
	pool := NewPathPool()

	pool.InternSegment("home")
	pool.InternSegment("user")
	pool.InternSegment("home") // duplicate

	pool.InternPath("/home/user/test.txt")
	pool.InternPath("/home/user/test2.txt")
	pool.InternPath("/home/user/test.txt") // duplicate

	segs, paths, segHits, segMisses, pathHits, pathMisses := pool.Stats()

	if segs != 2 {
		t.Errorf("Expected 2 segment entries, got %d", segs)
	}
	if paths != 2 {
		t.Errorf("Expected 2 path entries, got %d", paths)
	}
	if segHits != 1 {
		t.Errorf("Expected 1 segment hit, got %d", segHits)
	}
	if segMisses != 2 {
		t.Errorf("Expected 2 segment misses, got %d", segMisses)
	}
	if pathHits != 1 {
		t.Errorf("Expected 1 path hit, got %d", pathHits)
	}
	if pathMisses != 2 {
		t.Errorf("Expected 2 path misses, got %d", pathMisses)
	}
}
