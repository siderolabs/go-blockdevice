// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package lba

import (
	"fmt"
	"os"
)

// Buffer is an in-memory buffer for writing to byte slices in units of LBA.
type Buffer struct {
	lba *LBA
	b   []byte
}

// NewBuffer intializes and returns a `Buffer`.
func NewBuffer(lba *LBA, b []byte) *Buffer {
	return &Buffer{lba: lba, b: b}
}

// Read reads from a `Buffer`.
func (buf *Buffer) Read(off, length int64) (b []byte, err error) {
	b = make([]byte, length)

	n := copy(b, buf.b[off:off+length])

	if n != len(buf.b[off:off+length]) {
		return nil, fmt.Errorf("expected to write %d bytes, read %d", len(b), n)
	}

	return b, nil
}

// Write writes to a `Buffer`.
func (buf *Buffer) Write(b []byte, off int64) (err error) {
	n := copy(buf.b[off:off+int64(len(b))], b)

	if n != len(b) {
		return fmt.Errorf("expected to write %d bytes, wrote %d", len(b), n)
	}

	return nil
}

// Bytes returns the buffer bytes.
func (buf *Buffer) Bytes() []byte {
	return buf.b
}

// LBA represents logical block addressing.
type LBA struct {
	PhysicalBlockSize int64
	LogicalBlockSize  int64
	MinimalIOSize     int64
	OptimalIOSize     int64

	TotalSectors int64

	f *os.File
}

// AlignToPhysicalBlockSize aligns LBA value in LogicalBlockSize multiples to be aligned to PhysicalBlockSize.
func (l *LBA) AlignToPhysicalBlockSize(lba uint64) uint64 {
	physToLogical := uint64(l.PhysicalBlockSize / l.LogicalBlockSize)
	minIOToLogical := uint64(l.MinimalIOSize / l.LogicalBlockSize)

	ratio := physToLogical
	if minIOToLogical > ratio {
		ratio = minIOToLogical
	}

	if ratio <= 1 {
		return lba
	}

	return (lba + ratio - 1) / ratio * ratio
}
