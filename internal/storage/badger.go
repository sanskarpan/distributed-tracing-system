package storage

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/yourname/tracing/internal/model"
)

// BadgerStore wraps MemoryStore with durable BadgerDB persistence.
// Reads are served from the in-memory index; writes go to both.
// On Open, all stored traces are loaded back into the in-memory store.
type BadgerStore struct {
	*MemoryStore
	db *badger.DB
}

// OpenBadger opens (or creates) a BadgerDB database at dir and returns a BadgerStore.
// The in-memory store is seeded from persisted data on startup.
func OpenBadger(dir string, maxSize int) (*BadgerStore, error) {
	opts := badger.DefaultOptions(dir).
		WithLogger(nil). // suppress BadgerDB's noisy default logging
		WithNumVersionsToKeep(1).
		WithCompactL0OnClose(true)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("badger open %q: %w", dir, err)
	}

	mem := NewMemoryStore(maxSize)
	bs := &BadgerStore{MemoryStore: mem, db: db}

	if err := bs.loadAll(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("badger load: %w", err)
	}

	// Launch periodic GC to reclaim space from deleted/expired values.
	go bs.runGC()

	return bs, nil
}

// Close flushes pending writes and closes the BadgerDB handle.
func (bs *BadgerStore) Close() error {
	return bs.db.Close()
}

// Upsert persists a trace to BadgerDB and updates the in-memory index.
func (bs *BadgerStore) Upsert(trace *model.Trace) error {
	if err := bs.MemoryStore.Upsert(trace); err != nil {
		return err
	}
	return bs.persist(trace)
}

// persist serialises a single trace into BadgerDB.
func (bs *BadgerStore) persist(trace *model.Trace) error {
	data, err := json.Marshal(trace)
	if err != nil {
		return err
	}
	key := traceKey(trace.TraceID)
	return bs.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
}

// loadAll reads every trace from BadgerDB and inserts it into the MemoryStore.
func (bs *BadgerStore) loadAll() error {
	return bs.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte("trace:")
		it := txn.NewIterator(opts)
		defer it.Close()

		loaded := 0
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			if err := item.Value(func(val []byte) error {
				var trace model.Trace
				if err := json.Unmarshal(val, &trace); err != nil {
					return nil // skip corrupt entries
				}
				return bs.MemoryStore.Upsert(&trace)
			}); err != nil {
				log.Printf("badger: skipping corrupt trace: %v", err)
			}
			loaded++
		}
		if loaded > 0 {
			log.Printf("badger: loaded %d traces from disk", loaded)
		}
		return nil
	})
}

// runGC runs BadgerDB value-log GC periodically to reclaim disk space.
func (bs *BadgerStore) runGC() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		for {
			if err := bs.db.RunValueLogGC(0.5); err != nil {
				break // no more GC runs needed
			}
		}
	}
}

func traceKey(id model.TraceID) []byte {
	return []byte("trace:" + id.String())
}
