// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package gpt probes GPT partition tables.
package gpt

import (
	"bytes"

	"github.com/google/uuid"
	"github.com/siderolabs/go-pointer"
	"golang.org/x/text/encoding/unicode"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
	"github.com/siderolabs/go-blockdevice/v2/internal/gptstructs"
	"github.com/siderolabs/go-blockdevice/v2/internal/gptutil"
)

// nullMagic matches always.
var nullMagic = magic.Magic{}

// Probe for the partition table.
type Probe struct{}

// Magic returns the magic value for the partition table.
func (p *Probe) Magic() []*magic.Magic {
	return []*magic.Magic{&nullMagic}
}

// Name returns the name of the partition table.
func (p *Probe) Name() string {
	return "gpt"
}

const primaryLBA = 1

// Probe runs the further inspection and returns the result if successful.
func (p *Probe) Probe(r probe.Reader, _ magic.Magic) (*probe.Result, error) {
	lastLBA, ok := gptutil.LastLBA(r)
	if !ok {
		return nil, nil //nolint:nilnil
	}

	// try reading primary header
	hdr, entries, err := gptstructs.ReadHeader(r, primaryLBA, lastLBA)
	if err != nil {
		return nil, err
	}

	if hdr == nil {
		// try reading backup header
		hdr, entries, err = gptstructs.ReadHeader(r, lastLBA, lastLBA)
		if err != nil {
			return nil, err
		}
	}

	if hdr == nil {
		// no header, skip
		return nil, nil //nolint:nilnil
	}

	ptUUID, err := uuid.FromBytes(gptutil.GUIDToUUID(hdr.Get_disk_guid()))
	if err != nil {
		return nil, err
	}

	sectorSize := r.GetSectorSize()

	result := &probe.Result{
		UUID: &ptUUID,

		BlockSize:  uint32(sectorSize),
		ProbedSize: uint64(sectorSize) * (hdr.Get_last_usable_lba() - hdr.Get_first_usable_lba() + 1),
	}

	partIdx := uint(1)
	firstUsableLBA := hdr.Get_first_usable_lba()
	lastUsableLBA := hdr.Get_last_usable_lba()

	zeroGUID := make([]byte, 16)
	utf16 := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)

	for _, entry := range entries {
		offset := entry.Get_starting_lba() * uint64(sectorSize)
		size := (entry.Get_ending_lba() - entry.Get_starting_lba() + 1) * uint64(sectorSize)

		if entry.Get_starting_lba() < firstUsableLBA || entry.Get_ending_lba() > lastUsableLBA {
			partIdx++

			continue
		}

		// skip zero GUIDs
		if bytes.Equal(entry.Get_partition_type_guid(), zeroGUID) {
			partIdx++

			continue
		}

		partUUID, err := uuid.FromBytes(gptutil.GUIDToUUID(entry.Get_unique_partition_guid()))
		if err != nil {
			return nil, err
		}

		typeUUID, err := uuid.FromBytes(gptutil.GUIDToUUID(entry.Get_partition_type_guid()))
		if err != nil {
			return nil, err
		}

		name, err := utf16.NewDecoder().Bytes(entry.Get_partition_name())
		if err != nil {
			return nil, err
		}

		name = bytes.TrimRight(name, "\x00")

		result.Parts = append(result.Parts, probe.Partition{
			UUID:     &partUUID,
			TypeUUID: &typeUUID,
			Label:    pointer.To(string(name)),

			Index: partIdx,

			Offset: offset,
			Size:   size,
		})

		partIdx++
	}

	return result, nil
}
