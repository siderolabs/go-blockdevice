// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build linux

package gpt_test

import (
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"io"
	randv2 "math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/freddierice/go-losetup/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/siderolabs/go-blockdevice/v2/block"
	"github.com/siderolabs/go-blockdevice/v2/partitioning/gpt"
)

const (
	MiB = 1024 * 1024
	GiB = 1024 * MiB
)

func sfdiskDump(t *testing.T, devPath string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "sfdisk", "--dump", devPath)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	assert.NoError(t, err)

	output := string(out)
	output = regexp.MustCompile(`device:[^\n]+\n`).ReplaceAllString(output, "")
	output = regexp.MustCompile(`[^\s]+\s*:\s*start=`).ReplaceAllString(output, "start=")

	t.Log("sfdisk output:\n", output)

	return output
}

func gdiskDump(t *testing.T, devPath string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "gdisk", "-l", devPath)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	assert.NoError(t, err)

	output := string(out)
	output = regexp.MustCompile(`^GPT [^\n]+\n\n`).ReplaceAllString(output, "")
	output = regexp.MustCompile(`Disk (/[^:]+|[^:]+\.(raw|img)):`).ReplaceAllString(output, "")
	output = strings.ReplaceAll(output, "\a", "")

	t.Log("gdisk output:\n", output)

	return output
}

//go:embed testdata/*
var testdataFs embed.FS

func loadTestdata(t *testing.T, name string) string {
	t.Helper()

	data, err := testdataFs.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)

	return string(data)
}

func assertAllocated(t *testing.T, expectedIndex int) func(_ int, _ gpt.Partition, err error) {
	return func(index int, _ gpt.Partition, err error) {
		t.Helper()

		require.NoError(t, err)
		assert.Equal(t, expectedIndex, index)
	}
}

//nolint:maintidx
func TestGPT(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test requires root privileges")
	}

	partType1 := uuid.MustParse("C12A7328-F81F-11D2-BA4B-00A0C93EC93B")
	partType2 := uuid.MustParse("E6D6D379-F507-44C2-A23C-238F2A3DF928")

	for _, test := range []struct { //nolint:govet
		name string

		opts []gpt.Option

		diskSize uint64

		allocator func(*testing.T, *gpt.Table)

		expectedSfdiskDump string
		expectedGdiskDump  string
	}{
		{
			name:     "empty",
			diskSize: 2 * GiB,
			opts: []gpt.Option{
				gpt.WithDiskGUID(uuid.MustParse("D815C311-BDED-43FE-A91A-DCBE0D8025D5")),
			},

			expectedSfdiskDump: loadTestdata(t, "empty.sfdisk"),
			expectedGdiskDump:  loadTestdata(t, "empty.gdisk"),
		},
		{
			name:     "empty without PMBR",
			diskSize: 2 * GiB,
			opts: []gpt.Option{
				gpt.WithDiskGUID(uuid.MustParse("D815C311-BDED-43FE-A91A-DCBE0D8025D5")),
				gpt.WithSkipPMBR(),
			},

			expectedGdiskDump: loadTestdata(t, "empty-no-mbr.gdisk"),
		},
		{
			name:     "simple allocate",
			diskSize: 6 * GiB,
			opts: []gpt.Option{
				gpt.WithDiskGUID(uuid.MustParse("B6D003E5-7D1D-45E3-9F4B-4A2430B46D4A")),
			},
			allocator: func(t *testing.T, table *gpt.Table) {
				t.Helper()

				assertAllocated(t, 1)(table.AllocatePartition(
					1*GiB, "1G", partType1,
					gpt.WithUniqueGUID(uuid.MustParse("DA66737E-1ED4-4DDF-B98C-70CEBFE3ADA0")),
				))
				assertAllocated(t, 2)(table.AllocatePartition(
					100*MiB, "100M", partType1,
					gpt.WithUniqueGUID(uuid.MustParse("3D0FE86B-7791-4659-B564-FC49A542866D")),
					gpt.WithLegacyBIOSBootableAttribute(true),
				))
				assertAllocated(t, 3)(table.AllocatePartition(
					2.5*GiB, "2.5G", partType2,
					gpt.WithUniqueGUID(uuid.MustParse("EE1A711E-DE12-4D9F-98FF-672F7AD638F8")),
				))
				assertAllocated(t, 4)(table.AllocatePartition(
					1*GiB, "1G", partType2,
					gpt.WithUniqueGUID(uuid.MustParse("15E609C8-9775-4E86-AF59-8A87E7C03FAB")),
				))
			},

			expectedSfdiskDump: loadTestdata(t, "allocate.sfdisk"),
			expectedGdiskDump:  loadTestdata(t, "allocate.gdisk"),
		},
		{
			name:     "allocate with LBA compatibility",
			diskSize: 2 * GiB,
			opts: []gpt.Option{
				gpt.WithDiskGUID(uuid.MustParse("B6D003E5-7D1D-45E3-9F4B-4A2430B46D4A")),
				gpt.WithCompatFirstUsableLBA(),
			},
			allocator: func(t *testing.T, table *gpt.Table) {
				t.Helper()

				assertAllocated(t, 1)(table.AllocatePartition(
					1*GiB, "1G", partType1,
					gpt.WithUniqueGUID(uuid.MustParse("DA66737E-1ED4-4DDF-B98C-70CEBFE3ADA0")),
				))
			},

			expectedSfdiskDump: loadTestdata(t, "allocate-lba-compat.sfdisk"),
			expectedGdiskDump:  loadTestdata(t, "allocate-lba-compat.gdisk"),
		},
		{
			name:     "allocate with small delete",
			diskSize: 4 * GiB,
			opts: []gpt.Option{
				gpt.WithDiskGUID(uuid.MustParse("B6D003E5-7D1D-45E3-9F4B-4A2430B46D4A")),
			},
			allocator: func(t *testing.T, table *gpt.Table) {
				t.Helper()

				assertAllocated(t, 1)(table.AllocatePartition(
					1*GiB, "1G", partType1,
					gpt.WithUniqueGUID(uuid.MustParse("DA66737E-1ED4-4DDF-B98C-70CEBFE3ADA0")),
				))
				assertAllocated(t, 2)(table.AllocatePartition(
					100*MiB, "100M", partType1,
					gpt.WithUniqueGUID(uuid.MustParse("3D0FE86B-7791-4659-B564-FC49A542866D")),
					gpt.WithLegacyBIOSBootableAttribute(true),
				))
				assertAllocated(t, 3)(table.AllocatePartition(
					table.LargestContiguousAllocatable(), "2.5G", partType2,
					gpt.WithUniqueGUID(uuid.MustParse("EE1A711E-DE12-4D9F-98FF-672F7AD638F8")),
				))

				require.NoError(t, table.DeletePartition(1))

				assert.EqualValues(t, 100*MiB, table.LargestContiguousAllocatable())

				assertAllocated(t, 2)(table.AllocatePartition(
					100*MiB, "100M", partType2,
					gpt.WithUniqueGUID(uuid.MustParse("15E609C8-9775-4E86-AF59-8A87E7C03FAB")),
				))
			},

			expectedSfdiskDump: loadTestdata(t, "smalldelete.sfdisk"),
			expectedGdiskDump:  loadTestdata(t, "smalldelete.gdisk"),
		},
		{
			name:     "allocate with deletes",
			diskSize: 6 * GiB,
			opts: []gpt.Option{
				gpt.WithDiskGUID(uuid.MustParse("B6D003E5-7D1D-45E3-9F4B-4A2430B46D4A")),
			},
			allocator: func(t *testing.T, table *gpt.Table) {
				t.Helper()

				// allocate 4 1G partitions first, and delete two in the middle

				assertAllocated(t, 1)(table.AllocatePartition(
					1*GiB, "1G1", partType1,
					gpt.WithUniqueGUID(uuid.MustParse("DA66737E-1ED4-4DDF-B98C-70CEBFE3ADA0")),
				))
				assertAllocated(t, 2)(table.AllocatePartition(1*GiB, "1G2", partType1))
				assertAllocated(t, 3)(table.AllocatePartition(1*GiB, "1G3", partType1))
				assertAllocated(t, 4)(table.AllocatePartition(
					1*GiB, "1G4", partType2,
					gpt.WithUniqueGUID(uuid.MustParse("3D0FE86B-7791-4659-B564-FC49A542866D")),
				))

				require.NoError(t, table.DeletePartition(1))
				require.NoError(t, table.DeletePartition(2))

				// gap is 2 GiB, while the tail available space is < 2 GiB, so small partitions will be appended to the end
				assertAllocated(t, 5)(table.AllocatePartition(
					200*MiB, "200M", partType2,
					gpt.WithUniqueGUID(uuid.MustParse("EE1A711E-DE12-4D9F-98FF-672F7AD638F8")),
				))
				assertAllocated(t, 6)(table.AllocatePartition(
					400*MiB, "400M", partType2,
					gpt.WithUniqueGUID(uuid.MustParse("15E609C8-9775-4E86-AF59-8A87E7C03FAB")),
				))

				// bigger partition will fill the gap
				assertAllocated(t, 2)(table.AllocatePartition(
					1500*MiB, "1500M", partType2,
					gpt.WithUniqueGUID(uuid.MustParse("15E609C8-9775-4E86-AF59-8A87E7C03FAC")),
				))
			},

			expectedSfdiskDump: loadTestdata(t, "mix-allocate.sfdisk"),
			expectedGdiskDump:  loadTestdata(t, "mix-allocate.gdisk"),
		},
		{
			name:     "resize",
			diskSize: 6 * GiB,
			opts: []gpt.Option{
				gpt.WithDiskGUID(uuid.MustParse("B6D003E5-7D1D-45E3-9F4B-4A2430B46D4A")),
			},
			allocator: func(t *testing.T, table *gpt.Table) {
				t.Helper()

				// allocate 2 1G partitions first, and grow the last one
				assertAllocated(t, 1)(table.AllocatePartition(
					1*GiB, "1G", partType1,
					gpt.WithUniqueGUID(uuid.MustParse("DA66737E-1ED4-4DDF-B98C-70CEBFE3ADA0")),
				))
				assertAllocated(t, 2)(table.AllocatePartition(
					1*GiB, "GROW", partType2,
					gpt.WithUniqueGUID(uuid.MustParse("3D0FE86B-7791-4659-B564-FC49A542866D")),
				))

				// attempt to grow the first one
				growth, err := table.AvailablePartitionGrowth(0)
				require.NoError(t, err)

				assert.EqualValues(t, 0, growth)

				// grow the second one
				growth, err = table.AvailablePartitionGrowth(1)
				require.NoError(t, err)

				assert.EqualValues(t, 4*GiB-(2048+2048)*512, growth)

				require.NoError(t, table.GrowPartition(1, growth))
			},

			expectedSfdiskDump: loadTestdata(t, "grow.sfdisk"),
			expectedGdiskDump:  loadTestdata(t, "grow.gdisk"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			rawImage := filepath.Join(tmpDir, "image.raw")

			f, err := os.Create(rawImage)
			require.NoError(t, err)

			require.NoError(t, f.Truncate(int64(test.diskSize)))
			require.NoError(t, f.Close())

			loDev := losetupAttachHelper(t, rawImage, false)

			t.Cleanup(func() {
				assert.NoError(t, loDev.Detach())
			})

			disk, err := os.OpenFile(loDev.Path(), os.O_RDWR, 0)
			require.NoError(t, err)

			t.Cleanup(func() {
				assert.NoError(t, disk.Close())
			})

			blkdev := block.NewFromFile(disk)

			gptdev, err := gpt.DeviceFromBlockDevice(blkdev)
			require.NoError(t, err)

			table, err := gpt.New(gptdev, test.opts...)
			require.NoError(t, err)

			assert.EqualValues(t, test.diskSize-(2048+2048)*512, table.LargestContiguousAllocatable())

			if test.allocator != nil {
				test.allocator(t, table)
			}

			require.NoError(t, table.Write())

			if test.expectedSfdiskDump != "" {
				assert.Equal(t, test.expectedSfdiskDump, sfdiskDump(t, loDev.Path()))
			}

			if test.expectedGdiskDump != "" {
				assert.Equal(t, test.expectedGdiskDump, gdiskDump(t, loDev.Path()))
			}

			// re-read the table and check if it's the same
			table2, err := gpt.Read(gptdev, test.opts...)
			require.NoError(t, err)

			assert.Equal(t, table.Partitions(), table2.Partitions())

			// re-write the partition table
			require.NoError(t, table2.Write())

			if test.expectedSfdiskDump != "" {
				assert.Equal(t, test.expectedSfdiskDump, sfdiskDump(t, loDev.Path()))
			}

			if test.expectedGdiskDump != "" {
				assert.Equal(t, test.expectedGdiskDump, gdiskDump(t, loDev.Path()))
			}
		})

		t.Run(test.name+"-file", func(t *testing.T) {
			tmpDir := t.TempDir()

			rawImage := filepath.Join(tmpDir, "image.raw")

			f, err := os.Create(rawImage)
			require.NoError(t, err)

			require.NoError(t, f.Truncate(int64(test.diskSize)))

			gptdev, err := gpt.DeviceFromFile(f)
			require.NoError(t, err)

			table, err := gpt.New(gptdev, test.opts...)
			require.NoError(t, err)

			assert.EqualValues(t, test.diskSize-(2048+2048)*512, table.LargestContiguousAllocatable())

			if test.allocator != nil {
				test.allocator(t, table)
			}

			require.NoError(t, table.Write())

			// re-read the table and check if it's the same
			table2, err := gpt.Read(gptdev, test.opts...)
			require.NoError(t, err)

			assert.Equal(t, table.Partitions(), table2.Partitions())

			// re-write the partition table
			require.NoError(t, table2.Write())

			// read once more to verify consistency
			table3, err := gpt.Read(gptdev, test.opts...)
			require.NoError(t, err)

			assert.Equal(t, table.Partitions(), table3.Partitions())

			require.NoError(t, f.Close())
		})
	}
}

func TestGPTOverwrite(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test requires root privileges")
	}

	if hostname, _ := os.Hostname(); hostname == "buildkitsandbox" { //nolint: errcheck
		t.Skip("test not supported under buildkit as partition devices are not propagated from /dev")
	}

	partType1 := uuid.MustParse("C12A7328-F81F-11D2-BA4B-00A0C93EC93B")
	partType2 := uuid.MustParse("E6D6D379-F507-44C2-A23C-238F2A3DF928")

	// create a partition table, and then overwrite it with a new one with incompatible layout
	tmpDir := t.TempDir()

	rawImage := filepath.Join(tmpDir, "image.raw")

	f, err := os.Create(rawImage)
	require.NoError(t, err)

	require.NoError(t, f.Truncate(int64(3*GiB)))
	require.NoError(t, f.Close())

	loDev := losetupAttachHelper(t, rawImage, false)

	t.Cleanup(func() {
		assert.NoError(t, loDev.Detach())
	})

	disk, err := os.OpenFile(loDev.Path(), os.O_RDWR, 0)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, disk.Close())
	})

	blkdev := block.NewFromFile(disk)

	gptdev, err := gpt.DeviceFromBlockDevice(blkdev)
	require.NoError(t, err)

	table, err := gpt.New(gptdev)
	require.NoError(t, err)

	// allocate 2 1G partitions first
	assertAllocated(t, 1)(table.AllocatePartition(100*MiB, "1G", partType1))
	assertAllocated(t, 2)(table.AllocatePartition(1*GiB, "2G", partType2))

	require.NoError(t, table.Write())

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.FileExists(c, loDev.Path()+"p1")
		assert.FileExists(c, loDev.Path()+"p2")
	}, 5*time.Second, 100*time.Millisecond)

	// now attempt to overwrite the partition table with a new one with different layout
	table2, err := gpt.New(gptdev)
	require.NoError(t, err)

	// allocate new partitions first
	assertAllocated(t, 1)(table2.AllocatePartition(600*MiB, "1P", partType1))
	assertAllocated(t, 2)(table2.AllocatePartition(600*MiB, "2P", partType2))
	assertAllocated(t, 3)(table2.AllocatePartition(600*MiB, "3P", partType2))

	require.NoError(t, table2.Write())

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.FileExists(c, loDev.Path()+"p1")
		assert.FileExists(c, loDev.Path()+"p2")
		assert.FileExists(c, loDev.Path()+"p3")
	}, 5*time.Second, 100*time.Millisecond)
}

func TestGPTDifferentIOSize(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test requires root privileges")
	}

	tmpDir := t.TempDir()

	rawImage := filepath.Join(tmpDir, "image.raw")

	f, err := os.Create(rawImage)
	require.NoError(t, err)

	require.NoError(t, f.Truncate(2*GiB))
	require.NoError(t, f.Close())

	loDev := losetupAttachHelper(t, rawImage, false)

	t.Cleanup(func() {
		assert.NoError(t, loDev.Detach())
	})

	disk, err := os.OpenFile(loDev.Path(), os.O_RDWR, 0)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, disk.Close())
	})

	blkdev := block.NewFromFile(disk)

	gptdev, err := gpt.DeviceFromBlockDevice(blkdev)
	require.NoError(t, err)

	table, err := gpt.New(gptdev)
	require.NoError(t, err)

	partType1 := uuid.MustParse("C12A7328-F81F-11D2-BA4B-00A0C93EC93B")

	// allocate two partitions
	assertAllocated(t, 1)(table.AllocatePartition(1*MiB, "1M", partType1))
	assertAllocated(t, 2)(table.AllocatePartition(1*GiB, "1G", partType1))

	require.NoError(t, table.Write())

	// now wrap the gpt devices with a new IO size
	ioSizeWrapper := &ioSizeWrapper{
		Device: gptdev,
		ioSize: 2 * MiB,
	}

	table2, err := gpt.Read(ioSizeWrapper)
	require.NoError(t, err)

	// the partition table should be the same even if the IO size is different
	assert.Equal(t, table.Partitions(), table2.Partitions())
}

func TestGPTSkipBlocks(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test requires root privileges")
	}

	tmpDir := t.TempDir()

	rawImage := filepath.Join(tmpDir, "image.raw")

	f, err := os.Create(rawImage)
	require.NoError(t, err)

	require.NoError(t, f.Truncate(2*GiB))
	require.NoError(t, f.Close())

	loDev := losetupAttachHelper(t, rawImage, false)

	t.Cleanup(func() {
		assert.NoError(t, loDev.Detach())
	})

	disk, err := os.OpenFile(loDev.Path(), os.O_RDWR, 0)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, disk.Close())
	})

	blkdev := block.NewFromFile(disk)

	gptdev, err := gpt.DeviceFromBlockDevice(blkdev)
	require.NoError(t, err)

	table, err := gpt.New(gptdev, gpt.WithSkipLBAs(10240))
	require.NoError(t, err)

	partType1 := uuid.MustParse("C12A7328-F81F-11D2-BA4B-00A0C93EC93B")

	// allocate partition spanning whole disk
	assertAllocated(t, 1)(table.AllocatePartition(table.LargestContiguousAllocatable(), "1G", partType1))

	assert.EqualValues(t, 0, table.LargestContiguousAllocatable())

	require.NoError(t, table.Write())

	table2, err := gpt.Read(gptdev)
	require.NoError(t, err)

	// if we read the table back, it should honor first LBA skip
	assert.Equal(t, table2.LargestContiguousAllocatable(), table.LargestContiguousAllocatable())
}

type ioSizeWrapper struct {
	gpt.Device

	ioSize uint
}

func (w *ioSizeWrapper) GetIOSize() (uint, error) {
	return w.ioSize, nil
}

//nolint:unparam
func losetupAttachHelper(t *testing.T, rawImage string, readonly bool) losetup.Device {
	t.Helper()

	for range 10 {
		loDev, err := losetup.Attach(rawImage, 0, readonly)
		if err != nil {
			if errors.Is(err, unix.EBUSY) {
				spraySleep := max(randv2.ExpFloat64(), 2.0)

				t.Logf("retrying after %v seconds", spraySleep)

				time.Sleep(time.Duration(spraySleep * float64(time.Second)))

				continue
			}
		}

		require.NoError(t, err)

		return loDev
	}

	t.Fatal("failed to attach loop device") //nolint:revive

	panic("unreachable")
}

func TestGPTFileOverwrite(t *testing.T) {
	partType1 := uuid.MustParse("C12A7328-F81F-11D2-BA4B-00A0C93EC93B")
	partType2 := uuid.MustParse("E6D6D379-F507-44C2-A23C-238F2A3DF928")

	// create a partition table, and then overwrite it with a new one with incompatible layout
	tmpDir := t.TempDir()

	rawImage := filepath.Join(tmpDir, "image.raw")

	f, err := os.Create(rawImage)
	require.NoError(t, err)

	require.NoError(t, f.Truncate(int64(3*GiB)))

	gptdev, err := gpt.DeviceFromFile(f)
	require.NoError(t, err)

	table, err := gpt.New(gptdev)
	require.NoError(t, err)

	// allocate 2 partitions first
	assertAllocated(t, 1)(table.AllocatePartition(100*MiB, "1G", partType1))
	assertAllocated(t, 2)(table.AllocatePartition(1*GiB, "2G", partType2))

	require.NoError(t, table.Write())

	// now attempt to overwrite the partition table with a new one with different layout
	table2, err := gpt.New(gptdev)
	require.NoError(t, err)

	// allocate new partitions first
	assertAllocated(t, 1)(table2.AllocatePartition(600*MiB, "1P", partType1))
	assertAllocated(t, 2)(table2.AllocatePartition(600*MiB, "2P", partType2))
	assertAllocated(t, 3)(table2.AllocatePartition(600*MiB, "3P", partType2))

	require.NoError(t, table2.Write())

	// verify the new layout
	table3, err := gpt.Read(gptdev)
	require.NoError(t, err)

	assert.Equal(t, table2.Partitions(), table3.Partitions())
	assert.Len(t, table3.Partitions(), 3)

	require.NoError(t, f.Close())
}

func TestGPTFileDifferentIOSize(t *testing.T) {
	tmpDir := t.TempDir()

	rawImage := filepath.Join(tmpDir, "image.raw")

	f, err := os.Create(rawImage)
	require.NoError(t, err)

	require.NoError(t, f.Truncate(2*GiB))

	gptdev, err := gpt.DeviceFromFile(f)
	require.NoError(t, err)

	table, err := gpt.New(gptdev)
	require.NoError(t, err)

	partType1 := uuid.MustParse("C12A7328-F81F-11D2-BA4B-00A0C93EC93B")

	// allocate two partitions
	assertAllocated(t, 1)(table.AllocatePartition(1*MiB, "1M", partType1))
	assertAllocated(t, 2)(table.AllocatePartition(1*GiB, "1G", partType1))

	require.NoError(t, table.Write())

	// now wrap the gpt devices with a new IO size
	ioSizeWrapper := &ioSizeWrapper{
		Device: gptdev,
		ioSize: 2 * MiB,
	}

	table2, err := gpt.Read(ioSizeWrapper)
	require.NoError(t, err)

	// the partition table should be the same even if the IO size is different
	assert.Equal(t, table.Partitions(), table2.Partitions())

	require.NoError(t, f.Close())
}

func TestGPTFileSkipBlocks(t *testing.T) {
	tmpDir := t.TempDir()

	rawImage := filepath.Join(tmpDir, "image.raw")

	f, err := os.Create(rawImage)
	require.NoError(t, err)

	require.NoError(t, f.Truncate(2*GiB))

	gptdev, err := gpt.DeviceFromFile(f)
	require.NoError(t, err)

	table, err := gpt.New(gptdev, gpt.WithSkipLBAs(10240))
	require.NoError(t, err)

	partType1 := uuid.MustParse("C12A7328-F81F-11D2-BA4B-00A0C93EC93B")

	// allocate partition spanning whole disk
	assertAllocated(t, 1)(table.AllocatePartition(table.LargestContiguousAllocatable(), "1G", partType1))

	assert.EqualValues(t, 0, table.LargestContiguousAllocatable())

	require.NoError(t, table.Write())

	table2, err := gpt.Read(gptdev)
	require.NoError(t, err)

	// if we read the table back, it should honor first LBA skip
	assert.Equal(t, table2.LargestContiguousAllocatable(), table.LargestContiguousAllocatable())

	require.NoError(t, f.Close())
}

// createTestData creates a temporary directory with test data for filesystem images.
// Returns the directory path containing the test data structure (bin/ and etc/ directories with a hosts file).
func createTestData(t *testing.T) string {
	t.Helper()

	testData := filepath.Join(t.TempDir(), "test-data")
	require.NoError(t, os.MkdirAll(filepath.Join(testData, "etc"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(testData, "bin"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(testData, "etc", "hosts"),
		[]byte("127.0.0.1 localhost\n"),
		0o644,
	))

	// Set deterministic timestamps for reproducibility (epoch 0)
	epoch := time.Unix(0, 0)
	require.NoError(t, os.Chtimes(filepath.Join(testData, "etc", "hosts"), epoch, epoch))
	require.NoError(t, os.Chtimes(filepath.Join(testData, "etc"), epoch, epoch))
	require.NoError(t, os.Chtimes(filepath.Join(testData, "bin"), epoch, epoch))
	require.NoError(t, os.Chtimes(testData, epoch, epoch))

	return testData
}

// createVFATImage creates a 1MB VFAT filesystem image for testing.
func createVFATImage(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "vfat-1M.img")

	// Create 1MB image
	f, err := os.Create(imgPath)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(1*MiB))
	require.NoError(t, f.Close())

	// Format as VFAT with reproducible output
	// Use FAT32 with 4096 byte sectors to match systemd-repart defaults
	cmd := exec.CommandContext(t.Context(), "mkfs.vfat", "-I", "-F", "32", "-S", "4096", "-n", "TESTIMAGE", "--invariant", imgPath)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "mkfs.vfat failed: %s", output)

	cmd = exec.CommandContext(t.Context(), "mmd", "-i", imgPath, "::/bin", "::/etc")
	require.NoError(t, cmd.Run())

	// Copy test data from the test data directory
	testData := createTestData(t)

	cmd = exec.CommandContext(t.Context(), "mcopy", "-i", imgPath, filepath.Join(testData, "etc", "hosts"), "::/etc/hosts")
	require.NoError(t, cmd.Run())

	return imgPath
}

// createExt4Image creates a 32MB ext4 filesystem image for testing.
// Always creates 32MB to match systemd-repart's minimum ext4 size.
func createExt4Image(t *testing.T, label string, partUUID uuid.UUID) string {
	t.Helper()

	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "ext4.img")

	// Use the shared test data
	testData := createTestData(t)

	// Create 32MB ext4 image from directory with reproducible options
	// Extended options for reproducibility:
	// - hash_seed: deterministic directory hashing
	// - lazy_itable_init=0: disable lazy inode table initialization
	// - root_owner=0:0: set root owner to uid/gid 0
	extOpts := fmt.Sprintf("hash_seed=%s,lazy_itable_init=0,root_owner=0:0", partUUID.String())

	cmd := exec.CommandContext(
		t.Context(), "mkfs.ext4",
		"-b", "4096", // Block size to match systemd-repart
		"-m", "0", // 0% reserved blocks to match systemd-repart
		"-I", "256", // Inode size to match systemd-repart
		"-T", "default", // Filesystem type - default template sets bytes-per-inode ratio (results in 2048 inodes for 32MB)
		"-d", testData,
		"-L", label,
		"-U", partUUID.String(),
		"-E", extOpts,
		imgPath,
		"32M",
	)

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "mkfs.ext4 failed: %s", output)

	return imgPath
}

func TestGPTPartitionIO(t *testing.T) {
	diskGUID := uuid.MustParse("B6D003E5-7D1D-45E3-9F4B-4A2430B46D4A")
	partGUID1 := uuid.MustParse("DA66737E-1ED4-4DDF-B98C-70CEBFE3ADA0")
	partGUID2 := uuid.MustParse("3D0FE86B-7791-4659-B564-FC49A542866D")

	vfatImg := createVFATImage(t)
	ext4Img := createExt4Image(t, "TESTIMAGE", partGUID2)

	tmpDir := t.TempDir()
	rawImage := filepath.Join(tmpDir, "disk.raw")

	f, err := os.Create(rawImage)
	require.NoError(t, err)

	require.NoError(t, f.Truncate(50*MiB))

	gptdev, err := gpt.DeviceFromFile(f)
	require.NoError(t, err)

	table, err := gpt.New(gptdev, gpt.WithDiskGUID(diskGUID))
	require.NoError(t, err)

	partType := uuid.MustParse("EBD0A0A2-B9E5-4433-87C0-68B6B72699C7") // Microsoft Basic Data

	// Allocate partitions for the VFAT and ext4 images
	assertAllocated(t, 1)(table.AllocatePartition(1*MiB, "VFAT", partType, gpt.WithUniqueGUID(partGUID1)))
	assertAllocated(t, 2)(table.AllocatePartition(32*MiB, "EXT4", partType, gpt.WithUniqueGUID(partGUID2)))

	require.NoError(t, table.Write())

	// Write VFAT image to partition 1
	vfatFile, err := os.Open(vfatImg)
	require.NoError(t, err)

	defer vfatFile.Close() //nolint: errcheck

	w, size, err := table.PartitionWriter(0)
	require.NoError(t, err)

	assert.EqualValues(t, 1*MiB, size)

	n, err := io.Copy(w, vfatFile)
	require.NoError(t, err)

	assert.EqualValues(t, 1*MiB, n)

	// Write ext4 image to partition 2
	ext4File, err := os.Open(ext4Img)
	require.NoError(t, err)

	defer ext4File.Close() //nolint: errcheck

	w, size, err = table.PartitionWriter(1)
	require.NoError(t, err)

	assert.EqualValues(t, 32*MiB, size)

	n, err = io.Copy(w, ext4File)
	require.NoError(t, err)

	assert.EqualValues(t, 32*MiB, n)

	// Read VFAT partition back and verify
	readBackVfat := filepath.Join(tmpDir, "readback-vfat.img")
	readBackFile, err := os.Create(readBackVfat)
	require.NoError(t, err)

	r, err := table.PartitionReader(0)
	require.NoError(t, err)

	n, err = io.Copy(readBackFile, r)
	require.NoError(t, err)

	assert.EqualValues(t, 1*MiB, n)
	require.NoError(t, readBackFile.Close())

	// Compare VFAT checksums
	originalHash := sha256.New()
	_, err = vfatFile.Seek(0, 0)
	require.NoError(t, err)
	_, err = io.Copy(originalHash, vfatFile)
	require.NoError(t, err)

	readBackFile, err = os.Open(readBackVfat)
	require.NoError(t, err)

	defer readBackFile.Close() //nolint: errcheck

	readBackHash := sha256.New()
	_, err = io.Copy(readBackHash, readBackFile)
	require.NoError(t, err)

	assert.Equal(t, originalHash.Sum(nil), readBackHash.Sum(nil), "VFAT partition data should match original")

	// Read ext4 partition back and verify
	readBackExt4 := filepath.Join(tmpDir, "readback-ext4.img")
	readBackFile2, err := os.Create(readBackExt4)
	require.NoError(t, err)

	// PartitionReader returns an io.Reader for the partition content — copy
	// directly into the file so we can compare the bytes with the original image.
	r, err = table.PartitionReader(1)
	require.NoError(t, err)

	n, err = io.Copy(readBackFile2, r)
	require.NoError(t, err)

	assert.EqualValues(t, 32*MiB, n)
	require.NoError(t, readBackFile2.Close())

	// Compare ext4 checksums
	originalHashExt4 := sha256.New()
	_, err = ext4File.Seek(0, 0)
	require.NoError(t, err)
	_, err = io.Copy(originalHashExt4, ext4File)
	require.NoError(t, err)

	readBackFile2, err = os.Open(readBackExt4)
	require.NoError(t, err)

	defer readBackFile2.Close() //nolint: errcheck

	readBackHashExt4 := sha256.New()
	_, err = io.Copy(readBackHashExt4, readBackFile2)
	require.NoError(t, err)

	assert.Equal(t, originalHashExt4.Sum(nil), readBackHashExt4.Sum(nil), "ext4 partition data should match original")

	// Verify GPT structure with sfdisk
	actualSfdisk := sfdiskDump(t, rawImage)
	expectedSfdisk := loadTestdata(t, "partition-io.sfdisk")

	assert.Equal(t, expectedSfdisk, actualSfdisk)

	// Verify GPT structure with gdisk
	actualGdisk := gdiskDump(t, rawImage)
	expectedGdisk := loadTestdata(t, "partition-io.gdisk")

	assert.Equal(t, expectedGdisk, actualGdisk)

	require.NoError(t, f.Close())
}

// TestGPTPartitionIOSystemdRepart compares gpt partition I/O implementation
// with systemd-repart to ensure we produce compatible results.
func TestGPTPartitionIOSystemdRepart(t *testing.T) {
	// Check if systemd-repart is available
	if _, err := exec.LookPath("systemd-repart"); err != nil {
		t.Skip("systemd-repart not available, skipping integration test")
	}

	const fsSize = 100 * MiB

	tmpDir := t.TempDir()

	// for mtools reproducible builds
	// set SOURCE_DATE_EPOCH and TZ
	// to ensure timestamps are consistent
	t.Setenv("SOURCE_DATE_EPOCH", "0")
	t.Setenv("TZ", "UTC")

	// Create test data directory that will be used by both our implementation and systemd-repart
	testData := createTestData(t)

	// Create systemd-repart definition files
	defsDir := filepath.Join(tmpDir, "repart.d")
	require.NoError(t, os.MkdirAll(defsDir, 0o755))

	// VFAT partition definition
	vfatDef := `[Partition]
Type=esp
Label=VFAT
UUID=DA66737E-1ED4-4DDF-B98C-70CEBFE3ADA0
SizeMinBytes=1M
SizeMaxBytes=1M
Format=vfat
CopyFiles=%s:/
`
	require.NoError(t, os.WriteFile(
		filepath.Join(defsDir, "10-vfat.conf"),
		fmt.Appendf(nil, vfatDef, testData),
		0o644,
	))

	// ext4 partition definition
	// Note: Format=ext4 raises the minimum size to the minimal ext4 filesystem size
	// (32MiB with 4KiB FS sector size on images). Set 32M explicitly to match systemd-repart.
	ext4Def := `[Partition]
Type=linux-generic
Label=EXT4
UUID=3D0FE86B-7791-4659-B564-FC49A542866D
SizeMinBytes=32M
SizeMaxBytes=32M
Format=ext4
CopyFiles=%s:/
`
	require.NoError(t, os.WriteFile(
		filepath.Join(defsDir, "20-ext4.conf"),
		fmt.Appendf(nil, ext4Def, testData),
		0o644,
	))

	// Set mkfs options to match what we use in createVFATImage
	// Force FAT32 with 4096 byte sectors to match systemd-repart's default behavior
	t.Setenv("SYSTEMD_REPART_MKFS_OPTIONS_VFAT", "-F 32 -S 4096 -n TESTIMAGE --invariant")

	ext4GUID := uuid.MustParse("3D0FE86B-7791-4659-B564-FC49A542866D")

	// Set mkfs options for ext4 with reproducibility options
	// - UUID: use partition UUID as filesystem UUID
	// - hash_seed: deterministic directory hashing
	// - lazy_itable_init=0: disable lazy inode table initialization
	// - root_owner=0:0: set root owner to uid/gid 0
	extOpts := fmt.Sprintf("hash_seed=%s,lazy_itable_init=0,root_owner=0:0", ext4GUID)
	t.Setenv("SYSTEMD_REPART_MKFS_OPTIONS_EXT4", fmt.Sprintf("-U %s -E %s", ext4GUID, extOpts))

	// Create systemd-repart image
	repartImage := filepath.Join(tmpDir, "repart.raw")
	// see https://github.com/systemd/systemd/blob/main/test/units/TEST-58-REPART.sh#L23
	seedVal := "750b6cd5c4ae4012a15e7be3c29e6a47" // Same seed as systemd tests

	// this is the disk GUID that will be derived from the seed value by systemd-repart
	// see https://github.com/systemd/systemd/blob/main/test/units/TEST-58-REPART.sh#L115
	diskGUID := uuid.MustParse("1D2CE291-7CCE-4F7D-BC83-FDB49AD74EBD")

	cmd := exec.CommandContext(
		t.Context(),
		"systemd-repart",
		"--definitions="+defsDir,
		"--empty=create",
		"--size="+fmt.Sprintf("%d", fsSize),
		"--seed="+seedVal,
		"--dry-run=no",
		"--offline=yes",
		repartImage,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("systemd-repart output:\n%s", output)
		require.NoError(t, err, "systemd-repart failed")
	}

	// Create our own image using the same approach
	gptImage := filepath.Join(tmpDir, "gpt.raw")
	f, err := os.Create(gptImage)
	require.NoError(t, err)

	defer f.Close() //nolint: errcheck

	require.NoError(t, f.Truncate(fsSize))

	gptdev, err := gpt.DeviceFromFile(f)
	require.NoError(t, err)

	// Use the derived disk GUID for byte-for-byte identical output with systemd-repart
	table, err := gpt.New(gptdev, gpt.WithDiskGUID(diskGUID))
	require.NoError(t, err)

	partTypeESP := uuid.MustParse("C12A7328-F81F-11D2-BA4B-00A0C93EC93B")   // EFI System Partition
	partTypeLinux := uuid.MustParse("0FC63DAF-8483-4772-8E79-3D69D8477DE4") // Linux filesystem
	vfatGUID := uuid.MustParse("DA66737E-1ED4-4DDF-B98C-70CEBFE3ADA0")

	// Allocate VFAT partition
	assertAllocated(t, 1)(table.AllocatePartition(1*MiB, "VFAT", partTypeESP, gpt.WithUniqueGUID(vfatGUID)))
	// Allocate ext4 partition at 32MiB to match systemd-repart minimal ext4 size
	assertAllocated(t, 2)(table.AllocatePartition(32*MiB, "EXT4", partTypeLinux, gpt.WithUniqueGUID(ext4GUID)))

	require.NoError(t, table.Write())

	// Create VFAT filesystem with same data as systemd-repart
	vfatImg := createVFATImage(t)
	vfatFile, err := os.Open(vfatImg)
	require.NoError(t, err)

	defer vfatFile.Close() //nolint: errcheck

	w, size, err := table.PartitionWriter(0)
	require.NoError(t, err)

	assert.EqualValues(t, 1*MiB, size)

	n, err := io.Copy(w, vfatFile)
	require.NoError(t, err)

	assert.EqualValues(t, 1*MiB, n)

	// Create ext4 filesystem with same data as systemd-repart (32MiB)
	ext4Img := createExt4Image(t, "EXT4", ext4GUID)
	ext4File, err := os.Open(ext4Img)
	require.NoError(t, err)

	defer ext4File.Close() //nolint: errcheck

	w, size, err = table.PartitionWriter(1)
	require.NoError(t, err)

	assert.EqualValues(t, 32*MiB, size)

	n, err = io.Copy(w, ext4File)
	require.NoError(t, err)

	assert.EqualValues(t, 32*MiB, n)

	require.NoError(t, f.Close())

	sfdiskDump(t, gptImage)
	sfdiskDump(t, repartImage)

	// Compare the two images using sha256 checksums
	gptFile, err := os.Open(gptImage)
	require.NoError(t, err)

	defer gptFile.Close() //nolint: errcheck

	repartFile, err := os.Open(repartImage)
	require.NoError(t, err)

	defer repartFile.Close() //nolint: errcheck

	gptHash := sha256.New()
	_, err = io.Copy(gptHash, gptFile)
	require.NoError(t, err)

	repartHash := sha256.New()
	_, err = io.Copy(repartHash, repartFile)
	require.NoError(t, err)

	assert.Equal(t, gptHash.Sum(nil), repartHash.Sum(nil), "Our GPT partition I/O output should match systemd-repart output")
}
