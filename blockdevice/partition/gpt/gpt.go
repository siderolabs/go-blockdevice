// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gpt

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"

	"github.com/google/uuid"

	"github.com/talos-systems/go-blockdevice/blockdevice/blkpg"
	"github.com/talos-systems/go-blockdevice/blockdevice/lba"
)

const (
	// MagicEFIPart is the magic string in the GPT header signature field.
	MagicEFIPart = "EFI PART"
	// HeaderSize is the GUID partition table header size in bytes.
	HeaderSize = 92
)

var (
	// ErrPartitionTableDoesNotExist indicates that the partition table does not exist.
	ErrPartitionTableDoesNotExist = errors.New("block device partition table does not exist")
	// ErrHeaderCRCMismatch indicates that the header CRC does not match what is on disk.
	ErrHeaderCRCMismatch = errors.New("block device partition table header CRC mismatch")
	// ErrEntriesCRCMismatch indicates that the partitions array CRC does not match what is on disk.
	ErrEntriesCRCMismatch = errors.New("block device partition table entries CRC mismatch")
)

type outOfSpaceError struct {
	error
}

func (outOfSpaceError) OutOfSpaceError() {}

// GPT represents the GUID Partition Table.
type GPT struct {
	f               *os.File
	l               *lba.LBA
	h               *Header
	e               *Partitions
	markMBRBootable bool
}

// Open attempts to open a partition table on f.
func Open(f *os.File) (g *GPT, err error) {
	buf := make([]byte, 16)

	// PMBR protective entry starts at 446. The partition type is at offset
	// 4 from the start of the PMBR protective entry.
	var n int

	n, err = f.ReadAt(buf, 446)
	if err != nil {
		return nil, err
	}

	if n != len(buf) {
		return nil, fmt.Errorf("incomplete read: %d != %d", n, len(buf))
	}

	// For GPT, the partition type should be 0xEE (EFI GPT).
	if buf[4] == 0xEE {
		l, err := lba.NewLBA(f)
		if err != nil {
			return nil, err
		}

		h := &Header{LBA: l}

		if err = h.verifySignature(); err != nil {
			return nil, ErrPartitionTableDoesNotExist
		}

		g = &GPT{
			f:               f,
			l:               l,
			h:               h,
			e:               &Partitions{h: h, devname: f.Name()},
			markMBRBootable: buf[0] == 0x80,
		}

		return g, nil
	}

	return nil, ErrPartitionTableDoesNotExist
}

// New creates an in-memory partition table.
func New(f *os.File, setters ...Option) (g *GPT, err error) {
	opts, err := NewDefaultOptions(setters...)
	if err != nil {
		return nil, err
	}

	l, err := lba.NewLBA(f)
	if err != nil {
		return nil, err
	}

	h := &Header{LBA: l}

	h.Signature = MagicEFIPart
	h.Revision = binary.LittleEndian.Uint32([]byte{0x00, 0x00, 0x01, 0x00})
	h.Size = HeaderSize
	h.CurrentLBA = 1
	h.EntriesLBA = opts.PartitionEntriesStartLBA
	h.NumberOfPartitionEntries = 128
	h.PartitionEntrySize = 128
	h.FirstUsableLBA = opts.PartitionEntriesStartLBA + 32
	h.BackupLBA = uint64(l.TotalSectors - 1)
	h.LastUsableLBA = h.BackupLBA - 33

	guuid, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("failed to generate UUID for new partition table: %w", err)
	}

	h.GUUID = guuid

	g = &GPT{
		f:               f,
		l:               l,
		h:               h,
		e:               &Partitions{h: h, devname: f.Name()},
		markMBRBootable: opts.MarkMBRBootable,
	}

	return g, nil
}

// Read reads the partition table on disk and updates the in-memory representation.
func (g *GPT) Read() (err error) {
	err = g.h.read()
	if err != nil {
		return err
	}

	err = g.e.read()
	if err != nil {
		return err
	}

	g.renumberPartitions()

	return nil
}

func (g *GPT) Write() (err error) {
	var pmbr []byte

	pmbr, err = g.newPMBR(g.h)
	if err != nil {
		return err
	}

	err = g.l.WriteAt(0, 0x00, pmbr)
	if err != nil {
		return err
	}

	// NB: Write the partitions first so that the header CRC calculations are
	// correct.

	data, err := g.e.write()
	if err != nil {
		return err
	}

	err = g.l.WriteAt(int64(g.h.EntriesLBA), 0x00, data)
	if err != nil {
		return err
	}

	err = g.l.WriteAt(int64(g.h.LastUsableLBA+1), 0x00, data)
	if err != nil {
		return err
	}

	err = g.h.write()
	if err != nil {
		return err
	}

	if err = g.f.Sync(); err != nil {
		return err
	}

	if err = g.syncKernelPartitions(); err != nil {
		return fmt.Errorf("failed to sync kernel partitions: %w", err)
	}

	return nil
}

// Header returns the partition table header.
func (g *GPT) Header() *Header {
	if g.h == nil {
		return &Header{}
	}

	return g.h
}

// Partitions returns the partition table partitions.
func (g *GPT) Partitions() *Partitions {
	if g.e == nil {
		return &Partitions{}
	}

	return g.e
}

// Add adds a partition to the end of the list.
func (g *GPT) Add(size uint64, setters ...PartitionOption) (*Partition, error) {
	return g.InsertAt(len(g.e.p), size, setters...)
}

// InsertAt inserts partition before the partition at the position idx.
//
// If idx == 0, it inserts new partition as the first partition, etc., idx == 1 as the second, etc.
func (g *GPT) InsertAt(idx int, size uint64, setters ...PartitionOption) (*Partition, error) {
	opts := NewDefaultPartitionOptions(setters...)

	// find the minimum and maximum LBAs available
	var minLBA, maxLBA uint64

	minLBA = g.h.FirstUsableLBA

	if opts.Offset != 0 {
		minLBA = opts.Offset / uint64(g.l.LogicalBlockSize)
	}

	for i := idx - 1; i >= 0; i-- {
		if g.e.p[i] != nil {
			if opts.Offset == 0 {
				minLBA = g.e.p[i].LastLBA + 1
			} else if g.e.p[i].LastLBA >= minLBA {
				return nil, outOfSpaceError{fmt.Errorf("requested partition with offset %d bytes, overlapping partition %d", opts.Offset, i)}
			}

			break
		}
	}

	maxLBA = g.h.LastUsableLBA

	// Find the maximum LBAs available.
	for i := idx; i < len(g.e.p); i++ {
		if g.e.p[i] != nil {
			maxLBA = g.e.p[i].FirstLBA - 1

			break
		}
	}

	// Find partition boundaries.
	var start, end uint64

	start = g.l.AlignToPhysicalBlockSize(minLBA)

	if opts.MaximumSize {
		end = maxLBA

		if end < start {
			return nil, outOfSpaceError{fmt.Errorf("requested partition with maximum size, but no space available")}
		}
	} else {
		// In GPT, partition end is inclusive.
		end = start + size/uint64(g.l.LogicalBlockSize) - 1

		if end > maxLBA {
			// Convert the total available LBAs to units of bytes.
			available := (maxLBA - start) * uint64(g.l.LogicalBlockSize)

			return nil, outOfSpaceError{fmt.Errorf("requested partition size %d, available is %d (%d too many bytes)", size, available, size-available)}
		}
	}

	uuid, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	partition := &Partition{
		Type:       opts.Type,
		ID:         uuid,
		FirstLBA:   start,
		LastLBA:    end,
		Attributes: opts.Attibutes,
		Name:       opts.Name,
		devname:    g.e.devname,
	}

	g.e.p = append(g.e.p[:idx], append([]*Partition{partition}, g.e.p[idx:]...)...)
	g.renumberPartitions()

	return partition, nil
}

// Delete deletes a partition.
func (g *GPT) Delete(p *Partition) error {
	index := -1

	for i, part := range g.e.p {
		if part == nil {
			continue
		}

		if part.ID == p.ID {
			index = i

			break
		}
	}

	if index == -1 {
		return fmt.Errorf("partition not found")
	}

	g.e.p[index] = nil
	g.renumberPartitions()

	return nil
}

// Resize resizes a partition to next one if exists.
func (g *GPT) Resize(part *Partition) (bool, error) {
	idx := int(part.Number - 1)
	if len(g.e.p) < idx {
		return false, fmt.Errorf("unknown partition %d, only %d available", part.Number, len(g.e.p))
	}

	maxLBA := g.h.LastUsableLBA

	for i := idx + 1; i < len(g.e.p); i++ {
		if g.e.p[i] != nil {
			maxLBA = g.e.p[i].FirstLBA - 1

			break
		}
	}

	if part.LastLBA >= maxLBA {
		return false, nil
	}

	part.LastLBA = maxLBA

	g.e.p[idx] = part

	return true, nil
}

// Repair repairs the partition table.
func (g *GPT) Repair() error {
	g.h.BackupLBA = uint64(g.l.TotalSectors - 1)
	g.h.LastUsableLBA = g.h.BackupLBA - 33

	return nil
}

// References:
// 	- https://en.wikipedia.org/wiki/GUID_Partition_Table#Protective_MBR_(LBA_0)
// 	- https://www.syslinux.org/wiki/index.php?title=Doc/gpt
// 	- https://en.wikipedia.org/wiki/Master_boot_record
// 	- http://www.rodsbooks.com/gdisk/bios.html
func (g *GPT) newPMBR(h *Header) ([]byte, error) {
	p, err := g.l.ReadAt(0, 0, 512)
	if err != nil {
		return nil, err
	}

	// Boot signature.
	copy(p[510:], []byte{0x55, 0xaa})

	// PMBR protective entry.
	b := p[446 : 446+16]

	if g.markMBRBootable {
		// Some BIOSes in legacy mode won't boot from a disk unless there is at least one
		// partition in the MBR marked bootable.  Mark this partition as bootable.
		b[0] = 0x80
	} else {
		b[0] = 0x00
	}

	// Partition type: EFI data partition.
	b[4] = 0xee

	// CHS for the start of the partition
	copy(b[1:4], []byte{0x00, 0x02, 0x00})

	// CHS for the end of the partition
	copy(b[5:8], []byte{0xff, 0xff, 0xff})

	// Partition start LBA.
	binary.LittleEndian.PutUint32(b[8:12], 1)

	// Partition length in sectors.
	// This might overflow uint32, so check accordingly
	if h.BackupLBA > math.MaxUint32 {
		binary.LittleEndian.PutUint32(b[12:16], uint32(math.MaxUint32))
	} else {
		binary.LittleEndian.PutUint32(b[12:16], uint32(h.BackupLBA))
	}

	return p, nil
}

func (g *GPT) renumberPartitions() {
	// In gpt, partition numbers aren't stored, so numbers are just in-memory representation.
	idx := int32(1)

	for i := range g.e.p {
		if g.e.p[i] == nil {
			continue
		}

		g.e.p[i].Number = idx
		idx++
	}
}

func (g *GPT) syncKernelPartitions() error {
	kernelPartitions, err := blkpg.GetKernelPartitions(g.f)
	if err != nil {
		return err
	}

	// filter out nil partitions
	newPartitions := make([]*Partition, 0, len(g.e.p))

	for _, part := range g.e.p {
		if part == nil {
			continue
		}

		newPartitions = append(newPartitions, part)
	}

	var i int

	// find partitions matching exactly or partitions which can be simply resized
	for i = 0; i < len(kernelPartitions) && i < len(newPartitions); i++ {
		kernelPart := kernelPartitions[i]
		newPart := newPartitions[i]

		// non-contiguous kernel partition table, stop
		if kernelPart.No != i+1 {
			break
		}

		// skip partitions without any changes
		if uint64(kernelPart.Start) == newPart.FirstLBA && uint64(kernelPart.Length) == newPart.Length() {
			continue
		}

		// resizing a partition which is the last one in the kernel list (no overlaps)
		if uint64(kernelPart.Start) == newPart.FirstLBA && i == len(kernelPartitions)-1 {
			if err := blkpg.InformKernelOfResize(g.f, newPart.FirstLBA, newPart.Length(), newPart.Number); err != nil {
				return err
			}

			continue
		}

		// partitions don't match, stop
		break
	}

	// process remaining partitions: delete all the kernel partitions left, add new partitions from in-memory set
	for j := i; j < len(kernelPartitions); j++ {
		if err := blkpg.InformKernelOfDelete(g.f, 0, 0, int32(kernelPartitions[j].No)); err != nil {
			return err
		}
	}

	for j := i; j < len(newPartitions); j++ {
		if err := blkpg.InformKernelOfAdd(g.f, newPartitions[j].FirstLBA, newPartitions[j].Length(), newPartitions[j].Number); err != nil {
			return err
		}
	}

	return nil
}
