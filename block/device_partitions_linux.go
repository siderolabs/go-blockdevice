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

// KernelPartitionAdd invokes the BLKPG_ADD_PARTITION ioctl.
func (d *Device) KernelPartitionAdd(no int, start, length uint64) error {
	return d.inform(unix.BLKPG_ADD_PARTITION, int32(no), int64(start), int64(length))
}

// KernelPartitionResize invokes the BLKPG_RESIZE_PARTITION ioctl.
func (d *Device) KernelPartitionResize(no int, first, length uint64) error {
	return d.inform(unix.BLKPG_RESIZE_PARTITION, int32(no), int64(first), int64(length))
}

// KernelPartitionDelete invokes the BLKPG_DEL_PARTITION ioctl.
func (d *Device) KernelPartitionDelete(no int) error {
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

// GetKernelLastPartitionNum returns the maximum partition number in the kernel.
func (d *Device) GetKernelLastPartitionNum() (int, error) {
	sysFsPath, err := d.sysFsPath()
	if err != nil {
		return 0, err
	}

	devices, err := sysfs.KernelPartitionDevices(sysFsPath)
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
