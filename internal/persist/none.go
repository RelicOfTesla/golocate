package persist

import "github.com/RelicOfTesla/golocate/pkg/index"

// noneStrategy disables persistence entirely: nothing is written and nothing
// is restored, so every cold start performs a full rebuild.
type noneStrategy struct{}

func (s *noneStrategy) Restore(dirs []string) (*index.Index, bool, error) {
	return index.NewIndex(), false, nil
}

func (s *noneStrategy) Persist(idx *index.Index, dirs []string) error { return nil }

func (s *noneStrategy) ApplyChange(change Change) error { return nil }

func (s *noneStrategy) MarkDirty() error { return nil }

func (s *noneStrategy) Close() error { return nil }
