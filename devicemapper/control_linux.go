// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package devicemapper

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/siderolabs/go-blockdevice/v2/internal/dmioctl"
)

// ControlPath is the path of the device-mapper control device.
const ControlPath = "/dev/mapper/control"

// Buffer sizes for the commands which have the kernel write a reply back into the payload.
const (
	listBufferSize    = 16 * 1024
	maxListBufferSize = 4 * 1024 * 1024
)

// Control is an open handle for the device-mapper control device.
type Control struct {
	f *os.File
}

// NewControl opens the device-mapper control device.
//
// The returned Control should be closed.
func NewControl() (*Control, error) {
	f, err := os.OpenFile(ControlPath, os.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open the device-mapper control device: %w", err)
	}

	return &Control{f: f}, nil
}

// Close the control device.
func (c *Control) Close() error {
	return c.f.Close()
}

// Version returns the version of the device-mapper driver in the kernel.
func (c *Control) Version() (Version, error) {
	payload, err := dmioctl.Payload{}.Build()
	if err != nil {
		return Version{}, err
	}

	if err = c.ioctl(dmioctl.VersionRequest, payload); err != nil {
		return Version{}, fmt.Errorf("failed to query the device-mapper version: %w", err)
	}

	header := dmioctl.Header(payload)

	return Version{
		Major: header.Get_version_major(),
		Minor: header.Get_version_minor(),
		Patch: header.Get_version_patch(),
	}, nil
}

// CreateDevice creates a device-mapper device, gives it the table and resumes it, so that the
// device is live when the call returns.
//
// If the table can't be loaded, the device is removed again rather than left behind without one.
func (c *Control) CreateDevice(id DeviceID, targets []Target, opts ...TableOption) error {
	if err := c.command(dmioctl.DevCreateRequest, id, 0, nil); err != nil {
		return fmt.Errorf("failed to create device-mapper device %s: %w", id, err)
	}

	if err := c.loadAndResume(id, targets, opts...); err != nil {
		if removeErr := c.RemoveDevice(id); removeErr != nil {
			return fmt.Errorf("%w (removing the device failed as well: %w)", err, removeErr)
		}

		return err
	}

	return nil
}

// ReloadDevice replaces the table of an existing device-mapper device and resumes it.
//
// The filesystem on the device, if any, is not frozen while the table is swapped.
func (c *Control) ReloadDevice(id DeviceID, targets []Target, opts ...TableOption) error {
	return c.loadAndResume(id, targets, opts...)
}

// RemoveDevice removes a device-mapper device.
//
// Removing a device which doesn't exist fails with unix.ENXIO, and removing one which is held open
// fails with unix.EBUSY.
func (c *Control) RemoveDevice(id DeviceID) error {
	if err := c.command(dmioctl.DevRemoveRequest, id.lookup(), 0, nil); err != nil {
		return fmt.Errorf("failed to remove device-mapper device %s: %w", id, err)
	}

	return nil
}

// ListDevices returns every device-mapper device in the kernel.
func (c *Control) ListDevices() ([]DeviceInfo, error) {
	for size := listBufferSize; size <= maxListBufferSize; size *= 2 {
		payload, err := dmioctl.Payload{MinSize: size}.Build()
		if err != nil {
			return nil, err
		}

		if err = c.ioctl(dmioctl.ListDevicesRequest, payload); err != nil {
			return nil, fmt.Errorf("failed to list device-mapper devices: %w", err)
		}

		if dmioctl.Header(payload).Get_flags()&dmioctl.BufferFullFlag != 0 {
			// the reply didn't fit, so try again with a bigger buffer
			continue
		}

		entries, err := dmioctl.ParseNameList(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to parse the device-mapper device list: %w", err)
		}

		devices := make([]DeviceInfo, 0, len(entries))

		for _, entry := range entries {
			devices = append(devices, DeviceInfo{
				Name:  entry.Name,
				DevNo: entry.Dev,
			})
		}

		return devices, nil
	}

	return nil, fmt.Errorf("failed to list device-mapper devices: the reply exceeds %d bytes", maxListBufferSize)
}

// loadAndResume loads a table as the inactive table of the device and then resumes the device,
// which is what makes the kernel swap the new table in.
func (c *Control) loadAndResume(id DeviceID, targets []Target, opts ...TableOption) error {
	var options tableOptions

	for _, opt := range opts {
		opt(&options)
	}

	if err := c.command(dmioctl.TableLoadRequest, id.lookup(), options.flags(), targets); err != nil {
		return fmt.Errorf("failed to load the table of device-mapper device %s: %w", id, err)
	}

	// a resume is DevSuspendRequest without the suspend flag; the kernel suspends the device
	// itself, if it isn't suspended already, before swapping the table in
	//
	// that implicit suspend freezes the filesystem on the device unless it is told not to, which
	// would block every writer to a mounted filesystem for the duration of the call; the tables
	// loaded here only remap sectors, so there is nothing for a freeze to make consistent
	if err := c.command(dmioctl.DevSuspendRequest, id.lookup(), dmioctl.SkipLockfsFlag, nil); err != nil {
		return fmt.Errorf("failed to resume device-mapper device %s: %w", id, err)
	}

	return nil
}

// command assembles a payload and runs one ioctl with it.
func (c *Control) command(request uintptr, id DeviceID, flags uint32, targets []Target) error {
	payload, err := dmioctl.Payload{
		Name:    id.Name,
		UUID:    id.UUID,
		Targets: encodeTargets(targets),
		Flags:   flags,
	}.Build()
	if err != nil {
		return err
	}

	return c.ioctl(request, payload)
}

func encodeTargets(targets []Target) []dmioctl.Target {
	if len(targets) == 0 {
		return nil
	}

	encoded := make([]dmioctl.Target, 0, len(targets))

	for _, target := range targets {
		encoded = append(encoded, dmioctl.Target{
			Type:        target.Type,
			Params:      target.Params,
			SectorStart: target.SectorStart,
			Length:      target.Length,
		})
	}

	return encoded
}

func (c *Control) ioctl(request uintptr, payload []byte) error {
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		c.f.Fd(),
		request,
		uintptr(unsafe.Pointer(&payload[0])),
	)

	runtime.KeepAlive(c)
	runtime.KeepAlive(payload)

	if errno != 0 {
		return errno
	}

	return nil
}
