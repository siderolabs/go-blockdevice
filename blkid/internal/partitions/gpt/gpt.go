// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package gpt probes GPT partition tables.
package gpt

import (
	"bytes"
	"hash/crc32"

	"github.com/google/uuid"
	"github.com/siderolabs/go-pointer"
	"golang.org/x/text/encoding/unicode"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/utils"
)

//go:generate go run ../../cstruct/cstruct.go -pkg gpt -struct Header -input header.h -endianness LittleEndian

//go:generate go run ../../cstruct/cstruct.go -pkg gpt -struct Entry -input entry.h -endianness LittleEndian

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

const (
	primaryLBA      = 1
	headerSignature = 0x5452415020494645 // "EFI PART"
)

// Probe runs the further inspection and returns the result if successful.
func (p *Probe) Probe(r probe.Reader, _ magic.Magic) (*probe.Result, error) {
	lastLBA, ok := lastLBA(r)
	if !ok {
		return nil, nil //nolint:nilnil
	}

	// try reading primary header
	hdr, entries, err := readHeader(r, primaryLBA, lastLBA)
	if err != nil {
		return nil, err
	}

	if hdr == nil {
		// try reading backup header
		hdr, entries, err = readHeader(r, lastLBA, lastLBA)
		if err != nil {
			return nil, err
		}
	}

	if hdr == nil {
		// no header, skip
		return nil, nil //nolint:nilnil
	}

	ptUUID, err := uuid.FromBytes(guidToUUID(hdr.Get_disk_guid()))
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

		partUUID, err := uuid.FromBytes(guidToUUID(entry.Get_unique_partition_guid()))
		if err != nil {
			return nil, err
		}

		typeUUID, err := uuid.FromBytes(guidToUUID(entry.Get_partition_type_guid()))
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

func readHeader(r probe.Reader, lba, lastLBA uint64) (*Header, []Entry, error) {
	sectorSize := r.GetSectorSize()
	buf := make([]byte, sectorSize)

	if err := utils.ReadFullAt(r, buf, int64(lba)*int64(sectorSize)); err != nil {
		return nil, nil, err
	}

	hdr := Header(buf)

	// verify the header signature
	if hdr.Get_signature() != headerSignature {
		return nil, nil, nil
	}

	// sanity check the header size
	headerSize := hdr.Get_header_size()
	if headerSize < HEADER_SIZE || uint(headerSize) > sectorSize {
		return nil, nil, nil
	}

	// verify the header checksum
	if hdr.Get_header_crc32() != hdr.calculateChecksum() {
		return nil, nil, nil
	}

	// verify LBA
	if hdr.Get_my_lba() != lba {
		return nil, nil, nil
	}

	firstUsableLBA := hdr.Get_first_usable_lba()
	lastUsableLBA := hdr.Get_last_usable_lba()

	// verify the usable LBA range
	if lastUsableLBA < firstUsableLBA || firstUsableLBA > lastLBA || lastUsableLBA > lastLBA {
		return nil, nil, nil
	}

	// header should be outside the usable range
	if firstUsableLBA < lba && lba < lastUsableLBA {
		return nil, nil, nil
	}

	// read the partition entries
	if hdr.Get_sizeof_partition_entry() != ENTRY_SIZE {
		return nil, nil, nil
	}

	if hdr.Get_num_partition_entries() == 0 || hdr.Get_num_partition_entries() > 128 {
		return nil, nil, nil
	}

	// read partition entries, verify checksum
	entriesBuffer := make([]byte, hdr.Get_num_partition_entries()*ENTRY_SIZE)

	if err := utils.ReadFullAt(r, entriesBuffer, int64(hdr.Get_partition_entries_lba())*int64(sectorSize)); err != nil {
		return nil, nil, err
	}

	entriesChecksum := crc32.ChecksumIEEE(entriesBuffer)
	if entriesChecksum != hdr.Get_partition_entry_array_crc32() {
		return nil, nil, nil
	}

	entries := make([]Entry, hdr.Get_num_partition_entries())
	for i := range entries {
		entries[i] = Entry(entriesBuffer[i*ENTRY_SIZE : (i+1)*ENTRY_SIZE])
	}

	return &hdr, entries, nil
}
