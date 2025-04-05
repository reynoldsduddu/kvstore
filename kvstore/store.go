package kvstore

import (
	"database/sql"
	"fmt"
	"kvstore/consensus"
	"sync"

	_ "modernc.org/sqlite"
)

// KVStore represents a key-value store backed by SQLite.
type KVStore struct {
	mu        sync.RWMutex
	db        *sql.DB
	consensus *consensus.Consensus
}

// NewKVStore initializes the store with consensus.
func NewKVStore(dbPath string, consensus *consensus.Consensus) (*KVStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	fmt.Println("📂 Opening database at:", dbPath)
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

	return &KVStore{db: db, consensus: consensus}, nil
}

// Put stores a key-value pair in the store after reaching consensus.
func (kv *KVStore) Put(key, value string) error {
	fmt.Printf("Attempting consensus for key=%s, value=%s\n", key, value)

	if kv.consensus.ProposeChange("PUT", key, value) {
		kv.mu.Lock()
		defer kv.mu.Unlock()

		_, err := kv.db.Exec(`INSERT OR REPLACE INTO kv_store (key, value) VALUES (?, ?)`, key, value)
		if err != nil {
			fmt.Printf("SQLite write failed for key=%s: %v\n", key, err)
		} else {
			fmt.Printf("SQLite write successful for key=%s\n", key)
		}
		return err
	}

	fmt.Printf("Consensus rejected PUT request for key=%s\n", key)
	return fmt.Errorf("consensus not reached for key=%s", key)
}

// Get retrieves the value for a key (reads do not require consensus).
func (kv *KVStore) Get(key string) (string, bool, error) {
	kv.mu.RLock()
	defer kv.mu.RUnlock()

	var value string
	err := kv.db.QueryRow(`SELECT value FROM kv_store WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return value, true, err
}

// Delete removes a key-value pair after reaching consensus.
func (kv *KVStore) Delete(key string) error {
	if kv.consensus.ProposeChange("DELETE", key, "") {
		kv.mu.Lock()
		defer kv.mu.Unlock()

		_, err := kv.db.Exec(`DELETE FROM kv_store WHERE key = ?`, key)
		return err
	}
	return fmt.Errorf("consensus not reached")
}
func (kv *KVStore) ReplicatedPut(key, value string) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	kv.db.Exec(`INSERT OR REPLACE INTO kv_store (key, value) VALUES (?, ?)`, key, value)
}

func (kv *KVStore) ReplicatedDelete(key string) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	kv.db.Exec(`DELETE FROM kv_store WHERE key = ?`, key)
}

// Close closes the database connection.
func (kv *KVStore) Close() error {
	return kv.db.Close()
}
