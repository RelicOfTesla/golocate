package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RelicOfTesla/golocate/pkg/ignore"
	"github.com/RelicOfTesla/golocate/pkg/watcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdater_DirectoryCreateBackfills verifies that creating/moving in a
// directory indexes the files already inside it (inotify only emits a single
// event for the directory itself).
func TestUpdater_DirectoryCreateBackfills(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "inner.txt"), []byte("hi"), 0644))
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(sub, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "deep.go"), []byte("pkg"), 0644))

	idx := NewIndex()
	u := NewUpdater(idx)

	u.HandleEvent(watcher.Event{Path: dir, Op: watcher.Create})

	assert.True(t, idx.Len() >= 3, "dir + inner.txt + sub/deep.go should all be indexed, got %d", idx.Len())
	for _, p := range []string{dir, filepath.Join(dir, "inner.txt"), filepath.Join(sub, "deep.go")} {
		_, ok := idx.Get(p)
		assert.True(t, ok, "expected %s to be indexed", p)
	}
}

// TestUpdater_DirectoryBackfillRespectsIgnore verifies the ignore matcher is
// applied during backfill.
func TestUpdater_DirectoryBackfillRespectsIgnore(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("x"), 0644))
	ignored := filepath.Join(dir, "node_modules")
	require.NoError(t, os.MkdirAll(ignored, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(ignored, "lib.js"), []byte("y"), 0644))

	idx := NewIndex()
	u := NewUpdaterWithIgnore(idx, ignore.NewMatcher([]string{"node_modules"}))

	u.HandleEvent(watcher.Event{Path: dir, Op: watcher.Create})

	_, ok := idx.Get(filepath.Join(ignored, "lib.js"))
	assert.False(t, ok, "ignored path must not be backfilled")
	_, ok = idx.Get(filepath.Join(dir, "keep.txt"))
	assert.True(t, ok)
}

// TestUpdater_WriteRefreshesMetadata verifies a Write event updates size/mtime.
func TestUpdater_WriteRefreshesMetadata(t *testing.T) {
	file := filepath.Join(t.TempDir(), "data.bin")
	require.NoError(t, os.WriteFile(file, []byte("short"), 0644))

	idx := NewIndex()
	u := NewUpdater(idx)
	u.HandleEvent(watcher.Event{Path: file, Op: watcher.Create})

	entry, ok := idx.Get(file)
	require.True(t, ok)
	require.Equal(t, int64(5), entry.Size, "initial size should be 5")

	// Grow the file, then emit a Write event.
	require.NoError(t, os.WriteFile(file, []byte("a much longer content"), 0644))
	u.HandleEvent(watcher.Event{Path: file, Op: watcher.Write})

	entry, ok = idx.Get(file)
	require.True(t, ok)
	assert.Equal(t, int64(21), entry.Size, "Write event should refresh metadata")
}

// TestUpdater_CallbacksFire verifies the onUpsert/onDelete callbacks fire for
// create/remove events (used to feed persistence + content index).
func TestUpdater_CallbacksFire(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0644))

	idx := NewIndex()
	var upserts, deletes []string
	u := NewUpdaterWithCallbacks(idx, nil,
		func(e *Entry) { upserts = append(upserts, e.Path) },
		func(path string) { deletes = append(deletes, path) },
	)

	u.HandleEvent(watcher.Event{Path: file, Op: watcher.Create})
	require.Len(t, upserts, 1, "create should fire onUpsert")
	require.Equal(t, file, upserts[0])

	u.HandleEvent(watcher.Event{Path: file, Op: watcher.Remove})
	require.Len(t, deletes, 1, "remove should fire onDelete")
	require.Equal(t, file, deletes[0])

	// MoveFrom -> remove callback, MoveTo -> upsert callback.
	u.HandleEvent(watcher.Event{Path: file, Op: watcher.MoveFrom})
	u.HandleEvent(watcher.Event{Path: file, Op: watcher.MoveTo})
	require.Len(t, deletes, 2, "MoveFrom should fire onDelete")
	require.Len(t, upserts, 2, "MoveTo should fire onUpsert")
}

// TestUpdater_DeviceInfoPopulated verifies entries created by the updater
// carry Dev/Ino when the platform provides them (hard-link dedupe relies on
// this for watcher-added files too).
func TestUpdater_DeviceInfoPopulated(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a.txt")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0644))

	idx := NewIndex()
	u := NewUpdater(idx)
	u.HandleEvent(watcher.Event{Path: file, Op: watcher.Create})

	entry, ok := idx.Get(file)
	require.True(t, ok)
	if entry.Dev == 0 && entry.Ino == 0 {
		t.Log("note: platform provides no device/inode info; dedupe falls back to size+mtime")
	} else {
		assert.NotZero(t, entry.Ino, "inode should be recorded on unix")
	}
}
