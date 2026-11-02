// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package ext probes extfs filesystems.
package ext

//go:generate go run ../../../../internal/cstruct/cstruct.go -pkg ext -struct SuperBlock -input superblock.h -endianness LittleEndian

import (
	"bytes"

	"github.com/google/uuid"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/utils"
)

const sbOffset = 0x400

// Various extfs constants.
//
//nolint:revive
const (
	EXT2_FEATURE_RO_COMPAT_SPARSE_SUPER = 0x0001
	EXT2_FEATURE_RO_COMPAT_LARGE_FILE   = 0x0002
	EXT2_FEATURE_RO_COMPAT_BTREE_DIR    = 0x0004
	EXT2_FEATURE_INCOMPAT_FILETYPE      = 0x0002
	EXT2_FEATURE_INCOMPAT_META_BG       = 0x0010

	EXT3_FEATURE_INCOMPAT_RECOVER     = 0x0004
	EXT3_FEATURE_COMPAT_HAS_JOURNAL   = 0x0004
	EXT3_FEATURE_INCOMPAT_JOURNAL_DEV = 0x0008

	EXT2_FEATURE_RO_COMPAT_SUPP        = EXT2_FEATURE_RO_COMPAT_SPARSE_SUPER | EXT2_FEATURE_RO_COMPAT_LARGE_FILE | EXT2_FEATURE_RO_COMPAT_BTREE_DIR
	EXT2_FEATURE_INCOMPAT_SUPP         = EXT2_FEATURE_INCOMPAT_FILETYPE | EXT2_FEATURE_INCOMPAT_META_BG
	EXT2_FEATURE_INCOMPAT_UNSUPPORTED  = ^uint32(EXT2_FEATURE_INCOMPAT_SUPP)
	EXT2_FEATURE_RO_COMPAT_UNSUPPORTED = ^uint32(EXT2_FEATURE_RO_COMPAT_SUPP)

	EXT3_FEATURE_RO_COMPAT_SUPP        = EXT2_FEATURE_RO_COMPAT_SPARSE_SUPER | EXT2_FEATURE_RO_COMPAT_LARGE_FILE | EXT2_FEATURE_RO_COMPAT_BTREE_DIR
	EXT3_FEATURE_INCOMPAT_SUPP         = EXT2_FEATURE_INCOMPAT_FILETYPE | EXT3_FEATURE_INCOMPAT_RECOVER | EXT2_FEATURE_INCOMPAT_META_BG
	EXT3_FEATURE_INCOMPAT_UNSUPPORTED  = ^uint32(EXT3_FEATURE_INCOMPAT_SUPP)
	EXT3_FEATURE_RO_COMPAT_UNSUPPORTED = ^uint32(EXT3_FEATURE_RO_COMPAT_SUPP)

	EXT4_FEATURE_RO_COMPAT_METADATA_CSUM = 0x0400
)

var extfsMagic = magic.Magic{
	Offset: sbOffset + 0x38,
	Value:  []byte("\123\357"),
}

type probeCommon struct{}

// Magic returns the magic value for the filesystem.
func (p *probeCommon) Magic() []*magic.Magic {
	return []*magic.Magic{&extfsMagic}
}

func (p *probeCommon) readSuperblock(r probe.Reader) (SuperBlock, error) {
	buf := make([]byte, SUPERBLOCK_SIZE)

	if _, err := r.ReadAt(buf, sbOffset); err != nil {
		return nil, err
	}

	sb := SuperBlock(buf)

	if sb.Get_s_feature_ro_compat()&EXT4_FEATURE_RO_COMPAT_METADATA_CSUM > 0 {
		csum := utils.CRC32c(buf[:1020])

		if csum != sb.Get_s_checksum() {
			return nil, nil
		}
	}

	return sb, nil
}

func (p *probeCommon) buildResult(sb SuperBlock) (*probe.Result, error) {
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

		res.Label = new(string(lbl[:idx]))
	}

	return res, nil
}
