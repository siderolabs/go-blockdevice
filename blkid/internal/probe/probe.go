// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package probe defines common probe interfaces.
package probe

import (
	"io"

	"github.com/google/uuid"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
)

// Reader is a context for probing filesystems and volume managers.
type Reader interface {
	io.ReaderAt

	GetSectorSize() uint
	GetSize() uint64
}

// Prober is an interface for probing filesystems and volume managers.
type Prober interface {
	// Name returns the name of the filesystem or volume manager.
	Name() string
	// Magic returns the magic value for the filesystem or volume manager.
	Magic() []*magic.Magic
	// Probe runs the further inspection and returns the result if successful.
	Probe(Reader) (*Result, error)
}

// Result is a probe result.
type Result struct {
	UUID  *uuid.UUID
	Label *string

	Parts []Partition

	BlockSize           uint32
	FilesystemBlockSize uint32
	ProbedSize          uint64
}

// Partition is a probe sub-result.
type Partition struct {
	UUID     *uuid.UUID
	TypeUUID *uuid.UUID
	Label    *string

	Index uint // 1-based index

	Offset uint64
	Size   uint64
}
