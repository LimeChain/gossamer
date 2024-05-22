// Copyright 2023 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package database

import (
	"github.com/ChainSafe/gossamer/internal/database/interfaces"
	"io"
	"os"
	"path/filepath"
)

type Reader interface {
	Get(key []byte) ([]byte, error)
	Has(key []byte) (bool, error)
}

// Iterator iterates over key/value pairs in ascending key order.
// Must be released after use.
type Iterator interface {
	Valid() bool
	Next() bool
	Key() []byte
	Value() []byte
	First() bool
	Release()
	SeekGE(key []byte) bool
	io.Closer
}

// Database wraps all database operations. All methods are safe for concurrent use.
type Database interface {
	Reader
	interfaces.Writer
	io.Closer

	Path() string
	NewBatch() interfaces.Batch
	NewIterator() (Iterator, error)
	NewPrefixIterator(prefix []byte) (Iterator, error)
}

type Table interface {
	Reader
	interfaces.Writer
	Path() string
	NewBatch() interfaces.Batch
	NewIterator() (Iterator, error)
}

const DefaultDatabaseDir = "db"

// LoadDatabase will return an instance of database based on basepath
func LoadDatabase(basepath string, inMemory bool) (Database, error) {
	nodeDatabaseDir := filepath.Join(basepath, DefaultDatabaseDir)
	return NewPebble(nodeDatabaseDir, inMemory)
}

func ClearDatabase(basepath string) error {
	nodeDatabaseDir := filepath.Join(basepath, DefaultDatabaseDir)
	return os.RemoveAll(nodeDatabaseDir)
}
