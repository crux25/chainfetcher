package db

import (
	"strconv"

	"chainfetcher/types"

	"github.com/dgraph-io/badger/v4"
	"github.com/vmihailenco/msgpack/v5"
)

type DB struct {
	badger *badger.DB
}

// InitDB initializes the database
func InitDB(path string) (*DB, error) {
	opts := badger.DefaultOptions(path).WithLogger(nil)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}
	return &DB{badger: db}, nil
}

// CloseDB closes the database
func (db *DB) CloseDB() error {
	return db.badger.Close()
}

// GetNextIndex retrieves and increments the global log index
func (db *DB) GetNextIndex() (uint64, error) {
	var nextIndex uint64

	err := db.badger.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("log_index"))
		if err == nil {
			// Read the stored index
			err = item.Value(func(val []byte) error {
				nextIndex, _ = strconv.ParseUint(string(val), 10, 64)
				return nil
			})
			if err != nil {
				return err
			}
		} else if err == badger.ErrKeyNotFound {
			// If the key does not exist, start from 0 (not 1)
			nextIndex = 0
		} else {
			return err
		}

		// Store the incremented index
		return txn.Set([]byte("log_index"), []byte(strconv.FormatUint(nextIndex+1, 10)))
	})

	if err != nil {
		return 0, err
	}

	// Return the current index BEFORE incrementing
	return nextIndex, nil
}

// StoreLog saves the log entry using MsgPack serialization
func (db *DB) StoreLog(logEntry types.LogEntry) error {
	return db.badger.Update(func(txn *badger.Txn) error {
		// Convert log entry to binary format
		data, err := msgpack.Marshal(logEntry)
		if err != nil {
			return err
		}
		// Store in BadgerDB with key "log_<index>"
		key := []byte("log_" + strconv.FormatUint(logEntry.Index, 10))
		return txn.Set(key, data)
	})
}

// GetLog retrieves and deserializes a log entry by index
func (db *DB) GetLog(index uint64) (*types.LogEntry, error) {
	var logEntry types.LogEntry

	err := db.badger.View(func(txn *badger.Txn) error {
		key := []byte("log_" + strconv.FormatUint(index, 10))
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return msgpack.Unmarshal(val, &logEntry)
		})
	})
	if err != nil {
		return nil, err
	}
	return &logEntry, nil
}
