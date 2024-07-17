// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gptstructs

import (
	"hash/crc32"
	"io"
	"slices"

	"github.com/siderolabs/go-blockdevice/v2/internal/ioutil"
)

// HeaderSignature is the signature of the GPT header.
const HeaderSignature = 0x5452415020494645 // "EFI PART"

// CalculateChecksum calculates the checksum of the header.
func (h Header) CalculateChecksum() uint32 {
	b := slices.Clone(h[:HEADER_SIZE])

	b[16] = 0
	b[17] = 0
	b[18] = 0
	b[19] = 0

	return crc32.ChecksumIEEE(b)
}

// HeaderReader is an interface for reading GPT headers.
type HeaderReader interface {
	io.ReaderAt
	GetSectorSize() uint
}

// ReadHeader reads the GPT header and partition entries.
//
// It does sanity checks on the header and partition entries.
func ReadHeader(r HeaderReader, lba, lastLBA uint64) (*Header, []Entry, error) {
	sectorSize := r.GetSectorSize()
	buf := make([]byte, sectorSize)

	if err := ioutil.ReadFullAt(r, buf, int64(lba)*int64(sectorSize)); err != nil {
		return nil, nil, err
	}

	hdr := Header(buf)

	// verify the header signature
	if hdr.Get_signature() != HeaderSignature {
		return nil, nil, nil
	}

	// sanity check the header size
	headerSize := hdr.Get_header_size()
	if headerSize < HEADER_SIZE || uint(headerSize) > sectorSize {
		return nil, nil, nil
	}

	// verify the header checksum
	if hdr.Get_header_crc32() != hdr.CalculateChecksum() {
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

	if hdr.Get_num_partition_entries() == 0 || hdr.Get_num_partition_entries() > NumEntries {
		return nil, nil, nil
	}

	// read partition entries, verify checksum
	entriesBuffer := make([]byte, hdr.Get_num_partition_entries()*ENTRY_SIZE)

	if err := ioutil.ReadFullAt(r, entriesBuffer, int64(hdr.Get_partition_entries_lba())*int64(sectorSize)); err != nil {
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
