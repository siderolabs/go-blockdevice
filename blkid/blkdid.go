// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package blkid provides information about blockdevice filesystem types and IDs.
package blkid

import (
	"github.com/google/uuid"

	"github.com/siderolabs/go-blockdevice/v2/block"
)

// Info represents the result of the probe.
type Info struct { //nolint:govet
	// Link to the block device, only if the probed file is a blockdevice.
	BlockDevice *block.Device

	// Overall size of the probed device (in bytes).
	Size uint64

	// Optimal I/O size for the device (in bytes).
	IOSize uint64

	// TODO: API might be different.
	Name  string
	UUID  *uuid.UUID
	Label *string

	BlockSize           uint32
	FilesystemBlockSize uint32
	FilesystemSize      uint64
}
