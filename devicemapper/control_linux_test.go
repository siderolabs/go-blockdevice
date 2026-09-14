// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package devicemapper_test

import (
	"errors"
	"fmt"
	randv2 "math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/freddierice/go-losetup/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/siderolabs/go-blockdevice/v2/devicemapper"
)

const MiB = 1024 * 1024

func TestControl(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("skipping test; must be root")
	}

	if _, err := os.Stat(devicemapper.ControlPath); err != nil {
		t.Skip("skipping test; device-mapper control device is not available")
	}

	const imageSize = 64 * MiB

	control, err := devicemapper.NewControl()
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, control.Close())
	})

	version, err := control.Version()
	require.NoError(t, err)

	t.Logf("device-mapper version %s", version)

	// the interface this package speaks; the kernel refuses any other major version
	assert.EqualValues(t, 4, version.Major)

	rawImage := filepath.Join(t.TempDir(), "image.raw")

	f, err := os.Create(rawImage)
	require.NoError(t, err)

	require.NoError(t, f.Truncate(int64(imageSize)))
	require.NoError(t, f.Close())

	loDev := losetupAttachHelper(t, rawImage)

	t.Cleanup(func() {
		assert.NoError(t, loDev.Detach())
	})

	// unique names so that concurrent test runs on the same host don't collide
	suffix := fmt.Sprintf("%08x", randv2.Uint32())

	id := devicemapper.DeviceID{
		Name: "gbd-test-" + suffix,
		UUID: "gbd-test-uuid-" + suffix,
	}

	major, minor := loDevNo(t, loDev)

	linear := func(sectors uint64) []devicemapper.Target {
		return []devicemapper.Target{
			{
				Type:        "linear",
				Params:      fmt.Sprintf("%d:%d 0", major, minor),
				SectorStart: 0,
				Length:      sectors,
			},
		}
	}

	require.NoError(t, control.CreateDevice(id, linear(imageSize/devicemapper.SectorSize)))

	removed := false

	t.Cleanup(func() {
		if !removed {
			assert.NoError(t, retryWhileBusy(t, func() error {
				return control.RemoveDevice(devicemapper.ByUUID(id.UUID))
			}))
		}
	})

	t.Run("list devices", func(t *testing.T) {
		devices, err := control.ListDevices()
		require.NoError(t, err)

		// the host may have device-mapper devices of its own, so look for ours
		assert.Contains(t, deviceNames(devices), id.Name)
	})

	sysFsPath := sysFsPathHelper(t, control, id.Name)

	t.Run("device is live with the table it was given", func(t *testing.T) {
		assert.Equal(t, id.Name, readSysFsFile(t, filepath.Join(sysFsPath, "dm", "name")))
		assert.Equal(t, id.UUID, readSysFsFile(t, filepath.Join(sysFsPath, "dm", "uuid")))
		assert.Equal(t, "0", readSysFsFile(t, filepath.Join(sysFsPath, "dm", "suspended")))

		// the size is in 512-byte sectors, which is what the table asked for
		assert.Equal(t, fmt.Sprintf("%d", imageSize/devicemapper.SectorSize), readSysFsFile(t, filepath.Join(sysFsPath, "size")))

		// the loop device is the only slave of the map
		assert.Equal(t, []string{filepath.Base(loDev.Path())}, readSysFsDir(t, filepath.Join(sysFsPath, "slaves")))
	})

	t.Run("reload replaces the table", func(t *testing.T) {
		require.NoError(t, control.ReloadDevice(devicemapper.ByName(id.Name), linear(imageSize/devicemapper.SectorSize/2)))

		assert.Equal(t, fmt.Sprintf("%d", imageSize/devicemapper.SectorSize/2), readSysFsFile(t, filepath.Join(sysFsPath, "size")))
		assert.Equal(t, "0", readSysFsFile(t, filepath.Join(sysFsPath, "dm", "suspended")))
	})

	t.Run("read-only device", func(t *testing.T) {
		readOnly := devicemapper.DeviceID{
			Name: id.Name + "-ro",
			UUID: id.UUID + "-ro",
		}

		require.NoError(t, control.CreateDevice(readOnly, linear(1024), devicemapper.WithReadOnly(true)))

		t.Cleanup(func() {
			assert.NoError(t, retryWhileBusy(t, func() error {
				return control.RemoveDevice(devicemapper.ByUUID(readOnly.UUID))
			}))
		})

		readOnlySysFsPath := sysFsPathHelper(t, control, readOnly.Name)

		assert.Equal(t, "1", readSysFsFile(t, filepath.Join(readOnlySysFsPath, "ro")))

		// the mode is a property of the table, so a reload has to re-assert it
		require.NoError(t, control.ReloadDevice(devicemapper.ByUUID(readOnly.UUID), linear(2048), devicemapper.WithReadOnly(true)))
		assert.Equal(t, "1", readSysFsFile(t, filepath.Join(readOnlySysFsPath, "ro")))

		// and a reload without it makes the device writable again
		require.NoError(t, control.ReloadDevice(devicemapper.ByUUID(readOnly.UUID), linear(2048)))
		assert.Equal(t, "0", readSysFsFile(t, filepath.Join(readOnlySysFsPath, "ro")))
	})

	t.Run("create with a duplicate name fails", func(t *testing.T) {
		err := control.CreateDevice(devicemapper.DeviceID{Name: id.Name, UUID: id.UUID + "-other"}, linear(1024))
		assert.ErrorIs(t, err, unix.EBUSY)
	})

	t.Run("create with a bad table leaves nothing behind", func(t *testing.T) {
		other := devicemapper.DeviceID{
			Name: id.Name + "-bad",
			UUID: id.UUID + "-bad",
		}

		err := control.CreateDevice(other, []devicemapper.Target{
			{Type: "linear", Params: "not-a-device", Length: 1024},
		})
		require.Error(t, err)

		devices, err := control.ListDevices()
		require.NoError(t, err)

		assert.NotContains(t, deviceNames(devices), other.Name)
	})

	t.Run("remove by UUID", func(t *testing.T) {
		require.NoError(t, retryWhileBusy(t, func() error {
			return control.RemoveDevice(devicemapper.ByUUID(id.UUID))
		}))

		removed = true

		devices, err := control.ListDevices()
		require.NoError(t, err)

		assert.NotContains(t, deviceNames(devices), id.Name)
	})

	t.Run("removing a device which is gone reports ENXIO", func(t *testing.T) {
		err := control.RemoveDevice(devicemapper.ByUUID(id.UUID))
		assert.ErrorIs(t, err, unix.ENXIO)

		err = control.RemoveDevice(devicemapper.ByName(id.Name))
		assert.ErrorIs(t, err, unix.ENXIO)
	})
}

func deviceNames(devices []devicemapper.DeviceInfo) []string {
	names := make([]string, 0, len(devices))

	for _, device := range devices {
		names = append(names, device.Name)
	}

	return names
}

// sysFsPathHelper returns the sysfs path of a device-mapper device, resolved from the device number
// the kernel reports for it.
func sysFsPathHelper(t *testing.T, control *devicemapper.Control, name string) string {
	t.Helper()

	devices, err := control.ListDevices()
	require.NoError(t, err)

	for _, device := range devices {
		if device.Name != name {
			continue
		}

		sysFsPath := fmt.Sprintf("/sys/dev/block/%d:%d", unix.Major(device.DevNo), unix.Minor(device.DevNo))

		_, err = os.Stat(sysFsPath)
		require.NoError(t, err, "sysfs path %q of device %q", sysFsPath, name)

		return sysFsPath
	}

	require.FailNow(t, "device not found", "device %q is not in the device-mapper device list", name)

	panic("unreachable")
}

func loDevNo(t *testing.T, loDev losetup.Device) (uint32, uint32) {
	t.Helper()

	var st unix.Stat_t

	require.NoError(t, unix.Stat(loDev.Path(), &st))

	return unix.Major(st.Rdev), unix.Minor(st.Rdev)
}

func readSysFsFile(t *testing.T, path string) string {
	t.Helper()

	contents, err := os.ReadFile(path)
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

func losetupAttachHelper(t *testing.T, rawImage string) losetup.Device {
	t.Helper()

	for range 10 {
		loDev, err := losetup.Attach(rawImage, 0, false)
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

// retryWhileBusy runs an operation which removes device-mapper devices, retrying while the kernel
// reports one of them busy.
//
// A device-mapper device which was just created stays open for as long as whatever watches block
// devices on the host takes to probe it - udev, or the block subsystem of another Talos - and the
// kernel refuses to remove a device which is open. It is the race 'dmsetup remove --retry' exists
// for.
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
