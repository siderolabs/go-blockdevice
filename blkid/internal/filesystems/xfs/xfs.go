// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package xfs probes XFS filesystems.
package xfs

//go:generate go run ../../cstruct/cstruct.go -pkg xfs -struct SuperBlock -input superblock.h -endianness BigEndian

import (
	"bytes"

	"github.com/google/uuid"
	"github.com/siderolabs/go-pointer"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
)

var xfsMagic = magic.Magic{
	Offset: 0,
	Value:  []byte{0x58, 0x46, 0x53, 0x42},
}

// Probe for the filesystem.
type Probe struct{}

// Magic returns the magic value for the filesystem.
func (p *Probe) Magic() []*magic.Magic {
	return []*magic.Magic{&xfsMagic}
}

// Name returns the name of the filesystem.
func (p *Probe) Name() string {
	return "xfs"
}

// Probe runs the further inspection and returns the result if successful.
func (p *Probe) Probe(r probe.Reader) (*probe.Result, error) {
	buf := make([]byte, SUPERBLOCK_SIZE)

	if _, err := r.ReadAt(buf, 0); err != nil {
		return nil, err
	}

	sb := SuperBlock(buf)
	if !sb.Valid() {
		return nil, nil //nolint:nilnil
	}

	uuid, err := uuid.FromBytes(sb.Get_sb_uuid())
	if err != nil {
		return nil, err
	}

	res := &probe.Result{
		UUID: &uuid,

		BlockSize:           uint32(sb.Get_sb_sectsize()),
		FilesystemBlockSize: sb.Get_sb_blocksize(),
		ProbedSize:          sb.FilesystemSize(),
	}

	lbl := sb.Get_sb_fname()
	if lbl[0] != 0 {
		idx := bytes.IndexByte(lbl, 0)
		if idx == -1 {
			idx = len(lbl)
		}

		res.Label = pointer.To(string(lbl[:idx]))
	}

	return res, nil
}
