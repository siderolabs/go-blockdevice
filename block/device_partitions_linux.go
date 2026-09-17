// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package block

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/siderolabs/go-blockdevice/v2/block/internal/sysfs"
)

// KernelPartitionAdd makes the kernel aware of a partition of the device, at the given offset and
// of the given size, both in bytes.
//
// A device-mapper device never gets kernel partitions, so a partition map is created for it, the
// way kpartx does; every other device is informed of the partition with the BLKPG_ADD_PARTITION
// ioctl.
func (d *Device) KernelPartitionAdd(no int, start, length uint64) error {
	dm, err := d.deviceMapper()
	if err != nil {
		return err
	}

	if dm.ok() {
		return d.deviceMapperPartitionAdd(dm, no, start, length)
	}

	return d.inform(unix.BLKPG_ADD_PARTITION, int32(no), int64(start), int64(length))
}

// KernelPartitionResize changes the offset and the size of a partition of the device, both in bytes.
//
// For a device-mapper device the table of the partition map is replaced; for every other device the
// BLKPG_RESIZE_PARTITION ioctl is invoked.
func (d *Device) KernelPartitionResize(no int, first, length uint64) error {
	dm, err := d.deviceMapper()
	if err != nil {
		return err
	}

	if dm.ok() {
		return d.deviceMapperPartitionResize(dm, no, first, length)
	}

	return d.inform(unix.BLKPG_RESIZE_PARTITION, int32(no), int64(first), int64(length))
}

// KernelPartitionDelete drops a partition of the device from the kernel.
//
// For a device-mapper device the partition map is removed; for every other device the
// BLKPG_DEL_PARTITION ioctl is invoked. Either way a partition which doesn't exist reports
// unix.ENXIO, and one which is held open reports unix.EBUSY.
func (d *Device) KernelPartitionDelete(no int) error {
	dm, err := d.deviceMapper()
	if err != nil {
		return err
	}

	if dm.ok() {
		return d.deviceMapperPartitionDelete(dm, no)
	}

	return d.inform(unix.BLKPG_DEL_PARTITION, int32(no), 0, 0)
}

func (d *Device) inform(op int32, no int32, start, length int64) error {
	data := &unix.BlkpgPartition{
		Start:  start,
		Length: length,
		Pno:    no,
	}

	arg := &unix.BlkpgIoctlArg{
		Op:      op,
		Datalen: int32(unsafe.Sizeof(*data)),
		Data:    (*byte)(unsafe.Pointer(data)),
	}

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		d.f.Fd(),
		unix.BLKPG,
		uintptr(unsafe.Pointer(arg)),
	)

	runtime.KeepAlive(d)

	if errno == 0 {
		return nil
	}

	return errno
}

// GetKernelLastPartitionNum returns the maximum partition number the kernel knows for the device.
//
// For a device-mapper device the partitions are its partition maps, as it has no kernel partitions.
func (d *Device) GetKernelLastPartitionNum() (int, error) {
	sysFsPath, err := d.sysFsPath()
	if err != nil {
		return 0, err
	}

	devices, err := sysfs.PartitionDevices(sysFsPath)
	if err != nil {
		return 0, err
	}

	var maxPartNum int

	for partNo := range devices {
		maxPartNum = max(maxPartNum, int(partNo))
	}

	return maxPartNum, nil
}

// ErrPartitionNotFound is returned when the partition device is not found (yet).
var ErrPartitionNotFound = errors.New("partition device not found")

// GetPartitionDevices returns a map of partition number to the kernel device name of the partition device.
//
// For a device-mapper device, kernel partitions never exist (the kernel sets GENHD_FL_NO_PART on every
// device-mapper gendisk), so partitions are separate device-mapper devices carrying the UUID
// "part<N>-<parent UUID>" (as created by kpartx); they are enumerated via holders/.
// For any other device, kernel partitions are enumerated from /sys/dev/block/<dev>/*/partition.
func (d *Device) GetPartitionDevices() (map[uint]string, error) {
	sysFsPath, err := d.sysFsPath()
	if err != nil {
		return nil, err
	}

	return sysfs.PartitionDevices(sysFsPath)
}

// GetPartitionDevName returns the path of the device of the partition with the given number, e.g. "/dev/sda1".
//
// If the partition device hasn't appeared (yet), ErrPartitionNotFound is returned.
func (d *Device) GetPartitionDevName(no uint) (string, error) {
	devices, err := d.GetPartitionDevices()
	if err != nil {
		return "", err
	}

	devName, ok := devices[no]
	if !ok {
		return "", fmt.Errorf("%w: partition %d", ErrPartitionNotFound, no)
	}

	return filepath.Join("/dev", devName), nil
}
