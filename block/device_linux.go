// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package block

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

// GetSize returns blockdevice size in bytes.
func (d *Device) GetSize() (uint64, error) {
	var devsize uint64
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, d.f.Fd(), unix.BLKGETSIZE64, uintptr(unsafe.Pointer(&devsize))); errno != 0 {
		return 0, errno
	}

	return devsize, nil
}

// GetIOSize returns blockdevice optimal I/O size in bytes.
func (d *Device) GetIOSize() (uint, error) {
	for _, ioctl := range []uintptr{unix.BLKIOOPT, unix.BLKIOMIN, unix.BLKBSZGET} {
		var size uint
		if _, _, errno := unix.Syscall(unix.SYS_IOCTL, d.f.Fd(), ioctl, uintptr(unsafe.Pointer(&size))); errno != 0 {
			continue
		}

		if size > 0 && isPowerOf2(size) {
			return size, nil
		}
	}

	return DefaultBlockSize, nil
}

// GetSectorSize returns blockdevice sector size in bytes.
func (d *Device) GetSectorSize() uint {
	var size uint

	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, d.f.Fd(), uintptr(unix.BLKSSZGET), uintptr(unsafe.Pointer(&size))); errno != 0 {
		return DefaultBlockSize
	}

	return size
}

// IsCD returns true if the blockdevice is a CD-ROM device.
func (d *Device) IsCD() bool {
	const CDROM_GET_CAPABILITY = 0x5331 //nolint:revive,stylecheck

	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, d.f.Fd(), uintptr(CDROM_GET_CAPABILITY), 0); errno != 0 {
		return false
	}

	return true
}

// IsCDNoMedia returns true if the blockdevice is a CD-ROM device without media.
func (d *Device) IsCDNoMedia() bool {
	const CDROM_DRIVE_STATUS = 0x5326 //nolint:revive,stylecheck

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, d.f.Fd(), uintptr(CDROM_DRIVE_STATUS), uintptr(2147483647))

	return errno == 1 || errno == 2
}
