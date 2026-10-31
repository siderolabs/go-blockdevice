// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gpt_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/siderolabs/go-blockdevice/v2/partitioning/gpt"
)

// gptSignature is the on-disk "EFI PART" magic at the start of a GPT header.
var gptSignature = []byte("EFI PART")

func newImageFile(t *testing.T, size int64) *os.File {
	t.Helper()

	f, err := os.Create(filepath.Join(t.TempDir(), "image.raw"))
	require.NoError(t, err)
	require.NoError(t, f.Truncate(size))

	t.Cleanup(func() {
		assert.NoError(t, f.Close())
	})

	return f
}

func TestDeviceFromFileDefaultSectorSize(t *testing.T) {
	f := newImageFile(t, 16*1024*1024)

	dev, err := gpt.DeviceFromFile(f)
	require.NoError(t, err)

	assert.Equal(t, uint(512), dev.GetSectorSize())

	ioSize, err := dev.GetIOSize()
	require.NoError(t, err)
	assert.Equal(t, uint(512), ioSize)
}

func TestDeviceFromFileWithSectorSize(t *testing.T) {
	for _, sectorSize := range []uint{512, 4096} {
		t.Run("sector="+strconv.FormatUint(uint64(sectorSize), 10), func(t *testing.T) {
			f := newImageFile(t, 64*1024*1024)

			dev, err := gpt.DeviceFromFile(f, gpt.WithFileSectorSize(sectorSize))
			require.NoError(t, err)

			assert.Equal(t, sectorSize, dev.GetSectorSize())

			ioSize, err := dev.GetIOSize()
			require.NoError(t, err)
			assert.Equal(t, sectorSize, ioSize)
		})
	}
}

// TestDeviceFromFileSectorSizeRoundTrip ensures the configured sector size is
// honored end-to-end: a GPT table written through the file device with a 4K
// sector size must place the primary header at LBA 1 of a 4K sector (offset
// 4096), and a re-read must recover the same partition layout.
func TestDeviceFromFileSectorSizeRoundTrip(t *testing.T) {
	const (
		MiB        = 1024 * 1024
		imageSize  = 64 * MiB
		sectorSize = 4096
	)

	partType := uuid.MustParse("0FC63DAF-8483-4772-8E79-3D69D8477DE4")

	f := newImageFile(t, imageSize)

	dev, err := gpt.DeviceFromFile(f, gpt.WithFileSectorSize(sectorSize))
	require.NoError(t, err)

	table, err := gpt.New(dev)
	require.NoError(t, err)

	idx, _, err := table.AllocatePartition(8*MiB, "test", partType)
	require.NoError(t, err)
	require.Equal(t, 1, idx)

	require.NoError(t, table.Write())

	// Primary header sits at LBA 1, which with a 4K sector is offset 4096.
	hdr := make([]byte, len(gptSignature))
	_, err = f.ReadAt(hdr, sectorSize)
	require.NoError(t, err)
	assert.Equal(t, gptSignature, hdr, "expected GPT signature at offset %d (LBA 1 of %d-byte sectors)", sectorSize, sectorSize)

	// At offset 512 (LBA 1 if sector size were 512) there must NOT be a GPT
	// signature - that location is inside the protective MBR sector.
	at512 := make([]byte, len(gptSignature))
	_, err = f.ReadAt(at512, 512)
	require.NoError(t, err)
	assert.NotEqual(t, gptSignature, at512, "GPT signature unexpectedly found at offset 512; sector size override was not honored")

	// Re-read the table and confirm partitions round-trip.
	dev2, err := gpt.DeviceFromFile(f, gpt.WithFileSectorSize(sectorSize))
	require.NoError(t, err)

	table2, err := gpt.Read(dev2)
	require.NoError(t, err)

	assert.Equal(t, table.Partitions(), table2.Partitions())
}

// TestDeviceFromFileDefaultSectorPlacement is the 512-byte counterpart of the
// test above - it pins the documented default behavior, so any future change
// that silently shifts the default sector size will break here.
func TestDeviceFromFileDefaultSectorPlacement(t *testing.T) {
	const (
		MiB       = 1024 * 1024
		imageSize = 16 * MiB
	)

	partType := uuid.MustParse("0FC63DAF-8483-4772-8E79-3D69D8477DE4")

	f := newImageFile(t, imageSize)

	dev, err := gpt.DeviceFromFile(f)
	require.NoError(t, err)

	table, err := gpt.New(dev)
	require.NoError(t, err)

	_, _, err = table.AllocatePartition(2*MiB, "test", partType)
	require.NoError(t, err)

	require.NoError(t, table.Write())

	hdr := make([]byte, len(gptSignature))
	_, err = f.ReadAt(hdr, 512)
	require.NoError(t, err)
	assert.Equal(t, gptSignature, hdr, "expected GPT signature at offset 512 with default 512-byte sectors")
}

// TestAvailablePartitionGrowthAcrossEmptyEntries verifies that the free space
// after a partition is reported even when the GPT entries that follow it are empty.
func TestAvailablePartitionGrowthAcrossEmptyEntries(t *testing.T) {
	const (
		MiB       = 1024 * 1024
		imageSize = 64 * MiB
	)

	partType := uuid.MustParse("0FC63DAF-8483-4772-8E79-3D69D8477DE4")

	for _, test := range []struct {
		name           string
		deleted        []int
		expectedGrowth uint64
	}{
		{
			name:           "gap before next partition",
			deleted:        []int{1, 2},
			expectedGrowth: 16 * MiB,
		},
		{
			name:           "gap up to the end of the disk",
			deleted:        []int{1, 2, 3},
			expectedGrowth: 54 * MiB,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dev, err := gpt.DeviceFromFile(newImageFile(t, imageSize))
			require.NoError(t, err)

			table, err := gpt.New(dev)
			require.NoError(t, err)

			for i := range 4 {
				idx, _, allocErr := table.AllocatePartition(8*MiB, "part"+strconv.Itoa(i+1), partType)
				require.NoError(t, allocErr)
				require.Equal(t, i+1, idx)
			}

			for _, idx := range test.deleted {
				require.NoError(t, table.DeletePartition(idx))
			}

			growth, err := table.AvailablePartitionGrowth(0)
			require.NoError(t, err)
			assert.EqualValues(t, test.expectedGrowth, growth)

			require.NoError(t, table.GrowPartition(0, growth))

			growth, err = table.AvailablePartitionGrowth(0)
			require.NoError(t, err)
			assert.EqualValues(t, 0, growth)
		})
	}
}

// sfdiskImage creates a disk image with the partition table created by sfdisk from the script.
//
// The script can use `%s` as a placeholder for the image path, which allows to set explicit
// partition numbers, e.g. `%s3 : start=2048, size=2048`.
func sfdiskImage(t *testing.T, size int64, script string) *os.File {
	t.Helper()

	if _, err := exec.LookPath("sfdisk"); err != nil {
		t.Skip("sfdisk is not available")
	}

	f := newImageFile(t, size)

	cmd := exec.CommandContext(t.Context(), "sfdisk", "--quiet", "--no-reread", "--no-tell-kernel", f.Name())
	cmd.Stdin = strings.NewReader(strings.ReplaceAll(script, "%s", f.Name()))

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	return f
}

// assertRoundTrip writes the table to the device, reads it back and verifies that all partitions survived
// (gpt.Read drops entries outside of the usable space, so this also verifies the partition bounds).
func assertRoundTrip(t *testing.T, dev gpt.Device, table *gpt.Table) {
	t.Helper()

	expected := table.Partitions()

	require.NoError(t, table.Write())

	table, err := gpt.Read(dev)
	require.NoError(t, err)

	assert.Equal(t, expected, table.Partitions())
	assertNoOverlap(t, table)
}

// assertNoOverlap verifies that every partition has a valid LBA range and no two partitions in the table overlap.
func assertNoOverlap(t *testing.T, table *gpt.Table) {
	t.Helper()

	partitions := table.Partitions()

	for i, a := range partitions {
		if a == nil {
			continue
		}

		assert.LessOrEqual(t, a.FirstLBA, a.LastLBA, "partition %d", i+1)

		for j, b := range partitions[i+1:] {
			if b == nil {
				continue
			}

			assert.False(t, a.FirstLBA <= b.LastLBA && b.FirstLBA <= a.LastLBA,
				"partition %d (%d-%d) overlaps partition %d (%d-%d)", i+1, a.FirstLBA, a.LastLBA, i+j+2, b.FirstLBA, b.LastLBA)
		}
	}
}

// TestAvailablePartitionGrowthOnDisk verifies partition growth with tables read from the disk,
// which might be not sorted by LBA and might have partitions not aligned.
func TestAvailablePartitionGrowthOnDisk(t *testing.T) {
	const (
		MiB       = 1024 * 1024
		imageSize = 64 * MiB
		sector    = 512
	)

	for _, test := range []struct {
		name   string
		script string

		partition      int
		expectedGrowth uint64
	}{
		{
			// entries: [A, nil, C, B], B sits physically between A and C
			name: "empty entry followed by out of order partition",
			script: `label: gpt
%s1 : start=2048, size=16384
%s3 : start=40960, size=16384
%s4 : start=20480, size=16384
`,
			partition:      0,
			expectedGrowth: (20480 - 18432) * sector,
		},
		{
			name: "out of order partition grows up to the partition with lower index",
			script: `label: gpt
%s1 : start=2048, size=16384
%s3 : start=40960, size=16384
%s4 : start=20480, size=16384
`,
			partition:      3,
			expectedGrowth: (40960 - 36864) * sector,
		},
		{
			name: "partition with higher index grows up to the end of the disk",
			script: `label: gpt
%s1 : start=2048, size=16384
%s3 : start=40960, size=16384
%s4 : start=20480, size=16384
`,
			partition:      2,
			expectedGrowth: (129024 - 57344) * sector,
		},
		{
			// entries: [B, A], the last entry is physically before the first one
			name: "last entry is not the last partition",
			script: `label: gpt
%s1 : start=98304, size=16384
%s2 : start=2048, size=2048
`,
			partition:      1,
			expectedGrowth: (98304 - 4096) * sector,
		},
		{
			name: "unaligned partition end",
			script: `label: gpt
%s1 : start=2048, size=16393
%s2 : start=40960, size=16384
`,
			partition:      0,
			expectedGrowth: (40960 - 18441) * sector,
		},
		{
			name: "single sector gap",
			script: `label: gpt
%s1 : start=2048, size=2048
%s2 : start=4097, size=16384
`,
			partition:      0,
			expectedGrowth: sector,
		},
		{
			name: "no gap",
			script: `label: gpt
%s1 : start=2048, size=2048
%s2 : start=4096, size=16384
`,
			partition:      0,
			expectedGrowth: 0,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dev, err := gpt.DeviceFromFile(sfdiskImage(t, imageSize, test.script))
			require.NoError(t, err)

			table, err := gpt.Read(dev)
			require.NoError(t, err)

			growth, err := table.AvailablePartitionGrowth(test.partition)
			require.NoError(t, err)
			assert.EqualValues(t, test.expectedGrowth, growth)

			require.NoError(t, table.GrowPartition(test.partition, growth))
			assertNoOverlap(t, table)

			// growing by the available growth should fill the whole gap
			growth, err = table.AvailablePartitionGrowth(test.partition)
			require.NoError(t, err)
			assert.EqualValues(t, 0, growth)

			assertRoundTrip(t, dev, table)
		})
	}
}

// TestAllocatePartitionOnDisk verifies partition allocation with tables read from the disk,
// which might be not sorted by LBA.
func TestAllocatePartitionOnDisk(t *testing.T) {
	const (
		MiB       = 1024 * 1024
		imageSize = 64 * MiB
		sector    = 512
	)

	partType := uuid.MustParse("0FC63DAF-8483-4772-8E79-3D69D8477DE4")

	for _, test := range []struct {
		name   string
		script string
		size   uint64

		expectedIdx      int
		expectedFirstLBA uint64
		expectedLastLBA  uint64
	}{
		{
			// free ranges: 18432-20479 (1 MiB), 36864-40959 (2 MiB), 57344-129023
			name: "smallest fitting range between out of order partitions",
			script: `label: gpt
%s1 : start=2048, size=16384
%s3 : start=40960, size=16384
%s4 : start=20480, size=16384
`,
			size:             2 * MiB,
			expectedIdx:      2, // empty entry is reused
			expectedFirstLBA: 36864,
			expectedLastLBA:  40959,
		},
		{
			name: "range at the end of the disk",
			script: `label: gpt
%s1 : start=2048, size=16384
%s3 : start=40960, size=16384
%s4 : start=20480, size=16384
`,
			size:             8 * MiB,
			expectedIdx:      2, // empty entry is reused
			expectedFirstLBA: 57344,
			expectedLastLBA:  57344 + 16384 - 1,
		},
		{
			// entries: [A, nil, C, B], the range is bounded by B
			name: "range bounded by an out of order partition",
			script: `label: gpt
%s1 : start=2048, size=16384
%s3 : start=40960, size=16384
%s4 : start=20480, size=16384
`,
			size:             1 * MiB,
			expectedIdx:      2,
			expectedFirstLBA: 18432,
			expectedLastLBA:  20479,
		},
		{
			// entries: [B, A], the range between A and B is bounded by the first entry
			name: "new entry is appended when there are no empty entries",
			script: `label: gpt
%s1 : start=98304, size=16384
%s2 : start=2048, size=2048
`,
			size:             8 * MiB,
			expectedIdx:      3,
			expectedFirstLBA: 4096,
			expectedLastLBA:  4096 + 16384 - 1,
		},
		{
			name: "single sector range",
			script: `label: gpt
%s1 : start=2048, size=2048
%s2 : start=4097, size=16384
`,
			size:             sector,
			expectedIdx:      3, // existing partitions keep their numbers
			expectedFirstLBA: 4096,
			expectedLastLBA:  4096,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dev, err := gpt.DeviceFromFile(sfdiskImage(t, imageSize, test.script))
			require.NoError(t, err)

			table, err := gpt.Read(dev)
			require.NoError(t, err)

			before := table.Partitions()

			idx, part, err := table.AllocatePartition(test.size, "new", partType)
			require.NoError(t, err)

			assert.Equal(t, test.expectedIdx, idx)
			assert.Equal(t, test.expectedFirstLBA, part.FirstLBA)
			assert.Equal(t, test.expectedLastLBA, part.LastLBA)

			// existing partitions should never be renumbered
			after := table.Partitions()

			for i, entry := range before {
				if entry != nil {
					assert.Same(t, entry, after[i], "partition %d was moved", i+1)
				}
			}

			assertNoOverlap(t, table)
			assertRoundTrip(t, dev, table)
		})
	}
}

// TestAllocatePartitionTableFull verifies that allocation fails once all partition entries are used.
func TestAllocatePartitionTableFull(t *testing.T) {
	const (
		MiB        = 1024 * 1024
		numEntries = 128
	)

	partType := uuid.MustParse("0FC63DAF-8483-4772-8E79-3D69D8477DE4")

	dev, err := gpt.DeviceFromFile(newImageFile(t, (numEntries+16)*MiB))
	require.NoError(t, err)

	table, err := gpt.New(dev)
	require.NoError(t, err)

	for i := range numEntries {
		idx, _, allocErr := table.AllocatePartition(MiB, "part"+strconv.Itoa(i+1), partType)
		require.NoError(t, allocErr)
		require.Equal(t, i+1, idx)
	}

	require.Greater(t, table.LargestContiguousAllocatable(), uint64(MiB))

	_, _, err = table.AllocatePartition(MiB, "overflow", partType)
	require.ErrorContains(t, err, "partition table is full")

	// once an entry is freed, it is reused
	require.NoError(t, table.DeletePartition(4))

	idx, part, err := table.AllocatePartition(2*MiB, "reused", partType)
	require.NoError(t, err)
	assert.Equal(t, 5, idx)
	assert.Greater(t, part.FirstLBA, table.Partitions()[numEntries-1].LastLBA)

	assertNoOverlap(t, table)
	assertRoundTrip(t, dev, table)
}
