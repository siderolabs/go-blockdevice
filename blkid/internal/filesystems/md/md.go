// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package md probes Linux Software RAID (md) member devices with v1 superblocks.
package md

//go:generate go run ../../../../internal/cstruct/cstruct.go -pkg md -struct MDSuperblock -input md_header.h -endianness LittleEndian

import (
	"bytes"

	"github.com/google/uuid"
	"github.com/siderolabs/go-pointer"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
)

const (
	superMagic   = 0xa92b4efc // MD_SB_MAGIC
	superVersion = 1          // major_version

	sectorSize = 512

	// Superblock locations in sectors, per mdadm super1.c.
	sb11Offset        = 0  // 1.1: at the start of the device
	sb12Offset        = 8  // 1.2: 4KiB into the device
	sb10TailOffset    = 16 // 1.0: 8KiB from the end of the device
	sb10TailAlignment = 8  // 1.0: then rounded down to a 4KiB boundary
)

// nullMagic matches always: the 1.0 superblock lives at the device end, so we
// can't express detection as a fixed start-offset magic (see gpt/zfs probers).
var nullMagic = magic.Magic{}

// Probe for the md member device.
type Probe struct{}

// Magic returns the magic value for the md member device.
func (p *Probe) Magic() []*magic.Magic {
	return []*magic.Magic{&nullMagic}
}

// Name returns the name of the volume manager.
func (p *Probe) Name() string {
	return "linux_raid_member"
}

// Probe runs the further inspection and returns the result if successful.
func (p *Probe) Probe(r probe.Reader, _ magic.Magic) (*probe.Result, error) {
	sectors := r.GetSize() / sectorSize
	if sectors < 32 {
		return nil, nil //nolint:nilnil
	}

	candidateSectors := []uint64{sb11Offset, sb12Offset}

	if sectors > sb10TailOffset {
		tail := sectors - sb10TailOffset
		candidateSectors = append(candidateSectors, tail-tail%sb10TailAlignment)
	}

	buf := make([]byte, MDSUPERBLOCK_SIZE)

	for _, sbSectors := range candidateSectors {
		if _, err := r.ReadAt(buf, int64(sbSectors*sectorSize)); err != nil {
			return nil, err
		}

		sb := MDSuperblock(buf)

		if sb.Get_magic() != superMagic || sb.Get_major_version() != superVersion {
			continue
		}

		// super_offset must point back at where we found the superblock.
		if sb.Get_super_offset() != sbSectors {
			continue
		}

		res := &probe.Result{
			ExtraSignatures: []probe.SignatureRange{
				{Offset: sbSectors * sectorSize, Size: 4},
			},
		}

		if u, err := uuid.FromBytes(sb.Get_set_uuid()); err == nil {
			res.UUID = &u
		}

		if name := sb.Get_set_name(); name[0] != 0 {
			idx := bytes.IndexByte(name, 0)
			if idx < 0 {
				idx = len(name)
			}

			res.Label = pointer.To(string(name[:idx]))
		}

		return res, nil
	}

	return nil, nil //nolint:nilnil
}
