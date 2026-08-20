// Package persist provides pluggable index persistence strategies.
//
// The daemon treats the in-memory index as authoritative; persistence only
// exists to make restarts cheap (open the tool and search immediately instead
// of waiting for a full rescan). Because the index is derived data that can be
// rebuilt from the filesystem in seconds, strategies are allowed to trade
// durability for fewer disk writes:
//
//   - none:        no persistence at all; cold start always rebuilds
//   - snapshot:    full snapshot written after each index build; restored on
//     start when the directory fingerprint matches, the snapshot
//     is not marked dirty, and it is fresh enough
//   - incremental: full snapshot as a baseline plus watcher-driven changes
//     applied in batched low-volume writes, so the stored index
//     stays current without periodic full rewrites
//
// The daemon depends only on the Strategy interface, so new persistence
// backends can be plugged in without touching the service logic.
package persist

import (
	"time"

	"github.com/RelicOfTesla/golocate/internal/database"
	"github.com/RelicOfTesla/golocate/pkg/index"
)

// Change represents one index mutation from the watcher. Exactly one of
// Upsert / Delete is set.
type Change struct {
	Upsert *index.Entry // non-nil: add or update this entry
	Delete string       // non-empty: remove this path (takes precedence over Upsert)
}

// Strategy is the persistence component interface.
type Strategy interface {
	// Restore attempts to restore an index usable at startup.
	// restored=false means the caller should serve with an empty index and
	// trigger a (throttled) background rebuild.
	Restore(dirs []string) (idx *index.Index, restored bool, err error)

	// Persist is called after an index build completes (initial build, manual
	// build, config-change rebuild, scheduler rebuild) to persist the result.
	// dirs is the directory set the index was built from (recorded so a later
	// Restore can detect configuration changes).
	Persist(idx *index.Index, dirs []string) error

	// ApplyChange feeds a watcher-driven mutation to the strategy. Snapshot
	// and none strategies ignore it; incremental strategies buffer it and
	// flush in batches.
	ApplyChange(change Change) error

	// MarkDirty records that the index may be incomplete (e.g. watcher event
	// loss); the next Restore will refuse the stored data.
	MarkDirty() error

	// Close flushes any buffered changes and releases resources.
	Close() error
}

// Mode constants for the persist_mode config value.
const (
	ModeSnapshot    = "snapshot"
	ModeNone        = "none"
	ModeIncremental = "incremental"
)

// Options configures a persistence strategy.
type Options struct {
	SnapshotMaxAgeSecs int64         // snapshot mode: max snapshot age in seconds (0 = never stale)
	FlushInterval      time.Duration // incremental mode: batch flush interval (0 = default 30s)
	FlushThreshold     int           // incremental mode: buffer size that triggers an immediate flush
}

// New creates the persistence strategy selected by cfg.PersistMode.
// db may be nil when mode is "none".
func New(mode string, db *database.DB, opts Options) Strategy {
	switch mode {
	case ModeNone:
		return &noneStrategy{}
	case ModeSnapshot:
		return newSnapshotStrategy(db, opts.SnapshotMaxAgeSecs)
	default: // ModeIncremental
		return newIncrementalStrategy(db, opts.FlushInterval, opts.FlushThreshold)
	}
}
