// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package zfs probes ZFS filesystems.
package zfs

//go:generate go run ../../cstruct/cstruct.go -pkg zfs -struct ZFSUB -input zfs.h -endianness LittleEndian

import (
	"fmt"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
)

// https://github.com/util-linux/util-linux/blob/c0207d354ee47fb56acfa64b03b5b559bb301280/libblkid/src/superblocks/zfs.c
const (
	zfsUberblockCount = 128
	zfsUberblockSize  = 1024
	zfsLabelSize      = 1024
	zfsVdevLabelSize  = 1024 * 256
	zfsStartOffset    = 1024 * 128
	zfsMinUberblocks  = 4 // Number of uberblocks to be found
)

var (
	zfsMagic     = uint64(0x00bab10c)
	zfsMagicSwap = uint64(0x0cb1ba00) // endian-swapped
)

// nullMagic matches always.
var nullMagic = magic.Magic{}

// Probe for the filesystem.
type Probe struct{}

// Magic returns the magic value for the filesystem.
func (p *Probe) Magic() []*magic.Magic {
	return []*magic.Magic{&nullMagic}
}

// Name returns the name of the filesystem.
func (p *Probe) Name() string {
	return "zfs"
}

// Probe runs the further inspection and returns the result if successful.
func (p *Probe) Probe(r probe.Reader, _ magic.Magic) (*probe.Result, error) {
	size := r.GetSize()
	// How many bytes between end of last label and the block dev
	lastLabelOffset := size % zfsVdevLabelSize

	var ub ZFSUB

	found := 0

	for _, labelOffset := range []uint64{
		0,
		zfsStartOffset,
		zfsVdevLabelSize,
		size - 4*zfsStartOffset - lastLabelOffset,
		size - 3*zfsStartOffset - lastLabelOffset,
		size - 2*zfsStartOffset - lastLabelOffset,
		size - zfsStartOffset - lastLabelOffset,
	} {
		labelBuf := make([]byte, zfsVdevLabelSize)
		if _, err := r.ReadAt(labelBuf, int64(labelOffset)); err != nil {
			return nil, err
		}

		for i := range zfsUberblockCount {
			ubOffset := uint64(i) * zfsUberblockSize

			ub = ZFSUB(labelBuf[ubOffset : ubOffset+ZFSUB_SIZE])
			if ub.Get_ub_magic() == zfsMagic || ub.Get_ub_magic() == zfsMagicSwap {
				found++
			} else {
				// Not a UB
				continue
			}
		}

		if found >= zfsMinUberblocks {
			break
		}
	}

	if found < zfsMinUberblocks {
		// Not enough uberblocks
		return nil, nil //nolint:nilnil
	}

	// TODO: find out GUID name from nvlist
	uuidLabel := fmt.Sprintf("%016x", ub.Get_ub_guid_sum())
	res := &probe.Result{
		Label: &uuidLabel,
	}

	return res, nil
}
