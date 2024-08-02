// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package block provides support for operations on blockdevices.
package block

import "os"

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
	// Serial /sys/block/<dev>/device/serial.
	Serial string
	// Modalias /sys/block/<dev>/device/modalias.
	Modalias string
	// WWID /sys/block/<dev>/wwid.
	WWID string
	// UUID /sys/block/<dev>/uuid.
	// BusPath PCI bus path.
	BusPath string
	// SubSystem is the dest path of symlink /sys/block/<dev>/subsystem.
	SubSystem string
	// Transport of the device: SCSI, ata, ahci, nvme, etc.
	Transport string
	// Rotational is true if the device is a rotational disk.
	Rotational bool
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
