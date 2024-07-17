// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package block

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
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

	contents, err := os.ReadDir(sysFsPath)
	if err != nil {
		return 0, err
	}

	var max int

	for _, entry := range contents {
		if !entry.IsDir() {
			continue
		}

		contents := readSysFsFile(filepath.Join(sysFsPath, entry.Name(), "partition"))
		if len(contents) == 0 {
			continue
		}

		partNum, err := strconv.Atoi(contents)
		if err != nil {
			continue
		}

		if partNum > max {
			max = partNum
		}
	}

	return max, nil
}
