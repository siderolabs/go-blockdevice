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
//nolint: maligned
type Header struct {
	*lba.Buffer
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

func (h *Header) read() (err error) {
	err = h.DeserializeSignature()
	if err != nil {
		return err
	}

	err = h.DeserializeRevision()
	if err != nil {
		return err
	}

	err = h.DeserializeSize()
	if err != nil {
		return err
	}

	err = h.DeserializeCRC()
	if err != nil {
		return err
	}

	err = h.DeserializeCurrentLBA()
	if err != nil {
		return err
	}

	err = h.DeserializeBackupLBA()
	if err != nil {
		return err
	}

	err = h.DeserializeFirstUsableLBA()
	if err != nil {
		return err
	}

	err = h.DeserializeLastUsableLBA()
	if err != nil {
		return err
	}

	err = h.DeserializeGUUID()
	if err != nil {
		return err
	}

	err = h.DeserializeStartingLBA()
	if err != nil {
		return err
	}

	err = h.DeserializeNumberOfPartitionEntries()
	if err != nil {
		return err
	}

	err = h.DeserializePartitionEntrySize()
	if err != nil {
		return err
	}

	err = h.DeserializePartitionEntriesCRC()
	if err != nil {
		return err
	}

	return nil
}

func (h *Header) write() (err error) {
	p, err := h.Primary()
	if err != nil {
		return err
	}

	err = h.WriteAt(1, 0x00, p)
	if err != nil {
		return err
	}

	s, err := h.Secondary()
	if err != nil {
		return err
	}

	err = h.WriteAt(h.TotalSectors-1, 0x00, s)
	if err != nil {
		return err
	}

	return nil
}

// Primary returns the serialized primary header.
func (h *Header) Primary() (b []byte, err error) {
	err = h.SerializeSignature()
	if err != nil {
		return nil, err
	}

	err = h.SerializeRevision()
	if err != nil {
		return nil, err
	}

	err = h.SerializeSize()
	if err != nil {
		return nil, err
	}

	// Reserved; must be zero.

	data := bytes.Repeat([]byte{0x00}, 4)

	err = h.Write(data, 0x14)
	if err != nil {
		return nil, err
	}

	err = h.SerializeCurrentLBA()
	if err != nil {
		return nil, err
	}

	err = h.SerializeBackupLBA()
	if err != nil {
		return nil, err
	}

	err = h.SerializeFirstUsableLBA()
	if err != nil {
		return nil, err
	}

	err = h.SerializeLastUsableLBA()
	if err != nil {
		return nil, err
	}

	err = h.SerializeGUUID()
	if err != nil {
		return nil, err
	}

	err = h.SerializeStartingLBA()
	if err != nil {
		return nil, err
	}

	err = h.SerializeNumberOfPartitionEntries()
	if err != nil {
		return nil, err
	}

	err = h.SerializePartitionEntrySize()
	if err != nil {
		return nil, err
	}

	err = h.SerializePartitionEntriesCRC()
	if err != nil {
		return nil, err
	}

	// Reserved; must be zeroes for the rest of the block (420 bytes for a sector size of 512 bytes; but can be more with larger sector sizes)

	data = bytes.Repeat([]byte{0x00}, int(h.LogicalBlockSize)-HeaderSize)

	err = h.Write(data, 0x5C)
	if err != nil {
		return nil, err
	}

	err = h.SerializeCRC()
	if err != nil {
		return nil, err
	}

	return h.Bytes(), nil
}

// Secondary returns the serialized secondary header.
func (h *Header) Secondary() (b []byte, err error) {
	b = make([]byte, len(h.Bytes()))

	copy(b, h.Bytes())

	buf := lba.NewBuffer(h.LBA, b)

	// Current LBA (location of this header copy).

	data := make([]byte, 8)

	binary.LittleEndian.PutUint64(data, h.BackupLBA)

	err = buf.Write(data, 0x18)
	if err != nil {
		return nil, err
	}

	// Backup LBA (location of the other header copy).

	data = make([]byte, 8)

	binary.LittleEndian.PutUint64(data, 1)

	err = buf.Write(data, 0x20)
	if err != nil {
		return nil, err
	}

	// CRC32 of header (offset +0 up to header size) in little endian, with this field zeroed during calculation.

	data = make([]byte, 4)

	// Zero the CRC field during the calculation.
	n := copy(b[16:20], bytes.Repeat([]byte{0x00}, 4))
	if n != 4 {
		return nil, fmt.Errorf("expected to copy 4 bytes into header, copied %d", n)
	}

	crc := crc32.ChecksumIEEE(b[0:HeaderSize])

	binary.LittleEndian.PutUint32(data, crc)

	err = buf.Write(data, 0x10)
	if err != nil {
		return nil, err
	}

	return b, nil
}

// DeserializeSignature desirializes the signature ("EFI PART", 45h 46h 49h 20h 50h 41h 52h 54h or 0x5452415020494645ULL on little-endian machines).
func (h *Header) DeserializeSignature() (err error) {
	data, err := h.ReadAt(1, 0x00, 8)
	if err != nil {
		return fmt.Errorf("signature read: %w", err)
	}

	signature := string(data)

	if signature != MagicEFIPart {
		return fmt.Errorf("expected signature of %q, got %q", MagicEFIPart, signature)
	}

	h.Signature = signature

	return nil
}

// SerializeSignature serializes the signature ("EFI PART", 45h 46h 49h 20h 50h 41h 52h 54h or 0x5452415020494645ULL on little-endian machines).
func (h *Header) SerializeSignature() (err error) {
	err = h.Write([]byte(MagicEFIPart), 0x00)
	if err != nil {
		return err
	}

	return nil
}

// DeserializeRevision deserializes the revision (for GPT version 1.0 (through at least UEFI version 2.7 (May 2017)), the value is 00h 00h 01h 00h).
func (h *Header) DeserializeRevision() (err error) {
	data, err := h.ReadAt(1, 0x08, 4)
	if err != nil {
		return fmt.Errorf("revision read: %w", err)
	}

	h.Revision = binary.LittleEndian.Uint32(data)

	return nil
}

// SerializeRevision serializes the revision (for GPT version 1.0 (through at least UEFI version 2.7 (May 2017)), the value is 00h 00h 01h 00h).
func (h *Header) SerializeRevision() (err error) {
	data := make([]byte, 4)

	binary.LittleEndian.PutUint32(data, h.Revision)

	err = h.Write(data, 0x08)
	if err != nil {
		return err
	}

	return nil
}

// DeserializeSize deserializes the header size in little endian (in bytes, usually 5Ch 00h 00h 00h or 92 bytes).
func (h *Header) DeserializeSize() (err error) {
	data, err := h.ReadAt(1, 0x0C, 4)
	if err != nil {
		return fmt.Errorf("header size read: %w", err)
	}

	h.Size = binary.LittleEndian.Uint32(data)

	if h.Size < HeaderSize {
		return fmt.Errorf("header size too small: %d", h.Size)
	}

	return nil
}

// SerializeSize serializes the header size in little endian (in bytes, usually 5Ch 00h 00h 00h or 92 bytes).
func (h *Header) SerializeSize() (err error) {
	data := make([]byte, 4)

	binary.LittleEndian.PutUint32(data, h.Size)

	err = h.Write(data, 0x0C)
	if err != nil {
		return err
	}

	return nil
}

// DeserializeCRC deserializes the CRC32 of header (offset +0 up to header size) in little endian, with this field zeroed during calculation.
func (h *Header) DeserializeCRC() (err error) {
	data, err := h.ReadAt(1, 0x10, 4)
	if err != nil {
		return fmt.Errorf("header CRC32 read: %w", err)
	}

	crc := binary.LittleEndian.Uint32(data)

	hdr, err := h.ReadAt(1, 0x00, int64(h.Size))
	if err != nil {
		return fmt.Errorf("header read: %w", err)
	}

	// Zero the CRC field during the calculation.
	n := copy(hdr[16:20], []byte{0x00, 0x00, 0x00, 0x00})
	if n != 4 {
		return fmt.Errorf("expected to copy 4 bytes into header, copied %d", n)
	}

	checksum := crc32.ChecksumIEEE(hdr)

	if crc != checksum {
		return fmt.Errorf("expected header checksum of %v, got %v", checksum, crc)
	}

	h.Checksum = crc

	return nil
}

// SerializeCRC serializes the CRC32 of header (offset +0 up to header size) in little endian, with this field zeroed during calculation.
func (h *Header) SerializeCRC() (err error) {
	data := make([]byte, 4)

	// Zero the CRC field during the calculation.
	n := copy(h.Bytes()[16:20], bytes.Repeat([]byte{0x00}, 4))
	if n != 4 {
		return fmt.Errorf("expected to copy 4 bytes into header, copied %d", n)
	}

	crc := crc32.ChecksumIEEE(h.Bytes()[0:HeaderSize])

	binary.LittleEndian.PutUint32(data, crc)

	err = h.Write(data, 0x10)
	if err != nil {
		return err
	}

	h.Checksum = crc

	return nil
}

// DeserializeCurrentLBA deserializes the current LBA (location of this header copy).
func (h *Header) DeserializeCurrentLBA() (err error) {
	data, err := h.ReadAt(1, 0x18, 8)
	if err != nil {
		return fmt.Errorf("current LBA read: %w", err)
	}

	h.CurrentLBA = binary.LittleEndian.Uint64(data)

	return nil
}

// SerializeCurrentLBA serializes the current LBA (location of this header copy).
func (h *Header) SerializeCurrentLBA() (err error) {
	data := make([]byte, 8)

	binary.LittleEndian.PutUint64(data, h.CurrentLBA)

	err = h.Write(data, 0x18)
	if err != nil {
		return err
	}

	return nil
}

// DeserializeBackupLBA deserializes the backup LBA (location of the other header copy).
func (h *Header) DeserializeBackupLBA() (err error) {
	data, err := h.ReadAt(1, 0x20, 8)
	if err != nil {
		return fmt.Errorf("backup LBA read: %w", err)
	}

	h.BackupLBA = binary.LittleEndian.Uint64(data)

	return nil
}

// SerializeBackupLBA serializes the backup LBA (location of the other header copy).
func (h *Header) SerializeBackupLBA() (err error) {
	data := make([]byte, 8)

	binary.LittleEndian.PutUint64(data, h.BackupLBA)

	err = h.Write(data, 0x20)
	if err != nil {
		return err
	}

	return nil
}

// DeserializeFirstUsableLBA deserializes the first usable LBA for partitions (primary partition table last LBA + 1).
func (h *Header) DeserializeFirstUsableLBA() (err error) {
	data, err := h.ReadAt(1, 0x28, 8)
	if err != nil {
		return fmt.Errorf("first usable LBA read: %w", err)
	}

	h.FirstUsableLBA = binary.LittleEndian.Uint64(data)

	return nil
}

// SerializeFirstUsableLBA serializes the first usable LBA for partitions (primary partition table last LBA + 1).
func (h *Header) SerializeFirstUsableLBA() (err error) {
	data := make([]byte, 8)

	binary.LittleEndian.PutUint64(data, h.FirstUsableLBA)

	err = h.Write(data, 0x28)
	if err != nil {
		return err
	}

	return nil
}

// DeserializeLastUsableLBA deserializes the last usable LBA (secondary partition table first LBA − 1).
func (h *Header) DeserializeLastUsableLBA() (err error) {
	data, err := h.ReadAt(1, 0x30, 8)
	if err != nil {
		return fmt.Errorf("last usable LBA read: %w", err)
	}

	h.LastUsableLBA = binary.LittleEndian.Uint64(data)

	return nil
}

// SerializeLastUsableLBA serializes the last usable LBA (secondary partition table first LBA − 1).
func (h *Header) SerializeLastUsableLBA() (err error) {
	data := make([]byte, 8)

	binary.LittleEndian.PutUint64(data, h.LastUsableLBA)

	err = h.Write(data, 0x30)
	if err != nil {
		return err
	}

	return nil
}

// DeserializeGUUID deserializes the disk GUID in mixed endian.
func (h *Header) DeserializeGUUID() (err error) {
	data, err := h.ReadAt(1, 0x38, 16)
	if err != nil {
		return fmt.Errorf("guuid read: %w", err)
	}

	u, err := endianness.FromMiddleEndian(data)
	if err != nil {
		return err
	}

	guid, err := uuid.FromBytes(u)
	if err != nil {
		return fmt.Errorf("invalid GUUID: %w", err)
	}

	h.GUUID = guid

	return nil
}

// SerializeGUUID serializes the disk GUID in mixed endian.
func (h *Header) SerializeGUUID() (err error) {
	b, err := h.GUUID.MarshalBinary()
	if err != nil {
		return err
	}

	data, err := endianness.ToMiddleEndian(b)
	if err != nil {
		return err
	}

	err = h.Write(data, 0x38)
	if err != nil {
		return err
	}

	return nil
}

// DeserializeStartingLBA deserializes the starting LBA of array of partition entries (always 2 in primary copy).
func (h *Header) DeserializeStartingLBA() (err error) {
	data, err := h.ReadAt(1, 0x48, 8)
	if err != nil {
		return fmt.Errorf("starting LBA of entries read: %w", err)
	}

	h.EntriesLBA = binary.LittleEndian.Uint64(data)

	return nil
}

// SerializeStartingLBA serializes the starting LBA of array of partition entries (always 2 in primary copy).
func (h *Header) SerializeStartingLBA() (err error) {
	data := make([]byte, 8)

	binary.LittleEndian.PutUint64(data, h.EntriesLBA)

	err = h.Write(data, 0x48)
	if err != nil {
		return err
	}

	return nil
}

// DeserializeNumberOfPartitionEntries deserializes the number of partition entries in array.
func (h *Header) DeserializeNumberOfPartitionEntries() (err error) {
	data, err := h.ReadAt(1, 0x50, 4)
	if err != nil {
		return fmt.Errorf("number of partitin entries read: %w", err)
	}

	h.NumberOfPartitionEntries = binary.LittleEndian.Uint32(data)

	return nil
}

// SerializeNumberOfPartitionEntries serializes the number of partition entries in array.
func (h *Header) SerializeNumberOfPartitionEntries() (err error) {
	data := make([]byte, 4)

	binary.LittleEndian.PutUint32(data, h.NumberOfPartitionEntries)

	err = h.Write(data, 0x50)
	if err != nil {
		return err
	}

	return nil
}

// DeserializePartitionEntrySize deserializes the size of a single partition entry (usually 80h or 128).
func (h *Header) DeserializePartitionEntrySize() (err error) {
	data, err := h.ReadAt(1, 0x54, 4)
	if err != nil {
		return fmt.Errorf("last usable LBA read: %w", err)
	}

	h.PartitionEntrySize = binary.LittleEndian.Uint32(data)

	return nil
}

// SerializePartitionEntrySize serializes the size of a single partition entry (usually 80h or 128).
func (h *Header) SerializePartitionEntrySize() (err error) {
	data := make([]byte, 4)

	binary.LittleEndian.PutUint32(data, h.PartitionEntrySize)

	err = h.Write(data, 0x54)
	if err != nil {
		return err
	}

	return nil
}

// DeserializePartitionEntriesCRC deserializes the CRC32 of partition entries array in little endian.
func (h *Header) DeserializePartitionEntriesCRC() (err error) {
	data, err := h.ReadAt(1, 0x58, 4)
	if err != nil {
		return fmt.Errorf("partition entries array CRC32 read: %w", err)
	}

	crc := binary.LittleEndian.Uint32(data)

	entries, err := h.ReadAt(int64(h.EntriesLBA), 0x00, int64(h.NumberOfPartitionEntries*h.PartitionEntrySize))
	if err != nil {
		return fmt.Errorf("entries read: %w", err)
	}

	checksum := crc32.ChecksumIEEE(entries)

	if crc != checksum {
		return fmt.Errorf("expected partition checksum of %v, got %v", checksum, crc)
	}

	h.PartitionEntriesChecksum = crc

	return nil
}

// SerializePartitionEntriesCRC serializes the CRC32 of partition entries array in little endian.
func (h *Header) SerializePartitionEntriesCRC() (err error) {
	data := make([]byte, 4)

	entries, err := h.ReadAt(int64(h.EntriesLBA), 0x00, int64(h.NumberOfPartitionEntries*h.PartitionEntrySize))
	if err != nil {
		return err
	}

	crc := crc32.ChecksumIEEE(entries)

	binary.LittleEndian.PutUint32(data, crc)

	err = h.Write(data, 0x58)
	if err != nil {
		return err
	}

	h.PartitionEntriesChecksum = crc

	return nil
}
