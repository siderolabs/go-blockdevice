// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package ext probes extfs filesystems.
package ext

//go:generate go run ../../../../internal/cstruct/cstruct.go -pkg ext -struct SuperBlock -input superblock.h -endianness LittleEndian

import (
	"bytes"

	"github.com/google/uuid"
	"github.com/siderolabs/go-pointer"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/utils"
)

const sbOffset = 0x400

// Various extfs constants.
//
//nolint:stylecheck,revive
const (
	EXT4_FEATURE_RO_COMPAT_METADATA_CSUM = 0x0400
)

var extfsMagic = magic.Magic{
	Offset: sbOffset + 0x38,
	Value:  []byte("\123\357"),
}

// Probe for the filesystem.
type Probe struct{}

// Magic returns the magic value for the filesystem.
func (p *Probe) Magic() []*magic.Magic {
	return []*magic.Magic{&extfsMagic}
}

// Name returns the name of the xfs filesystem.
func (p *Probe) Name() string {
	return "extfs"
}

// Probe runs the further inspection and returns the result if successful.
func (p *Probe) Probe(r probe.Reader, _ magic.Magic) (*probe.Result, error) {
	buf := make([]byte, SUPERBLOCK_SIZE)

	if _, err := r.ReadAt(buf, sbOffset); err != nil {
		return nil, err
	}

	sb := SuperBlock(buf)

	if sb.Get_s_feature_ro_compat()&EXT4_FEATURE_RO_COMPAT_METADATA_CSUM > 0 {
		csum := utils.CRC32c(buf[:1020])

		if csum != sb.Get_s_checksum() {
			return nil, nil //nolint:nilnil
		}
	}

	uuid, err := uuid.FromBytes(sb.Get_s_uuid())
	if err != nil {
		return nil, err
	}

	res := &probe.Result{
		UUID: &uuid,

		BlockSize:           sb.BlockSize(),
		FilesystemBlockSize: sb.BlockSize(),
		ProbedSize:          sb.FilesystemSize(),
	}

	lbl := sb.Get_s_volume_name()
	if lbl[0] != 0 {
		idx := bytes.IndexByte(lbl, 0)
		if idx == -1 {
			idx = len(lbl)
		}

		res.Label = pointer.To(string(lbl[:idx]))
	}

	return res, nil
}
