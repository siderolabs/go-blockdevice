// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package btrfs probes btrfs filesystems.
package btrfs

//go:generate go run ../../../../internal/cstruct/cstruct.go -pkg btrfs -struct SuperBlock -input superblock.h -endianness LittleEndian

import (
	"bytes"

	"github.com/google/uuid"
	"github.com/siderolabs/go-pointer"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
)

// SuperBlockOffset is the offset of the primary btrfs superblock from the start of the device.
const SuperBlockOffset = 65536 // 64 KiB

// btrfsMagicOffset is the offset of the magic field within the superblock.
const btrfsMagicOffset = 64

var btrfsMagic = magic.Magic{
	Offset: SuperBlockOffset + btrfsMagicOffset,
	Value:  []byte{0x5f, 0x42, 0x48, 0x52, 0x66, 0x53, 0x5f, 0x4d}, // "_BHRfS_M"
}

// Probe for the filesystem.
type Probe struct{}

// Magic returns the magic value for the filesystem.
func (p *Probe) Magic() []*magic.Magic {
	return []*magic.Magic{&btrfsMagic}
}

// Name returns the name of the filesystem.
func (p *Probe) Name() string {
	return "btrfs"
}

// Probe runs the further inspection and returns the result if successful.
func (p *Probe) Probe(r probe.Reader, _ magic.Magic) (*probe.Result, error) {
	buf := make([]byte, SUPERBLOCK_SIZE)

	if _, err := r.ReadAt(buf, SuperBlockOffset); err != nil {
		return nil, err
	}

	sb := SuperBlock(buf)

	id, err := uuid.FromBytes(sb.Get_fsid())
	if err != nil {
		return nil, err
	}

	res := &probe.Result{
		UUID:                &id,
		BlockSize:           sb.Get_sectorsize(),
		FilesystemBlockSize: sb.Get_nodesize(),
		ProbedSize:          sb.Get_total_bytes(),
	}

	lbl := sb.Get_label()
	if lbl[0] != 0 {
		idx := bytes.IndexByte(lbl, 0)
		if idx == -1 {
			idx = len(lbl)
		}

		res.Label = pointer.To(string(lbl[:idx]))
	}

	return res, nil
}
