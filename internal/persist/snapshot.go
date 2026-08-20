package persist

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/RelicOfTesla/golocate/internal/database"
	"github.com/RelicOfTesla/golocate/pkg/index"
)

// Meta keys stored in the database's meta bucket.
const (
	metaDirectories  = "directories"   // dirsKey of the indexed directory set
	metaSnapshotTime = "snapshot_time" // RFC3339 time of the last persisted snapshot
	metaDirty        = "dirty"         // "1" when the index may be incomplete
)

// snapshotStrategy persists the full index after each build and restores it
// on start when it looks trustworthy: the directory fingerprint matches, the
// snapshot is not marked dirty, and it is not older than the configured max
// age (0 = never stale).
type snapshotStrategy struct {
	db             *database.DB
	snapshotMaxAge time.Duration // 0 = never stale
}

func newSnapshotStrategy(db *database.DB, snapshotMaxAgeSecs int64) *snapshotStrategy {
	return &snapshotStrategy{
		db:             db,
		snapshotMaxAge: time.Duration(snapshotMaxAgeSecs) * time.Second,
	}
}

// Restore implements Strategy.
func (s *snapshotStrategy) Restore(dirs []string) (*index.Index, bool, error) {
	if s.db == nil {
		return index.NewIndex(), false, nil
	}

	// Never restore a dirty snapshot (watcher event loss may have left gaps).
	dirty, err := s.db.GetMeta(metaDirty)
	if err == nil && string(dirty) == "1" {
		slog.Warn("index snapshot marked dirty, ignoring it")
		return index.NewIndex(), false, nil
	}

	// Only restore when the persisted directory set matches the current
	// config, otherwise the index would be stale or miss new directories.
	meta, err := s.db.GetMeta(metaDirectories)
	if err != nil || string(meta) != dirsKey(dirs) {
		return index.NewIndex(), false, nil
	}

	// Freshness check: an old snapshot is still usable immediately, but the
	// caller prefers a background rebuild over trusting it.
	if s.snapshotMaxAge > 0 {
		snapTime, err := s.db.GetMeta(metaSnapshotTime)
		if err == nil && len(snapTime) > 0 {
			if t, perr := time.Parse(time.RFC3339, string(snapTime)); perr == nil {
				if age := time.Since(t); age > s.snapshotMaxAge {
					slog.Info("snapshot too old, will rebuild in background", "age", age.Round(time.Second))
					return index.NewIndex(), false, nil
				}
			}
		}
	}

	entries, err := s.db.GetAllEntries()
	if err != nil || len(entries) == 0 {
		return index.NewIndex(), false, nil
	}

	restored := index.NewIndexWithCapacity(len(entries))
	restored.AddBatch(entries)
	return restored, true, nil
}

// Persist implements Strategy.
func (s *snapshotStrategy) Persist(idx *index.Index, dirs []string) error {
	if s.db == nil {
		return nil
	}
	start := time.Now()

	// Atomically replace all entries in one transaction.
	if err := s.db.ReplaceAllEntries(idx.GetAllEntries()); err != nil {
		return fmt.Errorf("failed to persist snapshot: %w", err)
	}

	now := time.Now()
	if err := s.db.SetMeta(metaDirectories, []byte(dirsKey(dirs))); err != nil {
		return fmt.Errorf("failed to persist directories meta: %w", err)
	}
	if err := s.db.SetMeta(metaSnapshotTime, []byte(now.Format(time.RFC3339))); err != nil {
		return fmt.Errorf("failed to persist snapshot time meta: %w", err)
	}
	if err := s.db.SetMeta(metaDirty, []byte("0")); err != nil {
		return fmt.Errorf("failed to clear dirty meta: %w", err)
	}

	slog.Info("index snapshot persisted", "entries", idx.Len(), "elapsed", time.Since(start))
	return nil
}

// MarkDirty implements Strategy.
func (s *snapshotStrategy) MarkDirty() error {
	if s.db == nil {
		return nil
	}
	slog.Warn("marking index snapshot as dirty")
	return s.db.SetMeta(metaDirty, []byte("1"))
}

// ApplyChange implements Strategy: snapshots are only written on builds, so
// watcher-driven changes are ignored here (they live in the in-memory index).
func (s *snapshotStrategy) ApplyChange(change Change) error { return nil }

// Close implements Strategy.
func (s *snapshotStrategy) Close() error { return nil }

// dirsKey returns a stable key for a directory list (sorted, NUL-joined).
func dirsKey(dirs []string) string {
	sorted := append([]string(nil), dirs...)
	sort.Strings(sorted)
	return strings.Join(sorted, "\x00")
}
