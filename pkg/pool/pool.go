// Package pool provides memory pool utilities for efficient memory management.
package pool

import (
	"sync"
)

// StringPool is a pool for string deduplication to reduce memory usage.
type StringPool struct {
	mu    sync.RWMutex
	strings map[string]string
	hits   int64
	misses int64
}

// NewStringPool creates a new string pool.
func NewStringPool() *StringPool {
	return &StringPool{
		strings: make(map[string]string),
	}
}

// Get returns a canonical string from the pool.
// If the string is already in the pool, it returns the existing instance.
// Otherwise, it adds the string to the pool and returns it.
func (p *StringPool) Get(s string) string {
	p.mu.RLock()
	if cached, ok := p.strings[s]; ok {
		p.mu.RUnlock()
		p.hits++
		return cached
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double check after acquiring write lock
	if cached, ok := p.strings[s]; ok {
		p.hits++
		return cached
	}

	p.strings[s] = s
	p.misses++
	return s
}

// Stats returns statistics about the pool.
func (p *StringPool) Stats() (entries, hits, misses int64) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return int64(len(p.strings)), p.hits, p.misses
}

// Clear clears the pool.
func (p *StringPool) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.strings = make(map[string]string)
	p.hits = 0
	p.misses = 0
}

// ByteBufferPool is a pool of byte buffers for reuse.
type ByteBufferPool struct {
	pool sync.Pool
}

// NewByteBufferPool creates a new byte buffer pool.
func NewByteBufferPool() *ByteBufferPool {
	return &ByteBufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, 1024)
			},
		},
	}
}

// Get returns a byte buffer from the pool.
func (p *ByteBufferPool) Get() []byte {
	return p.pool.Get().([]byte)
}

// Put returns a byte buffer to the pool.
// The buffer is reset before being returned.
func (p *ByteBufferPool) Put(b []byte) {
	b = b[:0]
	p.pool.Put(b)
}

// StringBuilderPool is a pool of string builders for reuse.
type StringBuilderPool struct {
	pool sync.Pool
}

// NewStringBuilderPool creates a new string builder pool.
func NewStringBuilderPool() *StringBuilderPool {
	return &StringBuilderPool{
		pool: sync.Pool{
			New: func() interface{} {
				return new(stringBuilder)
			},
		},
	}
}

type stringBuilder struct {
	buf []byte
}

func (sb *stringBuilder) WriteString(s string) {
	sb.buf = append(sb.buf, s...)
}

func (sb *stringBuilder) String() string {
	return string(sb.buf)
}

func (sb *stringBuilder) Reset() {
	sb.buf = sb.buf[:0]
}

// Get returns a string builder from the pool.
func (p *StringBuilderPool) Get() *stringBuilder {
	return p.pool.Get().(*stringBuilder)
}

// Put returns a string builder to the pool.
func (p *StringBuilderPool) Put(sb *stringBuilder) {
	sb.Reset()
	p.pool.Put(sb)
}

// PathPool is a specialized pool for file paths.
type PathPool struct {
	segments *StringPool
	paths    *StringPool
}

// NewPathPool creates a new path pool.
func NewPathPool() *PathPool {
	return &PathPool{
		segments: NewStringPool(),
		paths:    NewStringPool(),
	}
}

// InternSegment returns a canonical path segment.
func (p *PathPool) InternSegment(segment string) string {
	return p.segments.Get(segment)
}

// InternPath returns a canonical full path.
func (p *PathPool) InternPath(path string) string {
	return p.paths.Get(path)
}

// Stats returns statistics about the path pool.
func (p *PathPool) Stats() (segments, paths, segmentHits, segmentMisses, pathHits, pathMisses int64) {
	segs, segHits, segMisses := p.segments.Stats()
	pthCount, pHits, pMisses := p.paths.Stats()
	return segs, pthCount, segHits, segMisses, pHits, pMisses
}
