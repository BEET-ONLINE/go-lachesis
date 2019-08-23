package kvdb

import (
	"bytes"
	"sync"

	"github.com/Fantom-foundation/go-lachesis/lachesis/src/common"
)

var (
	// NOTE: key collisions are possible
	separator = []byte("::")
)

// MemDatabase is a kvbd.Database wrapper of map[string][]byte
// Do not use for any production it does not get persisted
type MemDatabase struct {
	db     map[string][]byte
	prefix []byte
	lock   *sync.RWMutex
}

// NewMemDatabase wraps map[string][]byte
func NewMemDatabase() *MemDatabase {
	return &MemDatabase{
		db:   make(map[string][]byte),
		lock: new(sync.RWMutex),
	}
}

/*
 * Database interface implementation
 */

// NewTable returns a Database object that prefixes all keys with a given prefix.
func (w *MemDatabase) NewTable(prefix []byte) Database {
	base := common.CopyBytes(w.prefix)
	return &MemDatabase{
		db:     w.db,
		prefix: append(append(base, []byte("-")...), prefix...),
		lock:   w.lock,
	}
}

func (w *MemDatabase) fullKey(key []byte) []byte {
	base := common.CopyBytes(w.prefix)
	return append(append(base, separator...), key...)
}

// Put puts key-value pair into db.
func (w *MemDatabase) Put(key []byte, value []byte) error {
	w.lock.Lock()
	defer w.lock.Unlock()

	key = w.fullKey(key)

	w.db[string(key)] = common.CopyBytes(value)
	return nil
}

// Has checks if key is in the db.
func (w *MemDatabase) Has(key []byte) (bool, error) {
	w.lock.RLock()
	defer w.lock.RUnlock()

	key = w.fullKey(key)

	_, ok := w.db[string(key)]
	return ok, nil
}

// Get returns key-value pair by key.
func (w *MemDatabase) Get(key []byte) ([]byte, error) {
	w.lock.RLock()
	defer w.lock.RUnlock()

	key = w.fullKey(key)

	if entry, ok := w.db[string(key)]; ok {
		return common.CopyBytes(entry), nil
	}
	return nil, nil
}

// ForEach scans key-value pair by key prefix.
func (w *MemDatabase) ForEach(prefix []byte, do func(key, val []byte) bool) error {
	w.lock.RLock()
	defer w.lock.RUnlock()

	prefix = w.fullKey(prefix)

	for k, val := range w.db {
		key := common.CopyBytes([]byte(k))
		if bytes.HasPrefix(key, prefix) {
			key = key[len(w.prefix)+len(separator):]
			if !do(key, val) {
				break
			}
		}
	}

	return nil
}

// Delete removes key-value pair by key.
func (w *MemDatabase) Delete(key []byte) error {
	w.lock.Lock()
	defer w.lock.Unlock()

	key = w.fullKey(key)

	delete(w.db, string(key))
	return nil
}

// Close leaves underlying database.
func (w *MemDatabase) Close() {
	w.db = nil
}

// NewBatch creates new batch.
func (w *MemDatabase) NewBatch() Batch {
	return &memBatch{db: w}
}

/*
 * Batch
 */

type kv struct {
	k, v []byte
	del  bool
}

// memBatch is a batch structure.
type memBatch struct {
	db     *MemDatabase
	writes []kv
	size   int
}

// Put puts key-value pair into batch.
func (b *memBatch) Put(key, value []byte) error {
	key = b.db.fullKey(key)

	b.writes = append(b.writes, kv{common.CopyBytes(key), common.CopyBytes(value), false})
	b.size += len(value)
	return nil
}

// Delete removes key-value pair from batch by key.
func (b *memBatch) Delete(key []byte) error {
	key = b.db.fullKey(key)

	b.writes = append(b.writes, kv{common.CopyBytes(key), nil, true})
	b.size++
	return nil
}

// Write writes batch into db.
func (b *memBatch) Write() error {
	b.db.lock.Lock()
	defer b.db.lock.Unlock()

	for _, kv := range b.writes {
		if kv.del {
			delete(b.db.db, string(kv.k))
			continue
		}
		b.db.db[string(kv.k)] = kv.v
	}
	return nil
}

// ValueSize returns values sizes sum.
func (b *memBatch) ValueSize() int {
	return b.size
}

// Reset cleans whole batch.
func (b *memBatch) Reset() {
	b.writes = b.writes[:0]
	b.size = 0
}
