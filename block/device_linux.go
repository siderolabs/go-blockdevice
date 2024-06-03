// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package block

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/unix"
)

// NewFromPath returns a new Device from the specified path.
func NewFromPath(path string) (*Device, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}

	return &Device{
		f:         f,
		ownedFile: true,
	}, nil
}

func (d *Device) clone() *Device {
	return &Device{
		f:         d.f,
		ownedFile: false,
		devNo:     d.devNo,
	}
}

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

	arg, _, errno := unix.Syscall(unix.SYS_IOCTL, d.f.Fd(), uintptr(CDROM_DRIVE_STATUS), 0)

	return errno == 0 && (arg == 1 || arg == 2)
}

// GetDevNo returns the device number of the blockdevice.
func (d *Device) GetDevNo() (uint64, error) {
	if d.devNo != 0 {
		return d.devNo, nil
	}

	var st unix.Stat_t
	if err := unix.Fstat(int(d.f.Fd()), &st); err != nil {
		return 0, err
	}

	d.devNo = st.Rdev

	return d.devNo, nil
}

func (d *Device) sysFsPath() (string, error) {
	devNo, err := d.GetDevNo()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("/sys/dev/block/%d:%d", unix.Major(devNo), unix.Minor(devNo)), nil
}

// IsReadOnly returns true if the blockdevice is read-only.
func (d *Device) IsReadOnly() (bool, error) {
	sysFsPath, err := d.sysFsPath()
	if err != nil {
		return false, err
	}

	roContents, err := os.ReadFile(filepath.Join(sysFsPath, "ro"))
	if err != nil {
		if !os.IsNotExist(err) {
			return false, err
		}
	}

	if len(roContents) > 0 {
		return roContents[0] == '1', nil
	}

	var flags int
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, d.f.Fd(), unix.BLKROGET, uintptr(unsafe.Pointer(&flags))); errno != 0 {
		return false, errno
	}

	return flags != 0, nil
}

// IsWholeDisk returns true if the blockdevice is a whole disk.
func (d *Device) IsWholeDisk() (bool, error) {
	sysFsPath, err := d.sysFsPath()
	if err != nil {
		return false, err
	}

	// check if this is a partition
	_, err = os.Stat(filepath.Join(sysFsPath, "partition"))
	isPartition := err == nil

	if isPartition {
		return false, nil
	}

	// device-mapper check
	contents, err := os.ReadFile(filepath.Join(sysFsPath, "dm", "uuid"))
	if err != nil {
		// not devmapper
		return true, nil //nolint:nilerr
	}

	return !bytes.HasPrefix(contents, []byte("part-")), nil
}

// GetWholeDisk returns the whole disk for the blockdevice.
//
// If the blockdevice is a whole disk, it returns itself.
// The returned block device should be closed.
func (d *Device) GetWholeDisk() (*Device, error) {
	sysFsPath, err := d.sysFsPath()
	if err != nil {
		return nil, err
	}

	// check if this is a partition
	_, err = os.Stat(filepath.Join(sysFsPath, "partition"))
	isPartition := err == nil

	if isPartition {
		var path string

		path, err = os.Readlink(sysFsPath)
		if err != nil {
			return nil, err
		}

		devName := filepath.Base(filepath.Dir(path))

		return NewFromPath(filepath.Join("/dev", devName))
	}

	// device-mapper check
	contents, err := os.ReadFile(filepath.Join(sysFsPath, "dm", "uuid"))
	if err != nil {
		// not devmapper
		return d.clone(), nil //nolint:nilerr
	}

	if !bytes.HasPrefix(contents, []byte("part-")) {
		// devmapper, but not a partition
		return d.clone(), nil
	}

	slaves, err := os.ReadDir(filepath.Join(sysFsPath, "slaves"))
	if err != nil {
		return nil, err
	}

	if len(slaves) == 0 {
		return nil, errors.New("no slaves found")
	}

	return NewFromPath(filepath.Join("/dev", slaves[0].Name()))
}

// IsPrivateDeviceMapper returns true if this is a private device-mapper device.
func (d *Device) IsPrivateDeviceMapper() (bool, error) {
	sysFsPath, err := d.sysFsPath()
	if err != nil {
		return false, err
	}

	contents, err := os.ReadFile(filepath.Join(sysFsPath, "dm", "uuid"))
	if err != nil {
		return false, nil //nolint:nilerr
	}

	// check for pattern "LVM-<uuid>-name"
	prefix, rest, ok := bytes.Cut(contents, []byte("-"))
	if !ok {
		return false, nil
	}

	if !bytes.Equal(prefix, []byte("LVM")) {
		return false, nil
	}

	_, _, ok = bytes.Cut(rest, []byte("-"))

	return ok, nil
}

// Lock (and block until the lock is acquired) for the block device.
func (d *Device) Lock(exclusive bool) error {
	return d.lock(exclusive, 0)
}

// TryLock (and return an error if failed).
func (d *Device) TryLock(exclusive bool) error {
	return d.lock(exclusive, unix.LOCK_NB)
}

// Unlock releases any lock.
func (d *Device) Unlock() error {
	for {
		if err := unix.Flock(int(d.f.Fd()), unix.LOCK_UN); !errors.Is(err, unix.EINTR) {
			return err
		}
	}
}

func (d *Device) lock(exclusive bool, flag int) error {
	if exclusive {
		flag |= unix.LOCK_EX
	} else {
		flag |= unix.LOCK_SH
	}

	for {
		if err := unix.Flock(int(d.f.Fd()), flag); !errors.Is(err, unix.EINTR) {
			return err
		}
	}
}
