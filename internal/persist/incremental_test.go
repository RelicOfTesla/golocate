package persist

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/RelicOfTesla/golocate/pkg/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIncremental_RoundTrip verifies baseline + applied changes are restored.
func TestIncremental_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	s := newIncrementalStrategy(db, 20*time.Millisecond, 1000)
	defer s.Close()

	require.NoError(t, s.Persist(testIndex(), []string{"/tmp"}))

	// Watcher-driven changes: upsert + delete.
	require.NoError(t, s.ApplyChange(Change{Upsert: &index.Entry{Name: "c.txt", Path: "/tmp/c.txt", Size: 5}}))
	require.NoError(t, s.ApplyChange(Change{Delete: "/tmp/a.go"}))

	// Wait for the periodic flush.
	deadline := time.Now().Add(2 * time.Second)
	for {
		s.mu.Lock()
		buffered := len(s.upserts) + len(s.deletes)
		s.mu.Unlock()
		if buffered == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("changes were not flushed in time")
		}
		time.Sleep(20 * time.Millisecond)
	}

	got, restored, err := s.Restore([]string{"/tmp"})
	require.NoError(t, err)
	assert.True(t, restored)
	assert.Equal(t, 2, got.Len(), "baseline minus deleted plus upserted")
	_, ok := got.Get("/tmp/c.txt")
	assert.True(t, ok, "upserted entry should be present")
	_, ok = got.Get("/tmp/a.go")
	assert.False(t, ok, "deleted entry should be gone")
}

// TestIncremental_DeleteWinsOverUpsert verifies the buffer merges by path and
// deletes take precedence.
func TestIncremental_DeleteWinsOverUpsert(t *testing.T) {
	db := openTestDB(t)
	s := newIncrementalStrategy(db, 20*time.Millisecond, 1000)
	defer s.Close()

	require.NoError(t, s.ApplyChange(Change{Upsert: &index.Entry{Name: "x", Path: "/tmp/x", Size: 1}}))
	require.NoError(t, s.ApplyChange(Change{Delete: "/tmp/x"}))
	require.NoError(t, s.ApplyChange(Change{Upsert: &index.Entry{Name: "x", Path: "/tmp/x", Size: 9}}))

	s.mu.Lock()
	assert.Len(t, s.upserts, 1, "path merged to one upsert")
	assert.Len(t, s.deletes, 0, "later upsert cancels the delete")
	s.mu.Unlock()
}

// TestIncremental_ThresholdFlush verifies a large buffer flushes immediately.
func TestIncremental_ThresholdFlush(t *testing.T) {
	db := openTestDB(t)
	s := newIncrementalStrategy(db, time.Hour, 5) // interval never fires; threshold does
	defer s.Close()

	for i := 0; i < 6; i++ {
		require.NoError(t, s.ApplyChange(Change{
			Upsert: &index.Entry{Name: "f", Path: filepath.Join("/tmp", "f"+string(rune('0'+i))), Size: 1},
		}))
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		s.mu.Lock()
		buffered := len(s.upserts)
		s.mu.Unlock()
		if buffered == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("threshold flush did not happen")
		}
		time.Sleep(20 * time.Millisecond)
	}

	entries, err := db.GetAllEntries()
	require.NoError(t, err)
	assert.Equal(t, 6, len(entries))
}

// TestIncremental_DirtyRefusesRestore verifies dirty refuses restore and a
// fresh baseline clears it.
func TestIncremental_DirtyRefusesRestore(t *testing.T) {
	db := openTestDB(t)
	s := newIncrementalStrategy(db, time.Hour, 100)
	defer s.Close()

	require.NoError(t, s.Persist(testIndex(), []string{"/tmp"}))
	require.NoError(t, s.MarkDirty())

	_, restored, err := s.Restore([]string{"/tmp"})
	require.NoError(t, err)
	assert.False(t, restored)

	require.NoError(t, s.Persist(testIndex(), []string{"/tmp"}))
	_, restored, err = s.Restore([]string{"/tmp"})
	require.NoError(t, err)
	assert.True(t, restored)
}
