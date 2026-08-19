package persist

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/RelicOfTesla/golocate/internal/database"
	"github.com/RelicOfTesla/golocate/pkg/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func testIndex() *index.Index {
	idx := index.NewIndex()
	idx.Add(&index.Entry{Name: "a.go", Path: "/tmp/a.go", Size: 10})
	idx.Add(&index.Entry{Name: "b.txt", Path: "/tmp/b.txt", Size: 20})
	return idx
}

// TestSnapshot_RoundTrip verifies Persist then Restore returns the same entries.
func TestSnapshot_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	s := newSnapshotStrategy(db, 0)

	idx := testIndex()
	require.NoError(t, s.Persist(idx, []string{"/tmp"}))

	got, restored, err := s.Restore([]string{"/tmp"})
	require.NoError(t, err)
	assert.True(t, restored)
	assert.Equal(t, 2, got.Len())
	_, ok := got.Get("/tmp/a.go")
	assert.True(t, ok)
}

// TestSnapshot_DirectoryMismatch verifies a different directory set refuses restore.
func TestSnapshot_DirectoryMismatch(t *testing.T) {
	db := openTestDB(t)
	s := newSnapshotStrategy(db, 0)

	require.NoError(t, s.Persist(testIndex(), []string{"/tmp"}))

	_, restored, err := s.Restore([]string{"/home"})
	require.NoError(t, err)
	assert.False(t, restored)
}

// TestSnapshot_DirtyRefusesRestore verifies a dirty snapshot is not restored.
func TestSnapshot_DirtyRefusesRestore(t *testing.T) {
	db := openTestDB(t)
	s := newSnapshotStrategy(db, 0)

	require.NoError(t, s.Persist(testIndex(), []string{"/tmp"}))
	require.NoError(t, s.MarkDirty())

	_, restored, err := s.Restore([]string{"/tmp"})
	require.NoError(t, err)
	assert.False(t, restored, "dirty snapshot must not be restored")

	// A fresh Persist clears the dirty flag.
	require.NoError(t, s.Persist(testIndex(), []string{"/tmp"}))
	_, restored, err = s.Restore([]string{"/tmp"})
	require.NoError(t, err)
	assert.True(t, restored)
}

// TestSnapshot_MaxAge verifies stale snapshots are refused when a max age is set.
func TestSnapshot_MaxAge(t *testing.T) {
	db := openTestDB(t)
	// 1 second max age: write the snapshot time far in the past.
	s := newSnapshotStrategy(db, 1)
	require.NoError(t, s.Persist(testIndex(), []string{"/tmp"}))
	require.NoError(t, db.SetMeta(metaSnapshotTime, []byte(time.Now().Add(-2*time.Hour).Format(time.RFC3339))))

	_, restored, err := s.Restore([]string{"/tmp"})
	require.NoError(t, err)
	assert.False(t, restored, "stale snapshot must not be restored")

	// maxAge=0 means never stale.
	s2 := newSnapshotStrategy(db, 0)
	_, restored, err = s2.Restore([]string{"/tmp"})
	require.NoError(t, err)
	assert.True(t, restored)
}

// TestNone_NeverRestores verifies the none strategy never restores or writes.
func TestNone_NeverRestores(t *testing.T) {
	s := &noneStrategy{}
	_, restored, err := s.Restore([]string{"/tmp"})
	require.NoError(t, err)
	assert.False(t, restored)
	require.NoError(t, s.Persist(testIndex(), []string{"/tmp"}))
	require.NoError(t, s.MarkDirty())
	require.NoError(t, s.Close())
}

// TestFactory_Modes verifies New builds the right strategy for each mode.
func TestFactory_Modes(t *testing.T) {
	db := openTestDB(t)

	// incremental (default path)
	s := New(ModeIncremental, db, Options{FlushInterval: 20 * time.Millisecond})
	require.NoError(t, s.Persist(testIndex(), []string{"/tmp"}))
	_, restored, err := s.Restore([]string{"/tmp"})
	require.NoError(t, err)
	assert.True(t, restored)
	require.NoError(t, s.Close())

	// snapshot
	s2 := New(ModeSnapshot, db, Options{})
	require.NoError(t, s2.Persist(testIndex(), []string{"/tmp"}))
	_, restored, err = s2.Restore([]string{"/tmp"})
	require.NoError(t, err)
	assert.True(t, restored)
	require.NoError(t, s2.Close())

	// none
	s3 := New(ModeNone, nil, Options{})
	_, restored, err = s3.Restore([]string{"/tmp"})
	require.NoError(t, err)
	assert.False(t, restored)
	require.NoError(t, s3.Close())
	_ = os.TempDir
}
