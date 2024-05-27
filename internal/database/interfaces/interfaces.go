package interfaces

import "io"

type Writer interface {
	Put(key, value []byte) error
	Del(key []byte) error
	Flush() error
}

// Batch is a write-only operation.
type Batch interface {
	io.Closer
	Writer

	ValueSize() int
	Reset()
}
