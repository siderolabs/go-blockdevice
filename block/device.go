// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package block provides support for operations on blockdevices.
package block

import (
	"os"

	"github.com/siderolabs/go-blockdevice/v2/block/internal/sysfs"
)

// Device wraps blockdevice operations.
type Device struct {
	f         *os.File
	ownedFile bool

	devNo uint64
}

// NewFromFile returns a new Device from the specified file.
func NewFromFile(f *os.File) *Device {
	return &Device{f: f}
}

// Close the device.
//
// No-op if the device was created from a file.
func (d *Device) Close() error {
	if d.ownedFile {
		return d.f.Close()
	}

	return nil
}

// File returns the underlying file.
func (d *Device) File() *os.File {
	return d.f
}

// DefaultBlockSize is the default block size in bytes.
const DefaultBlockSize = 512

// DeviceProperties contains the properties of a block device.
type DeviceProperties struct {
	// Device name, as in 'sda'.
	DeviceName string
	// Model from /sys/block/*/device/model.
	Model string
	// Vendor from /sys/block/<dev>/device/vendor.
	Vendor string
	// Serial /sys/block/<dev>/device/serial.
	Serial string
	// Modalias from /sys/block/<dev>/device/modalias, falling back to
	// /sys/block/<dev>/device/device/modalias when the former is absent (common on NVMe).
	Modalias string
	// WWID /sys/block/<dev>/wwid.
	WWID string
	// UUID /sys/block/<dev>/uuid.
	UUID string
	// BusPath PCI bus path.
	BusPath string
	// SubSystem is the dest path of symlink /sys/block/<dev>/subsystem.
	SubSystem string
	// Transport of the device: sata, nvme, virtio, dm, etc.
	Transport string
	// DeviceMapperName is /sys/block/<dev>/dm/name, only set for DM devices.
	DeviceMapperName string
	// DeviceMapperUUID is /sys/block/<dev>/dm/uuid, only set for DM devices.
	DeviceMapperUUID string
	// DeviceMapperKind is mpath, lvm, crypt, or dm.
	DeviceMapperKind string
	// DeviceMapperParentUUID is the DeviceMapperUUID of the parent device, only set for a DM
	// partition map (see ParseDeviceMapperPartitionUUID).
	DeviceMapperParentUUID string
	// FirmwareRevision from /sys/block/<dev>/device/firmware_rev (NVMe only).
	FirmwareRevision string
	// DeviceMapperPartitionNumber is the partition number, only set for a DM partition map.
	DeviceMapperPartitionNumber uint
	// Rotational is true if the device is a rotational disk.
	Rotational bool
}

// ParseDeviceMapperPartitionUUID parses the UUID of a device-mapper partition map, which is
// "part<N>-<parent UUID>" as kpartx composes it, returning the partition number and the UUID of the
// device it is a partition of.
//
// It returns false for the UUID of any other device-mapper device.
func ParseDeviceMapperPartitionUUID(uuid string) (partitionNumber uint, parentUUID string, ok bool) {
	return sysfs.ParseDeviceMapperPartitionUUID(uuid)
}

// Options for NewFromPath.
type Options struct {
	Flag int
}

// Option is a function that modifies Options.
type Option func(*Options)

// OpenForWrite opens the device for writing.
func OpenForWrite() Option {
	return func(o *Options) {
		o.Flag |= os.O_RDWR
	}
}

// OpenAssertNotMounted opens in O_EXCL mode to assert the device is not mounted.
//
// From the open(2):
//
// In  general,  the  behavior  of  O_EXCL is undefined if it is used without O_CREAT.
// There is one exception: on Linux 2.6 and later,
// O_EXCL can be used without O_CREAT if pathname refers to a block device.
// If the block  device  is  in  use  by  the  system  (e.g.,
// mounted), open() fails with the error EBUSY.
func OpenAssertNotMounted() Option {
	return func(o *Options) {
		o.Flag |= os.O_EXCL
	}
}
