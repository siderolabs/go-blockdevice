// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package block

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/siderolabs/go-blockdevice/v2/block/internal/sysfs"
	"github.com/siderolabs/go-blockdevice/v2/devicemapper"
)

// deviceMapperInfo is the device-mapper identity of a device.
//
// The zero value is a device which is not a device-mapper device: every device-mapper device has a
// name, while the UUID is the caller's to set and may be empty.
type deviceMapperInfo struct {
	name string
	uuid string
}

// ok returns true if the device is a device-mapper device.
func (dm deviceMapperInfo) ok() bool {
	return dm.name != ""
}

// partitionID returns the identity of the partition map for the partition with the given number.
//
// It is the identity kpartx would give the map: the name of the parent with a "-part<N>" suffix,
// and the UUID of the parent with a "part<N>-" prefix. Matching kpartx matters, because kpartx
// looks a partition up by UUID before name, so a map created here is adopted rather than duplicated
// if kpartx later runs over the same disk.
func (dm deviceMapperInfo) partitionID(no int) (devicemapper.DeviceID, error) {
	// partition numbers are 1-based: a map for any other number would be one no lookup of the
	// partitions of this device accepts, so it could never be found again to be resized or removed
	if no < 1 {
		return devicemapper.DeviceID{}, fmt.Errorf("invalid partition number %d for device-mapper device %q", no, dm.name)
	}

	if dm.uuid == "" {
		return devicemapper.DeviceID{}, fmt.Errorf(
			"device-mapper device %q has no UUID, so its partitions cannot be addressed", dm.name,
		)
	}

	return devicemapper.DeviceID{
		Name: fmt.Sprintf("%s-part%d", dm.name, no),
		UUID: fmt.Sprintf("part%d-%s", no, dm.uuid),
	}, nil
}

// deviceMapper returns the device-mapper identity of the device.
//
// The zero value is returned for a device which is not a device-mapper device.
func (d *Device) deviceMapper() (deviceMapperInfo, error) {
	sysFsPath, err := d.sysFsPath()
	if err != nil {
		return deviceMapperInfo{}, err
	}

	name := sysfs.ReadFile(filepath.Join(sysFsPath, "dm", "name"))
	if name == "" {
		return deviceMapperInfo{}, nil
	}

	return deviceMapperInfo{
		name: name,
		uuid: sysfs.ReadFile(filepath.Join(sysFsPath, "dm", "uuid")),
	}, nil
}

// deviceMapperPartitionTarget returns the table of the partition map for a partition at the given
// offset and of the given size, both in bytes: a linear target over this device.
func (d *Device) deviceMapperPartitionTarget(start, length uint64) (devicemapper.Target, error) {
	// the kernel rejects a target whose offset and length are not multiples of the logical block
	// size of the device being mapped, which may be larger than the device-mapper sector
	blockSize := max(uint64(d.GetSectorSize()), devicemapper.SectorSize)

	if start%blockSize != 0 || length%blockSize != 0 {
		return devicemapper.Target{}, fmt.Errorf(
			"partition offset %d and size %d should be multiples of %d", start, length, blockSize,
		)
	}

	devNo, err := d.GetDevNo()
	if err != nil {
		return devicemapper.Target{}, err
	}

	// the parent is named by device number rather than by path: the device node of a device-mapper
	// device is created asynchronously, so a path might not resolve yet
	return devicemapper.Target{
		Type:   "linear",
		Params: fmt.Sprintf("%d:%d %d", unix.Major(devNo), unix.Minor(devNo), start/devicemapper.SectorSize),
		Length: length / devicemapper.SectorSize,
	}, nil
}

// withControl runs f with an open handle for the device-mapper control device.
func withControl(f func(*devicemapper.Control) error) error {
	control, err := devicemapper.NewControl()
	if err != nil {
		return err
	}

	defer control.Close() //nolint:errcheck

	return f(control)
}

// deviceMapperPartitionAdd creates the partition map for a partition of this device.
func (d *Device) deviceMapperPartitionAdd(dm deviceMapperInfo, no int, start, length uint64) error {
	id, err := dm.partitionID(no)
	if err != nil {
		return err
	}

	target, err := d.deviceMapperPartitionTarget(start, length)
	if err != nil {
		return err
	}

	readOnly, err := d.IsReadOnly()
	if err != nil {
		return fmt.Errorf("failed to determine whether device-mapper device %q is read-only: %w", dm.name, err)
	}

	return withControl(func(control *devicemapper.Control) error {
		return control.CreateDevice(id, []devicemapper.Target{target}, devicemapper.WithReadOnly(readOnly))
	})
}

// deviceMapperPartitionResize replaces the table of the partition map for a partition of this device.
func (d *Device) deviceMapperPartitionResize(dm deviceMapperInfo, no int, first, length uint64) error {
	id, err := dm.partitionID(no)
	if err != nil {
		return err
	}

	target, err := d.deviceMapperPartitionTarget(first, length)
	if err != nil {
		return err
	}

	// the mode is a property of the table, so it has to be re-asserted on every load, or the
	// reload would make the partition map of a read-only disk writable
	readOnly, err := d.IsReadOnly()
	if err != nil {
		return fmt.Errorf("failed to determine whether device-mapper device %q is read-only: %w", dm.name, err)
	}

	return withControl(func(control *devicemapper.Control) error {
		return control.ReloadDevice(devicemapper.ByUUID(id.UUID), []devicemapper.Target{target}, devicemapper.WithReadOnly(readOnly))
	})
}

// deviceMapperPartitionDelete removes the partition map for a partition of this device.
//
// The map is removed by UUID, so a map kpartx created for the same partition is removed as well.
func (d *Device) deviceMapperPartitionDelete(dm deviceMapperInfo, no int) error {
	id, err := dm.partitionID(no)
	if err != nil {
		return err
	}

	return withControl(func(control *devicemapper.Control) error {
		return control.RemoveDevice(devicemapper.ByUUID(id.UUID))
	})
}
