// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package devicemapper implements a minimal client for the Linux device-mapper.
//
// It talks to the kernel directly over the /dev/mapper/control ioctl interface, and covers only
// what it takes to manage simple mapped devices: creating a device with a table, replacing the
// table of an existing device, removing a device and listing devices.
//
// Devices are created without a udev cookie, the way 'dmsetup --noudevsync' does, so nothing here
// waits for udev: a caller which needs the device node of a device it just created should resolve
// it through /sys/block/*/dm/uuid rather than through the /dev/mapper name, which udev creates
// asynchronously.
package devicemapper

import (
	"fmt"

	"github.com/siderolabs/go-blockdevice/v2/internal/dmioctl"
)

// SectorSize is the unit of the sector counts in a device-mapper table.
//
// It is fixed at 512 bytes regardless of the logical block size of the device being mapped.
const SectorSize = 512

// Target is a single target of a device-mapper table.
//
// A table is a list of targets covering a contiguous range of the mapped device.
type Target struct {
	// Type of the target, e.g. "linear".
	Type string
	// Params of the target, in the format its type defines, e.g. "<major>:<minor> <offset>" for
	// a "linear" target.
	Params string
	// SectorStart is the offset of this target within the mapped device, in sectors.
	SectorStart uint64
	// Length of this target, in sectors.
	Length uint64
}

// TableOption is an option of the table of a device-mapper device.
type TableOption func(*tableOptions)

type tableOptions struct {
	readOnly bool
}

// WithReadOnly marks the device read-only, or read-write, as its table is loaded.
//
// The mode is a property of the table rather than of the device, so it has to be set on every
// load: a reload without it makes a read-only device writable again.
func WithReadOnly(readOnly bool) TableOption {
	return func(o *tableOptions) {
		o.readOnly = readOnly
	}
}

func (o tableOptions) flags() uint32 {
	if o.readOnly {
		return dmioctl.ReadOnlyFlag
	}

	return 0
}

// DeviceID identifies a device-mapper device by name, by UUID, or by both.
//
// Creating a device takes both: the name it is known by, and the UUID it is identified by. Looking
// an existing device up takes exactly one of them — the kernel rejects a lookup which carries both —
// so where both are set, the UUID wins, being the durable identifier of the two.
type DeviceID struct {
	// Name of the device, as in /dev/mapper/<name>.
	Name string
	// UUID of the device, as in /sys/block/<dev>/dm/uuid.
	UUID string
}

// ByName identifies a device-mapper device by its name.
func ByName(name string) DeviceID {
	return DeviceID{Name: name}
}

// ByUUID identifies a device-mapper device by its UUID.
func ByUUID(uuid string) DeviceID {
	return DeviceID{UUID: uuid}
}

// lookup returns the identifier to look an existing device up by.
//
// The kernel refuses an ioctl which names a device both ways, so only one of the two is sent.
func (id DeviceID) lookup() DeviceID {
	if id.UUID != "" {
		return DeviceID{UUID: id.UUID}
	}

	return DeviceID{Name: id.Name}
}

// String implements fmt.Stringer.
func (id DeviceID) String() string {
	switch {
	case id.UUID != "" && id.Name != "":
		return id.Name + " (" + id.UUID + ")"
	case id.UUID != "":
		return id.UUID
	default:
		return id.Name
	}
}

// Version is the version of the device-mapper driver in the kernel.
type Version struct {
	Major uint32
	Minor uint32
	Patch uint32
}

// String implements fmt.Stringer.
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// DeviceInfo is a device-mapper device as reported by ListDevices.
type DeviceInfo struct {
	// Name of the device.
	Name string
	// DevNo is the device number of the device.
	DevNo uint64
}
