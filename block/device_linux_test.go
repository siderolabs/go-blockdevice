// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package block_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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

	var loDev losetup.Device

	loDev, err = losetup.Attach(rawImage, 0, false)
	require.NoError(t, err)

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

	cmd := exec.Command("sfdisk", devPath)
	cmd.Stdin = strings.NewReader(script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())

	cmd = exec.Command("partprobe", devPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())

	devWhole, err := block.NewFromPath(devPath)
	require.NoError(t, err)

	devWhole2, err := block.NewFromPath(devPath)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, devWhole.Close())
		assert.NoError(t, devWhole2.Close())
	})

	loReadOnly, err := losetup.Attach(rawImage, 0, true)
	require.NoError(t, err)

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
