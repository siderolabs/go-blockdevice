// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package gpt implements read/write support for GPT partition tables.
package gpt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"slices"

	"github.com/google/uuid"
	"github.com/siderolabs/gen/xslices"
	"golang.org/x/sys/unix"
	"golang.org/x/text/encoding/unicode"

	"github.com/siderolabs/go-blockdevice/v2/block"
	"github.com/siderolabs/go-blockdevice/v2/internal/gptstructs"
	"github.com/siderolabs/go-blockdevice/v2/internal/gptutil"
	"github.com/siderolabs/go-blockdevice/v2/internal/ioutil"
)

// Device is an interface around actual block device.
type Device interface {
	io.ReaderAt
	io.WriterAt

	GetSectorSize() uint
	GetSize() uint64
	GetIOSize() (uint, error)
	Sync() error

	GetKernelLastPartitionNum() (int, error)
	KernelPartitionAdd(no int, start, length uint64) error
	KernelPartitionResize(no int, first, length uint64) error
	KernelPartitionDelete(no int) error
}

// Table is a wrapper type around GPT partition table.
type Table struct {
	dev Device
	// partition entries are indexed with the partition number.
	//
	// if the partition is missing, its entry is `nil`.
	entries []*Partition

	lastLBA uint64

	primaryHeaderLBA, secondaryHeaderLBA         uint64
	primaryPartitionsLBA, secondaryPartitionsLBA uint64
	firstUsableLBA, lastUsableLBA                uint64

	diskGUID uuid.UUID

	options Options

	alignment  uint64
	sectorSize uint
}

// Partition is a single partition entry in GPT.
type Partition struct {
	Name string

	TypeGUID uuid.UUID
	PartGUID uuid.UUID

	FirstLBA uint64
	LastLBA  uint64

	Flags uint64
}

type deviceWrapper struct {
	*os.File
	*block.Device

	size uint64
}

func (wrapper *deviceWrapper) GetSize() uint64 {
	return wrapper.size
}

// DeviceFromBlockDevice creates a new Device from a block.Device.
func DeviceFromBlockDevice(dev *block.Device, f *os.File) (Device, error) {
	size, err := dev.GetSize()
	if err != nil {
		return nil, err
	}

	return &deviceWrapper{
		File:   f,
		Device: dev,
		size:   size,
	}, nil
}

// New creates a new (empty) partition table for a specified device.
func New(dev Device, opts ...Option) (*Table, error) {
	var options Options

	for _, opt := range opts {
		opt(&options)
	}

	lastLBA, ok := gptutil.LastLBA(dev)
	if !ok {
		return nil, errors.New("failed to calculate last LBA (device too small?)")
	}

	if lastLBA < 33 {
		return nil, errors.New("device too small for GPT")
	}

	diskGUID := options.DiskGUID
	if diskGUID == uuid.Nil {
		diskGUID = uuid.New()
	}

	t := &Table{
		dev:      dev,
		options:  options,
		diskGUID: diskGUID,
	}

	t.init(lastLBA)

	return t, nil
}

// Read reads the partition table from the device.
func Read(dev Device, opts ...Option) (*Table, error) {
	var options Options

	for _, opt := range opts {
		opt(&options)
	}

	lastLBA, ok := gptutil.LastLBA(dev)
	if !ok {
		return nil, errors.New("failed to calculate last LBA (device too small?)")
	}

	if lastLBA < 33 {
		return nil, errors.New("device too small for GPT")
	}

	hdr, entries, err := gptstructs.ReadHeader(dev, 1, lastLBA)
	if err != nil {
		return nil, err
	}

	if hdr == nil {
		hdr, entries, err = gptstructs.ReadHeader(dev, lastLBA, lastLBA)
		if err != nil {
			return nil, err
		}
	}

	if hdr == nil {
		return nil, errors.New("no GPT header found")
	}

	diskGUID, err := uuid.FromBytes(gptutil.GUIDToUUID(hdr.Get_disk_guid()))
	if err != nil {
		return nil, err
	}

	t := &Table{
		dev:      dev,
		options:  options,
		diskGUID: diskGUID,
	}

	t.init(lastLBA)

	// decode entries
	partitions := make([]*Partition, len(entries))

	zeroGUID := make([]byte, 16)
	utf16 := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)

	lastFilledIdx := -1

	for idx, entry := range entries {
		if entry.Get_starting_lba() < t.firstUsableLBA || entry.Get_ending_lba() > t.lastUsableLBA {
			continue
		}

		// skip zero GUIDs
		if bytes.Equal(entry.Get_partition_type_guid(), zeroGUID) {
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

		partitions[idx] = &Partition{
			Name: string(name),

			TypeGUID: typeUUID,
			PartGUID: partUUID,

			FirstLBA: entry.Get_starting_lba(),
			LastLBA:  entry.Get_ending_lba(),

			Flags: entry.Get_attributes(),
		}

		lastFilledIdx = idx
	}

	if lastFilledIdx >= 0 {
		t.entries = partitions[:lastFilledIdx+1]
	}

	return t, nil
}

func (t *Table) init(lastLBA uint64) {
	t.lastLBA = lastLBA
	t.sectorSize = t.dev.GetSectorSize()

	lbasForEntries := (gptstructs.ENTRY_SIZE*gptstructs.NumEntries + t.sectorSize - 1) / t.sectorSize

	t.primaryHeaderLBA = uint64(1)
	t.secondaryHeaderLBA = lastLBA

	t.primaryPartitionsLBA = t.primaryHeaderLBA + 1 + uint64(t.options.SkipLBAs)
	t.secondaryPartitionsLBA = t.secondaryHeaderLBA - uint64(lbasForEntries)

	t.firstUsableLBA = t.primaryPartitionsLBA + uint64(lbasForEntries)
	t.lastUsableLBA = t.secondaryPartitionsLBA - 1

	ioSize, err := t.dev.GetIOSize()
	if err != nil {
		ioSize = t.sectorSize
	}

	alignmentSize := max(ioSize, 2048*512)
	t.alignment = uint64((alignmentSize + t.sectorSize - 1) / t.sectorSize)
}

// Clear the partition table.
func (t *Table) Clear() {
	t.entries = nil
}

// Compact the partition table by removing empty entries.
func (t *Table) Compact() {
	t.entries = xslices.FilterInPlace(t.entries, func(e *Partition) bool {
		return e != nil
	})
}

type allocatableRange struct {
	lowLBA  uint64
	highLBA uint64

	partitionIdx int

	size uint64
}

// allocatableRanges returns the slices of LBA ranges that are not allocated to any partition.
func (t *Table) allocatableRanges() []allocatableRange {
	partitionIdx := 0
	lowLBA := t.firstUsableLBA

	var ranges []allocatableRange

	for {
		for partitionIdx < len(t.entries) {
			if t.entries[partitionIdx] == nil {
				partitionIdx++
			}

			break
		}

		var highLBA uint64

		if partitionIdx < len(t.entries) {
			highLBA = t.entries[partitionIdx].FirstLBA - 1
		} else {
			highLBA = t.lastUsableLBA
		}

		lowLBA = (lowLBA + t.alignment - 1) / t.alignment * t.alignment

		if highLBA > lowLBA {
			ranges = append(ranges, allocatableRange{
				lowLBA:       lowLBA,
				highLBA:      highLBA,
				partitionIdx: partitionIdx,
				size:         (highLBA - lowLBA + 1) * uint64(t.sectorSize),
			})
		}

		if highLBA == t.lastUsableLBA {
			break
		}

		lowLBA = t.entries[partitionIdx].LastLBA + 1
		partitionIdx++
	}

	return ranges
}

// LargestContiguousAllocatable returns the size of the largest contiguous allocatable range.
func (t *Table) LargestContiguousAllocatable() uint64 {
	ranges := t.allocatableRanges()

	var largest uint64

	for _, r := range ranges {
		if r.size > largest {
			largest = r.size
		}
	}

	return largest
}

// AllocatePartition adds a new partition to the table.
//
// If successful, returns the partition number (1-indexed) and the partition entry created.
func (t *Table) AllocatePartition(size uint64, name string, partType uuid.UUID, opts ...PartitionOption) (int, Partition, error) {
	var options PartitionOptions

	for _, o := range opts {
		o(&options)
	}

	if size < uint64(t.sectorSize) {
		return 0, Partition{}, errors.New("partition size must be greater than sector size")
	}

	if options.UniqueGUID == uuid.Nil {
		options.UniqueGUID = uuid.New()
	}

	var smallestRange allocatableRange

	for _, allocatableRange := range t.allocatableRanges() {
		if allocatableRange.size >= size && (smallestRange.size == 0 || allocatableRange.size < smallestRange.size) {
			smallestRange = allocatableRange
		}
	}

	if smallestRange.size == 0 {
		return 0, Partition{}, errors.New("no allocatable range found")
	}

	entry := &Partition{
		Name:     name,
		TypeGUID: partType,
		PartGUID: options.UniqueGUID,
		FirstLBA: smallestRange.lowLBA,
		LastLBA:  smallestRange.lowLBA + size/uint64(t.sectorSize) - 1,
		Flags:    options.Flags,
	}

	if smallestRange.partitionIdx > 0 && t.entries[smallestRange.partitionIdx-1] == nil {
		t.entries[smallestRange.partitionIdx-1] = entry
	} else {
		t.entries = slices.Insert(
			t.entries,
			smallestRange.partitionIdx,
			entry,
		)
	}

	return smallestRange.partitionIdx + 1, *entry, nil
}

// AvailablePartitionGrowth returns the number of bytes that can be added to the partition.
func (t *Table) AvailablePartitionGrowth(partition int) (uint64, error) {
	if partition < 0 || partition >= len(t.entries) {
		return 0, fmt.Errorf("partition %d out of range", partition)
	}

	if t.entries[partition] == nil {
		return 0, fmt.Errorf("partition %d is not allocated", partition)
	}

	for _, allocatableRange := range t.allocatableRanges() {
		if allocatableRange.partitionIdx == partition+1 {
			return allocatableRange.size, nil
		}
	}

	return 0, nil
}

// GrowPartition grows the partition by the specified number of bytes.
func (t *Table) GrowPartition(partition int, size uint64) error {
	allowedGrowth, err := t.AvailablePartitionGrowth(partition)
	if err != nil {
		return err
	}

	if size > allowedGrowth {
		return fmt.Errorf("requested growth %d exceeds available growth %d", size, allowedGrowth)
	}

	entry := t.entries[partition]
	entry.LastLBA += size / uint64(t.sectorSize)

	return nil
}

// DeletePartition deletes a partition from the table.
func (t *Table) DeletePartition(partition int) error {
	if partition < 0 || partition >= len(t.entries) {
		return fmt.Errorf("partition %d out of range", partition)
	}

	t.entries[partition] = nil

	return nil
}

// Partitions returns the list of partitions in the table.
//
// The returned list should not be modified.
// Partitions in the list are zero-indexed, while
// Linux kernel partitions are one-indexed.
func (t *Table) Partitions() []*Partition {
	return slices.Clone(t.entries)
}

// Write writes the partition table to the device.
func (t *Table) Write() error {
	// build entries
	entriesBuf := make([]byte, gptstructs.ENTRY_SIZE*gptstructs.NumEntries)

	utf16 := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)

	for i, entry := range t.entries {
		if entry == nil {
			// zeroed entry
			continue
		}

		// write partition entry
		entryBuf := gptstructs.Entry(entriesBuf[i*gptstructs.ENTRY_SIZE : (i+1)*gptstructs.ENTRY_SIZE])
		entryBuf.Put_partition_type_guid(gptutil.UUIDToGUID(entry.TypeGUID[:]))
		entryBuf.Put_unique_partition_guid(gptutil.UUIDToGUID(entry.PartGUID[:]))
		entryBuf.Put_starting_lba(entry.FirstLBA)
		entryBuf.Put_ending_lba(entry.LastLBA)
		entryBuf.Put_attributes(entry.Flags)

		nameBuf, err := utf16.NewEncoder().Bytes([]byte(entry.Name))
		if err != nil {
			return fmt.Errorf("failed to encode partition name: %w", err)
		}

		if len(nameBuf) > 72 {
			return fmt.Errorf("partition name %q too long: %d bytes", entry.Name, len(nameBuf))
		}

		entryBuf.Put_partition_name(nameBuf)
	}

	entriesChecksum := crc32.ChecksumIEEE(entriesBuf)

	// GPT header should occupy whole sector
	header := gptstructs.Header(make([]byte, t.sectorSize))
	header.Put_signature(gptstructs.HeaderSignature)
	header.Put_revision(0x00010000)
	header.Put_header_size(gptstructs.HEADER_SIZE)
	header.Put_first_usable_lba(t.firstUsableLBA)
	header.Put_last_usable_lba(t.lastUsableLBA)
	header.Put_disk_guid(gptutil.UUIDToGUID(t.diskGUID[:]))
	header.Put_num_partition_entries(gptstructs.NumEntries)
	header.Put_sizeof_partition_entry(gptstructs.ENTRY_SIZE)
	header.Put_partition_entry_array_crc32(entriesChecksum)

	// now, primary and secondary headers/entries
	primaryHeader := slices.Clone(header)
	primaryHeader.Put_my_lba(t.primaryHeaderLBA)
	primaryHeader.Put_alternate_lba(t.secondaryHeaderLBA)
	primaryHeader.Put_partition_entries_lba(t.primaryPartitionsLBA)
	primaryHeader.Put_header_crc32(primaryHeader.CalculateChecksum())

	_, err := t.dev.WriteAt(primaryHeader, int64(t.primaryHeaderLBA)*int64(t.sectorSize))
	if err != nil {
		return fmt.Errorf("failed to write primary header: %w", err)
	}

	_, err = t.dev.WriteAt(entriesBuf, int64(t.primaryPartitionsLBA)*int64(t.sectorSize))
	if err != nil {
		return fmt.Errorf("failed to write primary entries: %w", err)
	}

	secondaryHeader := slices.Clone(header)
	secondaryHeader.Put_my_lba(t.secondaryHeaderLBA)
	secondaryHeader.Put_alternate_lba(t.primaryHeaderLBA)
	secondaryHeader.Put_partition_entries_lba(t.secondaryPartitionsLBA)
	secondaryHeader.Put_header_crc32(secondaryHeader.CalculateChecksum())

	_, err = t.dev.WriteAt(secondaryHeader, int64(t.secondaryHeaderLBA)*int64(t.sectorSize))
	if err != nil {
		return fmt.Errorf("failed to write secondary header: %w", err)
	}

	_, err = t.dev.WriteAt(entriesBuf, int64(t.secondaryPartitionsLBA)*int64(t.sectorSize))
	if err != nil {
		return fmt.Errorf("failed to write secondary entries: %w", err)
	}

	if !t.options.SkipPMBR {
		// write protective MBR
		if err = t.writePMBR(); err != nil {
			return err
		}
	}

	if err = t.dev.Sync(); err != nil {
		return fmt.Errorf("failed to sync device: %w", err)
	}

	return t.syncKernel()
}

func (t *Table) writePMBR() error {
	protectiveMBR := make([]byte, 512)

	if err := ioutil.ReadFullAt(t.dev, protectiveMBR, 0); err != nil {
		return fmt.Errorf("failed to read protective MBR: %w", err)
	}

	// boot signature
	protectiveMBR[510], protectiveMBR[511] = 0x55, 0xAA
	protectiveMBR[511] = 0xAA

	// PMBR protective entry.
	b := protectiveMBR[446 : 446+16]

	if t.options.MarkPMBRBootable {
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
	if t.lastLBA > math.MaxUint32 {
		binary.LittleEndian.PutUint32(b[12:16], uint32(math.MaxUint32))
	} else {
		binary.LittleEndian.PutUint32(b[12:16], uint32(t.lastLBA))
	}

	_, err := t.dev.WriteAt(protectiveMBR, 0)
	if err != nil {
		return fmt.Errorf("failed to write protective MBR: %w", err)
	}

	return nil
}

func (t *Table) syncKernel() error {
	kernelPartitionNum, err := t.dev.GetKernelLastPartitionNum()
	if err != nil {
		return fmt.Errorf("failed to get kernel last partition number: %w", err)
	}

	partitionNum := max(kernelPartitionNum, len(t.entries))

	for no := 1; no <= partitionNum; no++ {
		var myEntry *Partition
		if no <= len(t.entries) {
			myEntry = t.entries[no-1]
		}

		// try to delete the partition first
		err := t.dev.KernelPartitionDelete(no)

		switch {
		case errors.Is(err, unix.ENXIO):
		// partition doesn't exist, ok
		case errors.Is(err, unix.EBUSY) && myEntry != nil:
			// proceed to resize
			err = t.dev.KernelPartitionResize(no,
				myEntry.FirstLBA*uint64(t.sectorSize),
				(myEntry.LastLBA-myEntry.FirstLBA+1)*uint64(t.sectorSize))
			if err != nil {
				return fmt.Errorf("failed to resize partition %d: %w", no, err)
			}

			continue
		case err != nil:
			return fmt.Errorf("failed to delete partition %d: %w", no, err)
		}

		err = t.dev.KernelPartitionAdd(no,
			myEntry.FirstLBA*uint64(t.sectorSize),
			(myEntry.LastLBA-myEntry.FirstLBA+1)*uint64(t.sectorSize),
		)
		if err != nil {
			return fmt.Errorf("failed to add partition %d: %w", no, err)
		}
	}

	return nil
}
