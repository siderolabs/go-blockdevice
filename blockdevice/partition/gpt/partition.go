// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gpt

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/text/encoding/unicode"

	"github.com/talos-systems/go-blockdevice/blockdevice/endianness"
	"github.com/talos-systems/go-blockdevice/blockdevice/filesystem"
	"github.com/talos-systems/go-blockdevice/blockdevice/lba"
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

func (p *Partitions) read() (err error) {
	partitions := make([]*Partition, 0, p.h.NumberOfPartitionEntries)

	for i := uint32(0); i < p.h.NumberOfPartitionEntries; i++ {
		offset := i * p.h.PartitionEntrySize

		data, err := p.h.ReadAt(int64(p.h.EntriesLBA), int64(offset), int64(p.h.PartitionEntrySize))
		if err != nil {
			return fmt.Errorf("partition read: %w", err)
		}

		buf := lba.NewBuffer(p.h.LBA, data)

		part := &Partition{Number: int32(i + 1), devname: p.devname}

		err = part.DeserializeType(buf)
		if err != nil {
			return err
		}

		err = part.DeserializeID(buf)
		if err != nil {
			return err
		}

		err = part.DeserializeFirstLBA(buf)
		if err != nil {
			return err
		}

		err = part.DeserializeLastLBA(buf)
		if err != nil {
			return err
		}

		err = part.DeserializeAttributes(buf)
		if err != nil {
			return err
		}

		err = part.DeserializeName(buf)
		if err != nil {
			return err
		}

		// The first LBA of the partition cannot start before the first usable
		// LBA specified in the header.
		if part.FirstLBA >= p.h.FirstUsableLBA {
			partitions = append(partitions, part)
		}
	}

	p.p = partitions

	return nil
}

func (p *Partitions) write() (data []byte, err error) {
	data = make([]byte, p.h.NumberOfPartitionEntries*p.h.PartitionEntrySize)

	for i, part := range p.p {
		if part == nil {
			continue
		}

		b := make([]byte, p.h.PartitionEntrySize)
		buf := lba.NewBuffer(p.h.LBA, b)

		err = part.SerializeType(buf)
		if err != nil {
			return nil, err
		}

		err = part.SerializeID(buf)
		if err != nil {
			return nil, err
		}

		err = part.SerializeFirstLBA(buf)
		if err != nil {
			return nil, err
		}

		err = part.SerializeLastLBA(buf)
		if err != nil {
			return nil, err
		}

		err = part.SerializeAttributes(buf)
		if err != nil {
			return nil, err
		}

		err = part.SerializeName(buf)
		if err != nil {
			return nil, err
		}

		copy(data[i*int(p.h.PartitionEntrySize):], b)
	}

	return data, nil
}

// DeserializeType deserializes the partition type GUID (mixed endian).
func (p *Partition) DeserializeType(buf *lba.Buffer) (err error) {
	data, err := buf.Read(0x00, 16)
	if err != nil {
		return fmt.Errorf("type read: %w", err)
	}

	u, err := endianness.FromMiddleEndian(data)
	if err != nil {
		return err
	}

	guid, err := uuid.FromBytes(u)
	if err != nil {
		return fmt.Errorf("invalid GUUID: %w", err)
	}

	// TODO: Provide a method for getting the human readable name of the type.
	// See https://en.wikipedia.org/wiki/GUID_Partition_Table.
	p.Type = guid

	return nil
}

// SerializeType serializes the partition type GUID (mixed endian).
func (p *Partition) SerializeType(buf *lba.Buffer) (err error) {
	b, err := p.Type.MarshalBinary()
	if err != nil {
		return err
	}

	data, err := endianness.ToMiddleEndian(b)
	if err != nil {
		return err
	}

	err = buf.Write(data, 0x00)
	if err != nil {
		return err
	}

	return nil
}

// DeserializeID deserializes the unique partition GUID (mixed endian).
func (p *Partition) DeserializeID(buf *lba.Buffer) (err error) {
	data, err := buf.Read(0x10, 16)
	if err != nil {
		return fmt.Errorf("id read: %w", err)
	}

	u, err := endianness.FromMiddleEndian(data)
	if err != nil {
		return err
	}

	guid, err := uuid.FromBytes(u)
	if err != nil {
		return fmt.Errorf("invalid GUUID: %w", err)
	}

	p.ID = guid

	return nil
}

// SerializeID serializes the unique partition GUID (mixed endian).
func (p *Partition) SerializeID(buf *lba.Buffer) (err error) {
	b, err := p.ID.MarshalBinary()
	if err != nil {
		return err
	}

	data, err := endianness.ToMiddleEndian(b)
	if err != nil {
		return err
	}

	err = buf.Write(data, 0x10)
	if err != nil {
		return err
	}

	return nil
}

// DeserializeFirstLBA deserializes the first LBA (little endian).
func (p *Partition) DeserializeFirstLBA(buf *lba.Buffer) (err error) {
	data, err := buf.Read(0x20, 8)
	if err != nil {
		return fmt.Errorf("first LBA read: %w", err)
	}

	p.FirstLBA = binary.LittleEndian.Uint64(data)

	return nil
}

// SerializeFirstLBA serializes the first LBA (little endian).
func (p *Partition) SerializeFirstLBA(buf *lba.Buffer) (err error) {
	data := make([]byte, 8)

	binary.LittleEndian.PutUint64(data, p.FirstLBA)

	err = buf.Write(data, 0x20)
	if err != nil {
		return err
	}

	return nil
}

// DeserializeLastLBA deserializes the last LBA (inclusive, usually odd).
func (p *Partition) DeserializeLastLBA(buf *lba.Buffer) (err error) {
	data, err := buf.Read(0x28, 8)
	if err != nil {
		return fmt.Errorf("last LBA read: %w", err)
	}

	p.LastLBA = binary.LittleEndian.Uint64(data)

	return nil
}

// SerializeLastLBA serializes the last LBA (inclusive, usually odd).
func (p *Partition) SerializeLastLBA(buf *lba.Buffer) (err error) {
	data := make([]byte, 8)

	binary.LittleEndian.PutUint64(data, p.LastLBA)

	err = buf.Write(data, 0x28)
	if err != nil {
		return err
	}

	return nil
}

// DeserializeAttributes deserializes the attribute flags (e.g. bit 60 denotes read-only).
func (p *Partition) DeserializeAttributes(buf *lba.Buffer) (err error) {
	data, err := buf.Read(0x30, 8)
	if err != nil {
		return fmt.Errorf("attributes read: %w", err)
	}

	p.Attributes = binary.LittleEndian.Uint64(data)

	return nil
}

// SerializeAttributes serializes the attribute flags (e.g. bit 60 denotes read-only).
func (p *Partition) SerializeAttributes(buf *lba.Buffer) (err error) {
	data := make([]byte, 8)

	binary.LittleEndian.PutUint64(data, p.Attributes)

	err = buf.Write(data, 0x30)
	if err != nil {
		return err
	}

	return nil
}

// DeserializeName deserializes partition name (36 UTF-16LE code units).
func (p *Partition) DeserializeName(buf *lba.Buffer) (err error) {
	data, err := buf.Read(0x38, 72)
	if err != nil {
		return fmt.Errorf("name read: %w", err)
	}

	utf16 := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)

	decoded, err := utf16.NewDecoder().Bytes(data)
	if err != nil {
		return err
	}

	p.Name = string(bytes.Trim(decoded, "\x00"))

	return nil
}

// SerializeName serializes the partition name (36 UTF-16LE code units).
func (p *Partition) SerializeName(buf *lba.Buffer) (err error) {
	utf16 := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)

	name, err := utf16.NewEncoder().Bytes([]byte(p.Name))
	if err != nil {
		return err
	}

	// TODO: Should we error if the name exceeds 72 bytes?
	data := make([]byte, 72)
	copy(data, name)

	err = buf.Write(data, 0x38)
	if err != nil {
		return err
	}

	return nil
}

// SuperBlock read partition superblock.
// if partition is encrypted it will always return superblock of the physical partition,
// instead of a mapped device partition.
func (p *Partition) SuperBlock() (filesystem.SuperBlocker, error) {
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
