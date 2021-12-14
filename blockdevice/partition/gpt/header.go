// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gpt

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"

	"github.com/google/uuid"

	"github.com/talos-systems/go-blockdevice/blockdevice/endianness"
	"github.com/talos-systems/go-blockdevice/blockdevice/lba"
)

// Header represents the GPT header.
//
//nolint:maligned,govet
type Header struct {
	*lba.LBA

	Signature                string
	Revision                 uint32
	Size                     uint32
	Checksum                 uint32
	CurrentLBA               uint64
	BackupLBA                uint64
	FirstUsableLBA           uint64
	LastUsableLBA            uint64
	GUUID                    uuid.UUID
	EntriesLBA               uint64
	NumberOfPartitionEntries uint32
	PartitionEntrySize       uint32
	PartitionEntriesChecksum uint32
}

func (h *Header) verifySignature() (err error) {
	b, err := h.LBA.ReadAt(1, 0, 8)
	if err != nil {
		return err
	}

	h.Signature = string(b[:8])

	if h.Signature != MagicEFIPart {
		return fmt.Errorf("expected signature of %q, got %q", MagicEFIPart, h.Signature)
	}

	return nil
}

func (h *Header) read() (err error) {
	b, err := h.LBA.ReadAt(1, 0, HeaderSize)
	if err != nil {
		return err
	}

	h.Signature = string(b[:8])

	if h.Signature != MagicEFIPart {
		return fmt.Errorf("expected signature of %q, got %q", MagicEFIPart, h.Signature)
	}

	h.Revision = binary.LittleEndian.Uint32(b[8:12])
	h.Size = binary.LittleEndian.Uint32(b[12:16])

	if h.Size < HeaderSize {
		return fmt.Errorf("header size too small: %d", h.Size)
	}

	if h.Size > uint32(h.LBA.LogicalBlockSize) {
		return fmt.Errorf("header size too big: %d", h.Size)
	}

	if h.Size > HeaderSize {
		// re-read the header to include all data
		b, err = h.LBA.ReadAt(1, 0, HeaderSize)
		if err != nil {
			return err
		}
	}

	h.Checksum = binary.LittleEndian.Uint32(b[16:20])

	// zero out checksum in the header before calculating CRC32
	copy(b[16:20], []byte{0x00, 0x00, 0x00, 0x00})

	checksum := crc32.ChecksumIEEE(b)

	if h.Checksum != checksum {
		return fmt.Errorf("expected header checksum of %v, got %v", checksum, h.Checksum)
	}

	h.CurrentLBA = binary.LittleEndian.Uint64(b[24:32])
	h.BackupLBA = binary.LittleEndian.Uint64(b[32:40])
	h.FirstUsableLBA = binary.LittleEndian.Uint64(b[40:48])
	h.LastUsableLBA = binary.LittleEndian.Uint64(b[48:56])

	h.GUUID, err = uuid.FromBytes(endianness.FromMiddleEndian(b[56:72]))
	if err != nil {
		return fmt.Errorf("invalid GUUID: %w", err)
	}

	h.EntriesLBA = binary.LittleEndian.Uint64(b[72:80])
	h.NumberOfPartitionEntries = binary.LittleEndian.Uint32(b[80:84])
	h.PartitionEntrySize = binary.LittleEndian.Uint32(b[84:88])

	h.PartitionEntriesChecksum = binary.LittleEndian.Uint32(b[88:92])

	return nil
}

func (h *Header) write() (err error) {
	p, err := h.primary()
	if err != nil {
		return err
	}

	err = h.WriteAt(1, 0x00, p)
	if err != nil {
		return err
	}

	h.secondaryFromPrimary(p)

	err = h.WriteAt(h.TotalSectors-1, 0x00, p)
	if err != nil {
		return err
	}

	return nil
}

// primary returns the serialized primary header.
func (h *Header) primary() (b []byte, err error) {
	b = make([]byte, h.LBA.LogicalBlockSize)

	copy(b[:8], MagicEFIPart)

	binary.LittleEndian.PutUint32(b[8:12], h.Revision)
	binary.LittleEndian.PutUint32(b[12:16], h.Size)

	// CRC is filled last 16:20

	// 4 reserved bytes 20:24

	binary.LittleEndian.PutUint64(b[24:32], h.CurrentLBA)
	binary.LittleEndian.PutUint64(b[32:40], h.BackupLBA)
	binary.LittleEndian.PutUint64(b[40:48], h.FirstUsableLBA)
	binary.LittleEndian.PutUint64(b[48:56], h.LastUsableLBA)

	uuid, err := h.GUUID.MarshalBinary()
	if err != nil {
		return nil, err
	}

	copy(b[56:72], endianness.ToMiddleEndian(uuid))

	binary.LittleEndian.PutUint64(b[72:80], h.EntriesLBA)
	binary.LittleEndian.PutUint32(b[80:84], h.NumberOfPartitionEntries)
	binary.LittleEndian.PutUint32(b[84:88], h.PartitionEntrySize)
	binary.LittleEndian.PutUint32(b[88:92], h.PartitionEntriesChecksum)
	binary.LittleEndian.PutUint32(b[16:20], crc32.ChecksumIEEE(b[0:h.Size]))

	return b, nil
}

// secondaryFromPrimary modifies in-place primary header to be secondary header.
func (h *Header) secondaryFromPrimary(b []byte) {
	// swap current and backup LBAs
	binary.LittleEndian.PutUint64(b[24:32], h.BackupLBA)
	binary.LittleEndian.PutUint64(b[32:40], h.CurrentLBA)

	// CRC32 of header (offset +0 up to header size) in little endian, with this field zeroed during calculation.
	copy(b[16:20], bytes.Repeat([]byte{0x00}, 4))

	binary.LittleEndian.PutUint32(b[16:20], crc32.ChecksumIEEE(b[0:h.Size]))
}
