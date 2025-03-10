package kvstore

import (
	"database/sql"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)

// KVStore represents a key-value store backed by SQLite.
type KVStore struct {
	mu sync.RWMutex
	db *sql.DB
}

// NewKVStore creates a new instance of KVStore.
func NewKVStore(dbPath string) (*KVStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS kv_store (
            key TEXT PRIMARY KEY,
            value TEXT
        )
    `)
	if err != nil {
		return nil, fmt.Errorf("failed to create table: %v", err)
	}

	return &KVStore{db: db}, nil
}

// Put stores a key-value pair in the store.
func (kv *KVStore) Put(key, value string) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	_, err := kv.db.Exec(`
        INSERT OR REPLACE INTO kv_store (key, value)
        VALUES (?, ?)
    `, key, value)
	return err
}

// Get retrieves the value for a key from the store.
func (kv *KVStore) Get(key string) (string, bool, error) {
	kv.mu.RLock()
	defer kv.mu.RUnlock()

	var value string
	err := kv.db.QueryRow(`
        SELECT value FROM kv_store WHERE key = ?
    `, key).Scan(&value)

	if err == sql.ErrNoRows {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	}

	return value, true, nil
}

// Delete removes a key-value pair from the store.
func (kv *KVStore) Delete(key string) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	_, err := kv.db.Exec(`
        DELETE FROM kv_store WHERE key = ?
    `, key)
	return err
}

// Close closes the database connection.
func (kv *KVStore) Close() error {
	return kv.db.Close()
}
