// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package block_test

import (
	"errors"
	"fmt"
	randv2 "math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/siderolabs/go-blockdevice/v2/block"
	"github.com/siderolabs/go-blockdevice/v2/devicemapper"
	"github.com/siderolabs/go-blockdevice/v2/partitioning/gpt"
)

// linuxFilesystemPartitionType is the GPT type of a Linux filesystem partition.
var linuxFilesystemPartitionType = uuid.MustParse("0FC63DAF-8483-4772-8E79-3D69D8477DE4")

// TestDeviceMapperGPT covers partitioning a device-mapper device.
//
// Device-mapper devices never get kernel partitions, so the partitions of the table are partition
// maps: device-mapper devices of their own, stacked on the device being partitioned.
//
//nolint:gocognit,maintidx
func TestDeviceMapperGPT(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("skipping test; must be root")
	}

	if _, err := os.Stat(devicemapper.ControlPath); err != nil {
		t.Skip("skipping test; device-mapper control device is not available")
	}

	const (
		imageSize     = 256 * MiB
		partitionSize = 32 * MiB
		numPartitions = 3
	)

	rawImage := filepath.Join(t.TempDir(), "image.raw")

	f, err := os.Create(rawImage)
	require.NoError(t, err)

	require.NoError(t, f.Truncate(int64(imageSize)))
	require.NoError(t, f.Close())

	loDev := losetupAttachHelper(t, rawImage, false)

	t.Cleanup(func() {
		assert.NoError(t, loDev.Detach())
	})

	control, err := devicemapper.NewControl()
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, control.Close())
	})

	// a linear map over the whole loop device, standing in for a multipath map over a LUN
	suffix := fmt.Sprintf("%08x", randv2.Uint32())

	parent := devicemapper.DeviceID{
		Name: "gbd-test-" + suffix,
		UUID: "mpath-3600508" + suffix,
	}

	var loStat unix.Stat_t

	require.NoError(t, unix.Stat(loDev.Path(), &loStat))

	require.NoError(t, control.CreateDevice(parent, []devicemapper.Target{
		{
			Type:   "linear",
			Params: fmt.Sprintf("%d:%d 0", unix.Major(loStat.Rdev), unix.Minor(loStat.Rdev)),
			Length: imageSize / devicemapper.SectorSize,
		},
	}))

	t.Cleanup(func() {
		assert.NoError(t, retryWhileBusy(t, func() error {
			return control.RemoveDevice(devicemapper.ByUUID(parent.UUID))
		}))
	})

	parentDevNo := dmDevNoHelper(t, control, parent.Name)
	parentDevName := fmt.Sprintf("dm-%d", unix.Minor(parentDevNo))

	dev, err := block.NewFromPath(mknodHelper(t, parentDevNo), block.OpenForWrite())
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, dev.Close())
	})

	// the device node was made by hand, so make sure it is the map before relying on it
	devNo, err := dev.GetDevNo()
	require.NoError(t, err)
	require.Equal(t, parentDevNo, devNo)

	gptdev, err := gpt.DeviceFromBlockDevice(dev)
	require.NoError(t, err)

	table, err := gpt.New(gptdev)
	require.NoError(t, err)

	for _, name := range []string{"EFI", "META", "STATE"} {
		_, _, err = table.AllocatePartition(partitionSize, name, linuxFilesystemPartitionType)
		require.NoError(t, err)
	}

	require.NoError(t, table.Write())

	// the partition maps hold the parent map open, so they have to be removed before it is
	t.Cleanup(func() {
		for no := 1; no <= numPartitions; no++ {
			removeErr := retryWhileBusy(t, func() error {
				return control.RemoveDevice(devicemapper.ByUUID(fmt.Sprintf("part%d-%s", no, parent.UUID)))
			})
			if removeErr != nil && !errors.Is(removeErr, unix.ENXIO) {
				assert.NoError(t, removeErr)
			}
		}
	})

	partitionDevices, err := dev.GetPartitionDevices()
	require.NoError(t, err)

	t.Run("a partition map per partition", func(t *testing.T) {
		require.Len(t, partitionDevices, numPartitions)

		for no, partition := range table.Partitions() {
			partNo := no + 1

			devName, ok := partitionDevices[uint(partNo)]
			require.True(t, ok, "no partition map for partition %d", partNo)

			sysFsPath := filepath.Join("/sys/block", devName)

			// the map carries the identity kpartx gives it, which is what makes it resolvable
			assert.Equal(t, fmt.Sprintf("%s-part%d", parent.Name, partNo), readSysFs(t, sysFsPath, "dm", "name"))
			assert.Equal(t, fmt.Sprintf("part%d-%s", partNo, parent.UUID), readSysFs(t, sysFsPath, "dm", "uuid"))

			// it covers exactly the partition, and is stacked on the device being partitioned
			expectedSectors := (partition.LastLBA - partition.FirstLBA + 1) * uint64(dev.GetSectorSize()) / devicemapper.SectorSize
			assert.Equal(t, strconv.FormatUint(expectedSectors, 10), readSysFs(t, sysFsPath, "size"))
			assert.Equal(t, []string{parentDevName}, readSysFsDir(t, filepath.Join(sysFsPath, "slaves")))
		}
	})

	t.Run("partitions resolve to the maps", func(t *testing.T) {
		for partNo, devName := range partitionDevices {
			partitionDevName, err := dev.GetPartitionDevName(partNo)
			require.NoError(t, err)

			assert.Equal(t, filepath.Join("/dev", devName), partitionDevName)
		}

		_, err := dev.GetPartitionDevName(numPartitions + 1)
		assert.ErrorIs(t, err, block.ErrPartitionNotFound)

		lastPartNum, err := dev.GetKernelLastPartitionNum()
		require.NoError(t, err)

		assert.Equal(t, numPartitions, lastPartNum)
	})

	t.Run("a partition map is not a whole disk", func(t *testing.T) {
		isWholeDisk, err := dev.IsWholeDisk()
		require.NoError(t, err)

		assert.True(t, isWholeDisk)

		partDev, err := block.NewFromPath(mknodHelper(t, dmDevNoHelper(t, control, parent.Name+"-part1")))
		require.NoError(t, err)

		t.Cleanup(func() {
			assert.NoError(t, partDev.Close())
		})

		isWholeDisk, err = partDev.IsWholeDisk()
		require.NoError(t, err)

		assert.False(t, isWholeDisk)

		// a partition map has no partitions of its own
		nested, err := partDev.GetPartitionDevices()
		require.NoError(t, err)

		assert.Empty(t, nested)
	})

	t.Run("resizing a partition reloads the map", func(t *testing.T) {
		partition := table.Partitions()[numPartitions-1]

		start := partition.FirstLBA * uint64(dev.GetSectorSize())
		length := (partition.LastLBA - partition.FirstLBA + 1) * uint64(dev.GetSectorSize()) / 2

		require.NoError(t, dev.KernelPartitionResize(numPartitions, start, length))

		sysFsPath := filepath.Join("/sys/block", partitionDevices[numPartitions])

		assert.Equal(t, strconv.FormatUint(length/devicemapper.SectorSize, 10), readSysFs(t, sysFsPath, "size"))
	})

	t.Run("deleting a partition removes the map", func(t *testing.T) {
		require.NoError(t, table.DeletePartition(1))
		require.NoError(t, retryWhileBusy(t, table.Write))

		remaining, err := dev.GetPartitionDevices()
		require.NoError(t, err)

		assert.NotContains(t, remaining, uint(2))
		assert.Contains(t, remaining, uint(1))
		assert.Contains(t, remaining, uint(3))

		// the map of the partition which is gone is gone from the kernel too
		err = control.RemoveDevice(devicemapper.ByUUID(fmt.Sprintf("part2-%s", parent.UUID)))
		assert.ErrorIs(t, err, unix.ENXIO)
	})

	// the round trip a reboot puts a device-mapper disk through: the maps go away with the devices
	// they are stacked on, and nothing in the kernel brings them back from the table on disk
	t.Run("the maps are removed and recreated from the table", func(t *testing.T) {
		var partitionMaps []block.PartitionMap

		for idx, partition := range table.Partitions() {
			if partition == nil {
				continue
			}

			partitionMaps = append(partitionMaps, block.PartitionMap{
				Number: uint(idx + 1),
				Offset: partition.FirstLBA * uint64(dev.GetSectorSize()),
				Size:   (partition.LastLBA - partition.FirstLBA + 1) * uint64(dev.GetSectorSize()),
			})
		}

		require.NotEmpty(t, partitionMaps)

		require.NoError(t, retryWhileBusy(t, func() error { return dev.SyncPartitionMaps(nil) }))

		remaining, err := dev.GetPartitionDevices()
		require.NoError(t, err)

		assert.Empty(t, remaining)

		// with the maps gone, there is nothing left to remove
		require.NoError(t, dev.SyncPartitionMaps(nil))

		require.NoError(t, dev.SyncPartitionMaps(partitionMaps))

		recreated, err := dev.GetPartitionDevices()
		require.NoError(t, err)

		require.Len(t, recreated, len(partitionMaps))

		for _, partitionMap := range partitionMaps {
			devName, ok := recreated[partitionMap.Number]
			require.True(t, ok, "no partition map for partition %d", partitionMap.Number)

			sysFsPath := filepath.Join("/sys/block", devName)

			// the map carries the identity it had before, so it is the same partition to everything
			// which resolves it
			assert.Equal(t, fmt.Sprintf("part%d-%s", partitionMap.Number, parent.UUID), readSysFs(t, sysFsPath, "dm", "uuid"))
			assert.Equal(t, fmt.Sprintf("%s-part%d", parent.Name, partitionMap.Number), readSysFs(t, sysFsPath, "dm", "name"))
			assert.Equal(t, strconv.FormatUint(partitionMap.Size/devicemapper.SectorSize, 10), readSysFs(t, sysFsPath, "size"))
		}

		// syncing what is already there changes nothing
		require.NoError(t, dev.SyncPartitionMaps(partitionMaps))

		again, err := dev.GetPartitionDevices()
		require.NoError(t, err)

		assert.Equal(t, recreated, again)
	})
}

// dmDevNoHelper returns the device number of a device-mapper device.
func dmDevNoHelper(t *testing.T, control *devicemapper.Control, name string) uint64 {
	t.Helper()

	devices, err := control.ListDevices()
	require.NoError(t, err)

	for _, device := range devices {
		if device.Name == name {
			return device.DevNo
		}
	}

	require.FailNow(t, "device not found", "device-mapper device %q does not exist", name)

	panic("unreachable")
}

// mknodHelper creates a block device node for the device number and returns its path.
//
// The device node of a device-mapper device is created by devtmpfs or by udev, neither of which is
// guaranteed in a test sandbox, so the test makes its own to open the device through. It goes into
// /dev, the one directory which is certain to carry device nodes, and is removed again on cleanup.
func mknodHelper(t *testing.T, devNo uint64) string {
	t.Helper()

	major, minor := uint64(unix.Major(devNo)), uint64(unix.Minor(devNo))

	path := fmt.Sprintf("/dev/gbd-test-%d-%d-%08x", major, minor, randv2.Uint32())

	// mknod(2) takes the device number in the kernel's packed encoding, which is not the layout
	// of a dev_t: the low 8 bits of the minor, then the major, then the rest of the minor
	packed := (minor & 0xff) | (major << 8) | ((minor &^ 0xff) << 12)

	require.NoError(t, unix.Mknod(path, unix.S_IFBLK|0o600, int(packed)))

	t.Cleanup(func() {
		assert.NoError(t, os.Remove(path))
	})

	return path
}

func readSysFs(t *testing.T, path ...string) string {
	t.Helper()

	contents, err := os.ReadFile(filepath.Join(path...))
	require.NoError(t, err)

	return strings.TrimSpace(string(contents))
}

func readSysFsDir(t *testing.T, path string) []string {
	t.Helper()

	entries, err := os.ReadDir(path)
	require.NoError(t, err)

	names := make([]string, 0, len(entries))

	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}

// TestDeviceMapperPartitionMapsHoldTheDisk covers the reason the partition maps of a device-mapper
// disk have to be removed before the disk is: they are stacked on it, and hold it open.
func TestDeviceMapperPartitionMapsHoldTheDisk(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("skipping test; must be root")
	}

	if _, err := os.Stat(devicemapper.ControlPath); err != nil {
		t.Skip("skipping test; device-mapper control device is not available")
	}

	const sectors = 2048

	control, err := devicemapper.NewControl()
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, control.Close())
	})

	suffix := fmt.Sprintf("%08x", randv2.Uint32())

	// a disk which needs no backing device of its own, standing in for a decrypted volume
	disk := devicemapper.DeviceID{
		Name: "gbd-test-" + suffix,
		UUID: "CRYPT-LUKS2-" + suffix + "-gbd-test",
	}

	require.NoError(t, control.CreateDevice(disk, []devicemapper.Target{{Type: "zero", Length: sectors}}))

	removed := false

	t.Cleanup(func() {
		if !removed {
			assert.NoError(t, retryWhileBusy(t, func() error {
				return control.RemoveDevice(devicemapper.ByUUID(disk.UUID))
			}))
		}
	})

	withDevice := func(f func(dev *block.Device)) {
		// the handle is closed before the disk is removed: an open handle holds the disk open just
		// as a map stacked on it does
		dev, openErr := block.NewFromPath(mknodHelper(t, dmDevNoHelper(t, control, disk.Name)))
		require.NoError(t, openErr)

		defer func() {
			assert.NoError(t, dev.Close())
		}()

		f(dev)
	}

	withDevice(func(dev *block.Device) {
		require.NoError(t, dev.SyncPartitionMaps([]block.PartitionMap{
			{Number: 1, Offset: 0, Size: sectors * devicemapper.SectorSize},
		}))
	})

	t.Cleanup(func() {
		removeErr := retryWhileBusy(t, func() error {
			return control.RemoveDevice(devicemapper.ByUUID("part1-" + disk.UUID))
		})
		if removeErr != nil && !errors.Is(removeErr, unix.ENXIO) {
			assert.NoError(t, removeErr)
		}
	})

	// the map is stacked on the disk, so the disk cannot go anywhere: this is what a close of the
	// encrypted volume underneath runs into
	err = control.RemoveDevice(devicemapper.ByUUID(disk.UUID))
	assert.ErrorIs(t, err, unix.EBUSY)

	// a map which is held open - by a mount of the partition, say - cannot be removed either, which
	// is the condition the caller of a teardown is expected to retry on
	held, err := block.NewFromPath(mknodHelper(t, dmDevNoHelper(t, control, disk.Name+"-part1")))
	require.NoError(t, err)

	withDevice(func(dev *block.Device) {
		assert.ErrorIs(t, dev.SyncPartitionMaps(nil), unix.EBUSY)
	})

	require.NoError(t, held.Close())

	withDevice(func(dev *block.Device) {
		require.NoError(t, retryWhileBusy(t, func() error { return dev.SyncPartitionMaps(nil) }))
	})

	// with the maps gone, the disk can be removed
	require.NoError(t, retryWhileBusy(t, func() error {
		return control.RemoveDevice(devicemapper.ByUUID(disk.UUID))
	}))

	removed = true
}

// TestDeviceMapperPartitionMapNameConflict covers a partition map whose name is taken by an
// unrelated device-mapper device: the kernel refuses to create it, and the partition must not be
// reported as synced when it is missing.
func TestDeviceMapperPartitionMapNameConflict(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("skipping test; must be root")
	}

	if _, err := os.Stat(devicemapper.ControlPath); err != nil {
		t.Skip("skipping test; device-mapper control device is not available")
	}

	const sectors = 2048

	control, err := devicemapper.NewControl()
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, control.Close())
	})

	suffix := fmt.Sprintf("%08x", randv2.Uint32())

	disk := devicemapper.DeviceID{
		Name: "gbd-test-" + suffix,
		UUID: "CRYPT-LUKS2-" + suffix + "-gbd-test",
	}

	require.NoError(t, control.CreateDevice(disk, []devicemapper.Target{{Type: "zero", Length: sectors}}))

	t.Cleanup(func() {
		assert.NoError(t, retryWhileBusy(t, func() error {
			return control.RemoveDevice(devicemapper.ByUUID(disk.UUID))
		}))
	})

	// an unrelated device which happens to carry the name of the partition map of the disk
	squatter := devicemapper.DeviceID{
		Name: disk.Name + "-part1",
		UUID: "gbd-test-squatter-" + suffix,
	}

	require.NoError(t, control.CreateDevice(squatter, []devicemapper.Target{{Type: "zero", Length: sectors}}))

	t.Cleanup(func() {
		assert.NoError(t, retryWhileBusy(t, func() error {
			return control.RemoveDevice(devicemapper.ByUUID(squatter.UUID))
		}))
	})

	dev, err := block.NewFromPath(mknodHelper(t, dmDevNoHelper(t, control, disk.Name)))
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, dev.Close())
	})

	err = dev.SyncPartitionMaps([]block.PartitionMap{
		{Number: 1, Offset: 0, Size: sectors * devicemapper.SectorSize},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, unix.EBUSY)

	partitionDevices, err := dev.GetPartitionDevices()
	require.NoError(t, err)

	assert.Empty(t, partitionDevices)
}

// retryWhileBusy runs an operation which removes device-mapper devices, retrying while the kernel
// reports one of them busy.
//
// A device-mapper device which was just created stays open for as long as whatever watches block
// devices on the host takes to probe it - udev, or the block subsystem of another Talos - and the
// kernel refuses to remove a device which is open. It is the race 'dmsetup remove --retry' exists
// for, and the reason the test helper further down passes that flag.
func retryWhileBusy(t *testing.T, op func() error) error {
	t.Helper()

	const (
		timeout = 10 * time.Second
		pause   = 50 * time.Millisecond
	)

	deadline := time.Now().Add(timeout)

	for {
		err := op()
		if !errors.Is(err, unix.EBUSY) || time.Now().After(deadline) {
			return err
		}

		time.Sleep(pause)
	}
}
