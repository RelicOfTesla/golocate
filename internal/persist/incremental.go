package persist

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/RelicOfTesla/golocate/internal/database"
	"github.com/RelicOfTesla/golocate/pkg/index"
)

const (
	defaultFlushInterval  = 30 * time.Second
	defaultFlushThreshold = 200
)

// incrementalStrategy persists a full snapshot as a baseline (on builds) and
// then applies watcher-driven changes in batched, low-volume writes. The
// stored index therefore stays current without periodic full rewrites: a
// quiet filesystem writes nothing, and heavy change bursts are merged into a
// few transactions.
type incrementalStrategy struct {
	db *database.DB

	// Change buffer: per-path final state, deletes win over upserts.
	mu      sync.Mutex
	upserts map[string]*index.Entry
	deletes map[string]struct{}

	flushInterval  time.Duration
	flushThreshold int
	flushCh        chan struct{}

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newIncrementalStrategy(db *database.DB, flushInterval time.Duration, flushThreshold int) *incrementalStrategy {
	if flushInterval <= 0 {
		flushInterval = defaultFlushInterval
	}
	if flushThreshold <= 0 {
		flushThreshold = defaultFlushThreshold
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &incrementalStrategy{
		db:             db,
		upserts:        make(map[string]*index.Entry),
		deletes:        make(map[string]struct{}),
		flushInterval:  flushInterval,
		flushThreshold: flushThreshold,
		flushCh:        make(chan struct{}, 1),
		ctx:            ctx,
		cancel:         cancel,
	}
	s.wg.Add(1)
	go s.flushLoop()
	return s
}

// Restore implements Strategy. The database always holds the latest state
// (baseline + applied changes), so no age check is needed — unlike the
// snapshot strategy, there is no staleness to age.
func (s *incrementalStrategy) Restore(dirs []string) (*index.Index, bool, error) {
	if s.db == nil {
		return index.NewIndex(), false, nil
	}

	dirty, err := s.db.GetMeta(metaDirty)
	if err == nil && string(dirty) == "1" {
		slog.Warn("index snapshot marked dirty, ignoring it")
		return index.NewIndex(), false, nil
	}

	meta, err := s.db.GetMeta(metaDirectories)
	if err != nil || string(meta) != dirsKey(dirs) {
		return index.NewIndex(), false, nil
	}

	entries, err := s.db.GetAllEntries()
	if err != nil || len(entries) == 0 {
		return index.NewIndex(), false, nil
	}

	restored := index.NewIndexWithCapacity(len(entries))
	restored.AddBatch(entries)
	return restored, true, nil
}

// Persist implements Strategy: full baseline replacement. Buffered changes
// are intentionally kept — they are idempotent upserts/deletes that simply
// re-apply over the fresh baseline on the next flush.
func (s *incrementalStrategy) Persist(idx *index.Index, dirs []string) error {
	if s.db == nil {
		return nil
	}
	start := time.Now()

	if err := s.db.ReplaceAllEntries(idx.GetAllEntries()); err != nil {
		return fmt.Errorf("failed to persist baseline: %w", err)
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

	slog.Info("index baseline persisted", "entries", idx.Len(), "elapsed", time.Since(start))
	return nil
}

// ApplyChange implements Strategy: merge into the change buffer; flush when
// the buffer grows past the threshold.
func (s *incrementalStrategy) ApplyChange(change Change) error {
	s.mu.Lock()
	if change.Delete != "" {
		delete(s.upserts, change.Delete)
		s.deletes[change.Delete] = struct{}{}
	} else if change.Upsert != nil {
		delete(s.deletes, change.Upsert.Path)
		s.upserts[change.Upsert.Path] = change.Upsert
	}
	count := len(s.upserts) + len(s.deletes)
	needFlush := count >= s.flushThreshold
	s.mu.Unlock()

	if needFlush {
		select {
		case s.flushCh <- struct{}{}:
		default: // a flush is already pending
		}
	}
	return nil
}

// MarkDirty implements Strategy.
func (s *incrementalStrategy) MarkDirty() error {
	if s.db == nil {
		return nil
	}
	slog.Warn("marking index snapshot as dirty")
	return s.db.SetMeta(metaDirty, []byte("1"))
}

// Close implements Strategy: flush pending changes and stop the flush loop.
func (s *incrementalStrategy) Close() error {
	s.flush()
	s.cancel()
	s.wg.Wait()
	return nil
}

// flushLoop periodically flushes the change buffer.
func (s *incrementalStrategy) flushLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.flushCh:
			s.flush()
		case <-ticker.C:
			s.flush()
		}
	}
}

// flush applies all buffered changes in one transaction and clears the buffer.
func (s *incrementalStrategy) flush() {
	s.mu.Lock()
	if len(s.upserts) == 0 && len(s.deletes) == 0 {
		s.mu.Unlock()
		return
	}
	upserts := make([]*index.Entry, 0, len(s.upserts))
	for _, e := range s.upserts {
		upserts = append(upserts, e)
	}
	deletes := make([]string, 0, len(s.deletes))
	for p := range s.deletes {
		deletes = append(deletes, p)
	}
	s.upserts = make(map[string]*index.Entry)
	s.deletes = make(map[string]struct{})
	s.mu.Unlock()

	start := time.Now()
	if err := s.db.ApplyChanges(upserts, deletes); err != nil {
		slog.Error("failed to flush incremental changes", "error", err)
		return
	}
	slog.Info("incremental changes flushed",
		"upserts", len(upserts), "deletes", len(deletes), "elapsed", time.Since(start))
}
