// Package database provides index persistence using BBolt.
package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/RelicOfTesla/golocate/pkg/index"
	bolt "go.etcd.io/bbolt"
	bbolterr "go.etcd.io/bbolt/errors"
)

// DB wraps BBolt database for index storage.
type DB struct {
	db   *bolt.DB
	path string
	mu   sync.RWMutex
}

// buckets
var (
	filesBucket = []byte("files")
	metaBucket  = []byte("meta")
)

// Open opens or creates the database at the given path.
func Open(path string) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := bolt.Open(path, 0600, &bolt.Options{
		Timeout: 1 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create buckets if they don't exist
	err = db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(filesBucket); err != nil {
			return fmt.Errorf("failed to create files bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists(metaBucket); err != nil {
			return fmt.Errorf("failed to create meta bucket: %w", err)
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	return &DB{db: db, path: path}, nil
}

// Close closes the database.
func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.db.Close()
}

// SaveEntry saves a file entry to the database.
func (d *DB) SaveEntry(entry *index.Entry) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
	}

	return d.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(filesBucket)
		return b.Put([]byte(entry.Path), data)
	})
}

// DeleteEntry removes a file entry from the database.
func (d *DB) DeleteEntry(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(filesBucket)
		return b.Delete([]byte(path))
	})
}

// GetEntry retrieves a file entry by path.
func (d *DB) GetEntry(path string) (*index.Entry, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var entry index.Entry
	err := d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(filesBucket)
		data := b.Get([]byte(path))
		if data == nil {
			return fmt.Errorf("entry not found: %s", path)
		}
		return json.Unmarshal(data, &entry)
	})
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// GetAllEntries retrieves all file entries.
func (d *DB) GetAllEntries() ([]*index.Entry, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var entries []*index.Entry
	err := d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(filesBucket)
		return b.ForEach(func(k, v []byte) error {
			var entry index.Entry
			if err := json.Unmarshal(v, &entry); err != nil {
				return err
			}
			entries = append(entries, &entry)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// Search searches for entries matching the query.
func (d *DB) Search(query string, opts index.SearchOptions) ([]*index.Entry, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var results []*index.Entry
	err := d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(filesBucket)
		return b.ForEach(func(k, v []byte) error {
			var entry index.Entry
			if err := json.Unmarshal(v, &entry); err != nil {
				return err
			}

			// Check if matches
			target := string(k)
			if opts.Basename {
				target = entry.Name
			}

			matches := false
			if opts.IgnoreCase {
				matches = strings.Contains(strings.ToLower(target), strings.ToLower(query))
			} else {
				matches = strings.Contains(target, query)
			}

			if matches {
				results = append(results, &entry)
				if opts.Limit > 0 && len(results) >= opts.Limit {
					return nil
				}
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// Count returns the number of entries matching the query.
func (d *DB) Count(query string, opts index.SearchOptions) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	count := 0
	err := d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(filesBucket)
		return b.ForEach(func(k, v []byte) error {
			var entry index.Entry
			if err := json.Unmarshal(v, &entry); err != nil {
				return err
			}

			// Check if matches
			target := string(k)
			if opts.Basename {
				target = entry.Name
			}

			matches := false
			if opts.IgnoreCase {
				matches = strings.Contains(strings.ToLower(target), strings.ToLower(query))
			} else {
				matches = strings.Contains(target, query)
			}

			if matches {
				count++
			}
			return nil
		})
	})
	return count, err
}

// GetStats returns database statistics.
func (d *DB) GetStats() (map[string]any, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stats := make(map[string]any)
	err := d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(filesBucket)
		stats["file_count"] = b.Stats().KeyN
		return nil
	})
	return stats, err
}

// SetMeta sets a metadata value.
func (d *DB) SetMeta(key string, value []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(metaBucket)
		return b.Put([]byte(key), value)
	})
}

// GetMeta gets a metadata value.
func (d *DB) GetMeta(key string) ([]byte, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var value []byte
	err := d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(metaBucket)
		v := b.Get([]byte(key))
		if v != nil {
			value = make([]byte, len(v))
			copy(value, v)
		}
		return nil
	})
	return value, err
}

// ApplyChanges applies a batch of upserts and deletes in a single
// transaction. This is the incremental persistence path: the write volume is
// proportional to the actual changes, not the whole index.
func (d *DB) ApplyChanges(upserts []*index.Entry, deletes []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(filesBucket)
		for _, entry := range upserts {
			data, err := json.Marshal(entry)
			if err != nil {
				return fmt.Errorf("failed to marshal entry %s: %w", entry.Path, err)
			}
			if err := b.Put([]byte(entry.Path), data); err != nil {
				return fmt.Errorf("failed to put entry %s: %w", entry.Path, err)
			}
		}
		for _, path := range deletes {
			if err := b.Delete([]byte(path)); err != nil {
				return fmt.Errorf("failed to delete entry %s: %w", path, err)
			}
		}
		return nil
	})
}

// ReplaceAllEntries atomically replaces all entries in the database.
// This operation is performed within a single transaction, ensuring atomicity.
// If the operation fails, the database remains unchanged.
func (d *DB) ReplaceAllEntries(entries []*index.Entry) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.db.Update(func(tx *bolt.Tx) error {
		// Clear existing entries by deleting and recreating the bucket
		// Note: BBolt doesn't have a "clear bucket" operation,
		// so we delete and recreate the bucket
		if err := tx.DeleteBucket(filesBucket); err != nil && !errors.Is(err, bbolterr.ErrBucketNotFound) {
			return fmt.Errorf("failed to delete files bucket: %w", err)
		}

		// Recreate the bucket
		newB, err := tx.CreateBucket(filesBucket)
		if err != nil {
			return fmt.Errorf("failed to recreate files bucket: %w", err)
		}

		// Write new entries
		for _, entry := range entries {
			data, err := json.Marshal(entry)
			if err != nil {
				return fmt.Errorf("failed to marshal entry %s: %w", entry.Path, err)
			}
			if err := newB.Put([]byte(entry.Path), data); err != nil {
				return fmt.Errorf("failed to put entry %s: %w", entry.Path, err)
			}
		}

		return nil
	})
}
