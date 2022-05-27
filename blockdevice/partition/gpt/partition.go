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
	"golang.org/x/text/encoding/unicode"

	"github.com/talos-systems/go-blockdevice/blockdevice/endianness"
	"github.com/talos-systems/go-blockdevice/blockdevice/filesystem"
	"github.com/talos-systems/go-blockdevice/blockdevice/util"
)

// Partitions represents the GPT partitions array.
//
//nolint:govet
type Partitions struct {
	h       *Header
	p       []*Partition
	devname string
}

// Partition represents a GPT partition.
//
//nolint:govet
type Partition struct {
	Type       uuid.UUID
	ID         uuid.UUID
	FirstLBA   uint64
	LastLBA    uint64
	Attributes uint64
	Name       string

	Number int32

	devname string
}

// Items returns the partitions.
func (p *Partitions) Items() []*Partition {
	return p.p
}

// FindByName finds partition by label.
func (p *Partitions) FindByName(name string) *Partition {
	for _, part := range p.Items() {
		if part.Name == name {
			return part
		}
	}

	return nil
}

// Length returns the partition's length in LBA.
func (p *Partition) Length() uint64 {
	// in GPT, LastLBA is inclusive, so +1
	return p.LastLBA - p.FirstLBA + 1
}

func (p *Partitions) read() error {
	partitions := make([]*Partition, 0, p.h.NumberOfPartitionEntries)

	checksummer := crc32.NewIEEE()

	for i := uint32(0); i < p.h.NumberOfPartitionEntries; i++ {
		offset := i * p.h.PartitionEntrySize

		data, err := p.h.ReadAt(int64(p.h.EntriesLBA), int64(offset), int64(p.h.PartitionEntrySize))
		if err != nil {
			return fmt.Errorf("partition read: %w", err)
		}

		checksummer.Write(data)

		part := &Partition{Number: int32(i + 1), devname: p.devname}

		err = part.deserialize(data)
		if err != nil {
			return err
		}

		// The first LBA of the partition cannot start before the first usable
		// LBA specified in the header.
		if part.FirstLBA >= p.h.FirstUsableLBA {
			partitions = append(partitions, part)
		}
	}

	if checksummer.Sum32() != p.h.PartitionEntriesChecksum {
		return fmt.Errorf("expected partition checksum of %v, got %v", p.h.PartitionEntriesChecksum, checksummer.Sum32())
	}

	p.p = partitions

	return nil
}

func (p *Partitions) write() ([]byte, error) {
	data := make([]byte, p.h.NumberOfPartitionEntries*p.h.PartitionEntrySize)

	for i, part := range p.p {
		if part == nil {
			continue
		}

		if err := part.serialize(data[i*int(p.h.PartitionEntrySize):]); err != nil {
			return nil, err
		}
	}

	p.h.PartitionEntriesChecksum = crc32.ChecksumIEEE(data)

	return data, nil
}

var utf16 = unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)

func (p *Partition) deserialize(b []byte) error {
	var err error

	p.Type, err = uuid.FromBytes(endianness.FromMiddleEndian(b[:16]))
	if err != nil {
		return fmt.Errorf("invalid GUUID: %w", err)
	}

	// TODO: Provide a method for getting the human readable name of the type.
	// See https://en.wikipedia.org/wiki/GUID_Partition_Table.

	p.ID, err = uuid.FromBytes(endianness.FromMiddleEndian(b[16:32]))
	if err != nil {
		return fmt.Errorf("invalid GUUID: %w", err)
	}

	p.FirstLBA = binary.LittleEndian.Uint64(b[32:40])
	p.LastLBA = binary.LittleEndian.Uint64(b[40:48])
	p.Attributes = binary.LittleEndian.Uint64(b[48:56])

	decoded, err := utf16.NewDecoder().Bytes(b[56:128])
	if err != nil {
		return err
	}

	p.Name = string(bytes.Trim(decoded, "\x00"))

	return nil
}

func (p *Partition) serialize(b []byte) error {
	uuid, err := p.Type.MarshalBinary()
	if err != nil {
		return err
	}

	copy(b[:16], endianness.ToMiddleEndian(uuid))

	uuid, err = p.ID.MarshalBinary()
	if err != nil {
		return err
	}

	copy(b[16:32], endianness.ToMiddleEndian(uuid))

	binary.LittleEndian.PutUint64(b[32:40], p.FirstLBA)
	binary.LittleEndian.PutUint64(b[40:48], p.LastLBA)
	binary.LittleEndian.PutUint64(b[48:56], p.Attributes)

	name, err := utf16.NewEncoder().Bytes([]byte(p.Name))
	if err != nil {
		return err
	}

	if len(name) > 72 {
		return fmt.Errorf("partition name is too long: %q", p.Name)
	}

	copy(b[56:128], name)

	return nil
}

// SuperBlock read partition superblock.
// if partition is encrypted it will always return superblock of the physical partition,
// instead of a mapped device partition.
func (p *Partition) SuperBlock() (filesystem.SuperBlocker, error) { //nolint:ireturn
	path, err := p.Path()
	if err != nil {
		return nil, err
	}

	superblock, err := filesystem.Probe(path)
	if err != nil {
		return nil, err
	}

	return superblock, nil
}

// Filesystem returns partition filesystem type.
// if partition is encrypted it will return /dev/mapper parition filesystem kind.
func (p *Partition) Filesystem() (string, error) {
	sb, err := p.SuperBlock()
	if err != nil {
		return "", err
	}

	if sb == nil {
		return filesystem.Unknown, nil
	}

	if sb.Encrypted() {
		path, err := p.Path()
		if err != nil {
			return "", err
		}

		sb, err = filesystem.Probe(path)
		if err != nil {
			return "", err
		}

		if sb != nil {
			return sb.Type(), nil
		}

		return filesystem.Unknown, nil
	}

	return sb.Type(), nil
}

// Encrypted checks if partition is encrypted.
func (p *Partition) Encrypted() (bool, error) {
	sb, err := p.SuperBlock()
	if err != nil {
		return false, err
	}

	return sb != nil && sb.Encrypted(), nil
}

// Path returns partition path.
func (p *Partition) Path() (string, error) {
	return util.PartPath(p.devname, int(p.Number))
}
