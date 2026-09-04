// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package block_test

import (
	"context"
	"errors"
	"fmt"
	randv2 "math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/freddierice/go-losetup/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/siderolabs/go-blockdevice/v2/block"
)

const (
	MiB = 1024 * 1024
	GiB = 1024 * MiB
)

//nolint:maintidx
func TestDevice(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("skipping test; must be root")
	}

	tmpDir := t.TempDir()

	rawImage := filepath.Join(tmpDir, "image.raw")

	f, err := os.Create(rawImage)
	require.NoError(t, err)

	require.NoError(t, f.Truncate(int64(2*GiB)))
	require.NoError(t, f.Close())

	loDev := losetupAttachHelper(t, rawImage, false)

	t.Cleanup(func() {
		assert.NoError(t, loDev.Detach())
	})

	devPath := loDev.Path()

	script := strings.TrimSpace(`
	label: gpt
	label-id: DDDA0816-8B53-47BF-A813-9EBB1F73AAA2
	size=      204800, type=C12A7328-F81F-11D2-BA4B-00A0C93EC93B, uuid=3C047FF8-E35C-4918-A061-B4C1E5A291E5, name="EFI"
	size=        2048, type=21686148-6449-6E6F-744E-656564454649, uuid=942D2017-052E-4216-B4E4-2110507E4CD4, name="BIOS", attrs="LegacyBIOSBootable"
	size=     2048000, type=0FC63DAF-8483-4772-8E79-3D69D8477DE4, uuid=E8516F6B-F03E-45AE-8D9D-9958456EE7E4, name="BOOT"
	size=        2048, type=0FC63DAF-8483-4772-8E79-3D69D8477DE4, uuid=CE6B2D56-7A70-4546-926C-7A9B41607347, name="META"
	size=      204800, type=0FC63DAF-8483-4772-8E79-3D69D8477DE4, uuid=7F5FCD6C-A703-40D2-8796-E5CF7F3A9EB5, name="STATE"
					   type=0FC63DAF-8483-4772-8E79-3D69D8477DE4, uuid=0F06E81A-E78D-426B-A078-30A01AAB3FB7, name="EPHEMERAL"
	`)

	cmd := exec.CommandContext(t.Context(), "sfdisk", devPath)
	cmd.Stdin = strings.NewReader(script)
	cmd.Stdout = t.Output()
	cmd.Stderr = t.Output()

	require.NoError(t, cmd.Run())

	cmd = exec.CommandContext(t.Context(), "partprobe", devPath)
	cmd.Stdout = t.Output()
	cmd.Stderr = t.Output()

	require.NoError(t, cmd.Run())

	devWhole, err := block.NewFromPath(devPath)
	require.NoError(t, err)

	devWhole2, err := block.NewFromPath(devPath)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, devWhole.Close())
		assert.NoError(t, devWhole2.Close())
	})

	loReadOnly := losetupAttachHelper(t, rawImage, true)

	t.Cleanup(func() {
		assert.NoError(t, loReadOnly.Detach())
	})

	t.Run("whole disk", func(t *testing.T) {
		if hostname, _ := os.Hostname(); hostname == "buildkitsandbox" { //nolint:errcheck
			t.Skip("test not supported under buildkit as partition devices are not propagated from /dev")
		}

		isWhole, err := devWhole.IsWholeDisk()
		require.NoError(t, err)

		assert.True(t, isWhole)

		devPartition, err := block.NewFromPath(devPath + "p3")
		require.NoError(t, err)

		t.Cleanup(func() {
			assert.NoError(t, devPartition.Close())
		})

		isWhole, err = devPartition.IsWholeDisk()
		require.NoError(t, err)

		assert.False(t, isWhole)

		partitionNum, err := devWhole.GetKernelLastPartitionNum()
		require.NoError(t, err)

		assert.Equal(t, 6, partitionNum)
	})

	t.Run("partition devices", func(t *testing.T) {
		if hostname, _ := os.Hostname(); hostname == "buildkitsandbox" { //nolint:errcheck
			t.Skip("test not supported under buildkit as partition devices are not propagated from /dev")
		}

		devName := filepath.Base(devPath)

		partitionDevices, err := devWhole.GetPartitionDevices()
		require.NoError(t, err)

		assert.Equal(t, map[uint]string{
			1: devName + "p1",
			2: devName + "p2",
			3: devName + "p3",
			4: devName + "p4",
			5: devName + "p5",
			6: devName + "p6",
		}, partitionDevices)

		partitionDevName, err := devWhole.GetPartitionDevName(3)
		require.NoError(t, err)

		assert.Equal(t, devPath+"p3", partitionDevName)

		_, err = devWhole.GetPartitionDevName(7)
		assert.ErrorIs(t, err, block.ErrPartitionNotFound)

		// a partition doesn't have partitions of its own
		devPartition, err := block.NewFromPath(devPath + "p3")
		require.NoError(t, err)

		t.Cleanup(func() {
			assert.NoError(t, devPartition.Close())
		})

		partitionDevices, err = devPartition.GetPartitionDevices()
		require.NoError(t, err)

		assert.Empty(t, partitionDevices)
	})

	t.Run("get whole disk", func(t *testing.T) {
		if hostname, _ := os.Hostname(); hostname == "buildkitsandbox" { //nolint:errcheck
			t.Skip("test not supported under buildkit as partition devices are not propagated from /dev")
		}

		devPartition, err := block.NewFromPath(devPath + "p3")
		require.NoError(t, err)

		t.Cleanup(func() {
			assert.NoError(t, devPartition.Close())
		})

		wholeDisk, err := devPartition.GetWholeDisk()
		require.NoError(t, err)

		devNoExpected, err := devWhole.GetDevNo()
		require.NoError(t, err)

		devNoActual, err := wholeDisk.GetDevNo()
		require.NoError(t, err)

		assert.Equal(t, devNoExpected, devNoActual)

		t.Cleanup(func() {
			assert.NoError(t, wholeDisk.Close())
		})

		wholeDiskSame, err := devPartition.GetWholeDisk()
		require.NoError(t, err)

		devNoActual, err = wholeDiskSame.GetDevNo()
		require.NoError(t, err)

		assert.Equal(t, devNoExpected, devNoActual)

		t.Cleanup(func() {
			assert.NoError(t, wholeDiskSame.Close())
		})
	})

	t.Run("is CD", func(t *testing.T) {
		isCD := devWhole.IsCD()
		assert.False(t, isCD)
	})

	t.Run("size", func(t *testing.T) {
		size, err := devWhole.GetSize()
		require.NoError(t, err)

		assert.EqualValues(t, 2*GiB, size)
	})

	t.Run("sector size", func(t *testing.T) {
		assert.EqualValues(t, 512, devWhole.GetSectorSize())

		ioSize, err := devWhole.GetIOSize()
		require.NoError(t, err)
		assert.EqualValues(t, 512, ioSize)
	})

	t.Run("private dm", func(t *testing.T) {
		privateDM, err := devWhole.IsPrivateDeviceMapper()
		require.NoError(t, err)
		assert.False(t, privateDM)
	})

	t.Run("lock unlock", func(t *testing.T) {
		require.NoError(t, devWhole.Lock(true))
		require.NoError(t, devWhole.Unlock())
	})

	t.Run("lock try lock unlock", func(t *testing.T) {
		require.NoError(t, devWhole.Lock(true))

		err := devWhole2.TryLock(false)
		require.Error(t, err)
		require.ErrorIs(t, err, unix.EWOULDBLOCK)

		require.NoError(t, devWhole.Unlock())

		require.NoError(t, devWhole2.TryLock(false))
		require.NoError(t, devWhole2.Unlock())
	})

	t.Run("retry lock", func(t *testing.T) {
		require.NoError(t, devWhole.Lock(true))

		errCh := make(chan error)

		go func() {
			errCh <- devWhole2.RetryLockWithTimeout(t.Context(), true, 10*time.Second)
		}()

		time.Sleep(1 * time.Second)

		require.NoError(t, devWhole.Unlock())

		require.NoError(t, <-errCh)
		require.NoError(t, devWhole2.Unlock())
	})

	t.Run("read only", func(t *testing.T) {
		readOnly, err := devWhole.IsReadOnly()
		require.NoError(t, err)

		assert.False(t, readOnly)

		devReadOnly, err := block.NewFromPath(loReadOnly.Path())
		require.NoError(t, err)

		t.Cleanup(func() {
			assert.NoError(t, devReadOnly.Close())
		})

		readOnly, err = devReadOnly.IsReadOnly()
		require.NoError(t, err)
		assert.True(t, readOnly)
	})

	t.Run("properties", func(t *testing.T) {
		props, err := devWhole.GetProperties()
		require.NoError(t, err)

		assert.Equal(t, "/virtual", props.BusPath)
		assert.Equal(t, "/sys/class/block", props.SubSystem)
		assert.False(t, props.Rotational)
	})
}

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

// TestDeviceMapperPartitions verifies that partition maps of a device-mapper device are resolved
// from sysfs: device-mapper devices never have kernel partitions, partitions are separate
// device-mapper devices carrying the UUID "part<N>-<parent UUID>".
func TestDeviceMapperPartitions(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("skipping test; must be root")
	}

	if _, err := exec.LookPath("dmsetup"); err != nil {
		t.Skip("skipping test; dmsetup is not available")
	}

	if _, err := os.Stat("/dev/mapper/control"); err != nil {
		t.Skip("skipping test; /dev/mapper/control is not available")
	}

	const (
		sectorSize = 512
		imageSize  = 64 * MiB
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

	// unique suffix so that concurrent test runs on the same host don't collide
	suffix := fmt.Sprintf("%08x", randv2.Uint32())

	parentName := "gbd-test-" + suffix
	parentUUID := "mpath-3600508" + suffix

	// the "disk": a linear map over the whole loop device
	parent := dmsetupCreateHelper(t, parentName, parentUUID,
		fmt.Sprintf("0 %d linear %s 0", imageSize/sectorSize, loDev.Path()))

	// two partition maps, in the shape kpartx creates them; the parent is referenced by major:minor,
	// as kpartx does, so that the kernel doesn't have to resolve a path for it
	part1 := dmsetupCreateHelper(t, parentName+"-part1", "part1-"+parentUUID,
		fmt.Sprintf("0 2048 linear %s 2048", parent.devNo))
	part3 := dmsetupCreateHelper(t, parentName+"-part3", "part3-"+parentUUID,
		fmt.Sprintf("0 4096 linear %s 8192", parent.devNo))

	// a device-mapper device stacked on the "disk" which is not a partition map
	dmsetupCreateHelper(t, parentName+"-lv", "LVM-"+suffix+"-lvol0",
		fmt.Sprintf("0 2048 linear %s 20480", parent.devNo))

	devParent, err := block.NewFromPath(parent.path)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, devParent.Close())
	})

	partitionDevices, err := devParent.GetPartitionDevices()
	require.NoError(t, err)

	assert.Equal(t, map[uint]string{
		1: part1.devName,
		3: part3.devName,
	}, partitionDevices)

	partitionDevName, err := devParent.GetPartitionDevName(3)
	require.NoError(t, err)

	assert.Equal(t, filepath.Join("/dev", part3.devName), partitionDevName)

	_, err = devParent.GetPartitionDevName(2)
	assert.ErrorIs(t, err, block.ErrPartitionNotFound)

	// a partition map doesn't have partitions of its own
	devPart1, err := block.NewFromPath(part1.path)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, devPart1.Close())
	})

	partitionDevices, err = devPart1.GetPartitionDevices()
	require.NoError(t, err)

	assert.Empty(t, partitionDevices)
}

// dmDevice is a device-mapper device created by dmsetupCreateHelper.
type dmDevice struct {
	// devName is the kernel device name, e.g. "dm-1".
	devName string
	// devNo is the device number as "major:minor", which a device-mapper table accepts in place of a path.
	devNo string
	// path is the /dev/mapper node of the device.
	path string
}

// dmsetupCreateHelper creates a device-mapper device, removing it on test cleanup.
//
// udev synchronization is disabled, as udev might not be running, and the device node is created
// explicitly with 'dmsetup mknodes': /dev/dm-<minor> is a devtmpfs node, which is not to be relied
// upon in a sandbox with a private /dev (a container, for one), where devices created after the
// sandbox was set up never show up.
func dmsetupCreateHelper(t *testing.T, name, uuid, table string) dmDevice {
	t.Helper()

	dmsetupHelper(t, "create", name, "--uuid", uuid, "--noudevsync", "--table", table)

	t.Cleanup(func() {
		cmd := exec.CommandContext(context.Background(), "dmsetup", "remove", "--noudevsync", "--retry", name)
		cmd.Stdout = t.Output()
		cmd.Stderr = t.Output()

		assert.NoError(t, cmd.Run())
	})

	dmsetupHelper(t, "mknodes", name)

	devName := "dm-" + dmsetupHelper(t, "info", "--columns", "--noheadings", "-o", "minor", name)

	devNo, err := os.ReadFile(filepath.Join("/sys/block", devName, "dev"))
	require.NoError(t, err)

	return dmDevice{
		devName: devName,
		devNo:   strings.TrimSpace(string(devNo)),
		path:    filepath.Join("/dev/mapper", name),
	}
}

// dmsetupHelper runs dmsetup with the given arguments, returning the trimmed standard output.
func dmsetupHelper(t *testing.T, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "dmsetup", args...)
	cmd.Stderr = t.Output()

	out, err := cmd.Output()
	require.NoError(t, err)

	return strings.TrimSpace(string(out))
}
