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
	"runtime"
	"strconv"
	"strings"
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

	runtime.KeepAlive(d)

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

	runtime.KeepAlive(d)

	return DefaultBlockSize, nil
}

// GetSectorSize returns blockdevice sector size in bytes.
func (d *Device) GetSectorSize() uint {
	var size uint

	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, d.f.Fd(), uintptr(unix.BLKSSZGET), uintptr(unsafe.Pointer(&size))); errno != 0 {
		return DefaultBlockSize
	}

	runtime.KeepAlive(d)

	return size
}

// IsCD returns true if the blockdevice is a CD-ROM device.
func (d *Device) IsCD() bool {
	const CDROM_GET_CAPABILITY = 0x5331 //nolint:revive,stylecheck

	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, d.f.Fd(), uintptr(CDROM_GET_CAPABILITY), 0); errno != 0 {
		return false
	}

	runtime.KeepAlive(d)

	return true
}

// IsCDNoMedia returns true if the blockdevice is a CD-ROM device without media.
func (d *Device) IsCDNoMedia() bool {
	const CDROM_DRIVE_STATUS = 0x5326 //nolint:revive,stylecheck

	arg, _, errno := unix.Syscall(unix.SYS_IOCTL, d.f.Fd(), uintptr(CDROM_DRIVE_STATUS), 0)

	runtime.KeepAlive(d)

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

	runtime.KeepAlive(d)

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

	runtime.KeepAlive(d)

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

		runtime.KeepAlive(d)
	}
}

// GetProperties returns the properties of the block device.
func (d *Device) GetProperties() (*DeviceProperties, error) {
	sysFsPath, err := d.sysFsPath()
	if err != nil {
		return nil, err
	}

	props := &DeviceProperties{
		Model:    readSysFsFile(filepath.Join(sysFsPath, "device", "model")),
		Serial:   readSysFsFile(filepath.Join(sysFsPath, "device", "serial")),
		Modalias: readSysFsFile(filepath.Join(sysFsPath, "device", "modalias")),
		WWID:     readSysFsFile(filepath.Join(sysFsPath, "wwid")),
	}

	if props.WWID == "" {
		props.WWID = readSysFsFile(filepath.Join(sysFsPath, "device", "wwid"))
	}

	fullPath, err := os.Readlink(sysFsPath)
	if err == nil {
		props.BusPath = filepath.Dir(filepath.Dir(strings.TrimPrefix(fullPath, "../../devices")))
		props.DeviceName = filepath.Base(fullPath)
	}

	props.Rotational = readSysFsFile(filepath.Join(sysFsPath, "queue", "rotational")) == "1"

	if subsystemPath, err := filepath.EvalSymlinks(filepath.Join(sysFsPath, "subsystem")); err == nil {
		props.SubSystem = subsystemPath
	}

	props.Transport = d.getTransport(sysFsPath, props.DeviceName)

	return props, nil
}

func (d *Device) getTransport(sysFsPath, deviceName string) string {
	switch {
	case strings.HasPrefix(deviceName, "nvme"):
		return "nvme"
	case strings.HasPrefix(deviceName, "vd"):
		return "virtio"
	case strings.HasPrefix(deviceName, "mmcblk"):
		return "mmc"
	}

	devicePath, err := os.Readlink(filepath.Join(sysFsPath, "device"))
	if err != nil {
		return ""
	}

	devicePath = filepath.Base(devicePath)

	hostStr, _, ok := strings.Cut(devicePath, ":")
	if !ok {
		return ""
	}

	host, err := strconv.Atoi(hostStr)
	if err != nil {
		return ""
	}

	switch {
	case isScsiHost(host, "sas"):
		return "sas"
	case isScsiHost(host, "fc"):
		return "fc"
	case isScsiHost(host, "sas") && scsiHasAttribute(devicePath, "sas_device"):
		return "sas"
	case scsiHasAttribute(devicePath, "ieee1394_id"):
		return "ibp"
	case isScsiHost(host, "iscsi"):
		return "iscsi"
	case scsiPathContains(devicePath, "usb"):
		return "usb"
	case isScsiHost(host, "scsi"):
		procName := readScsiHostAttribute(host, "scsi", "proc_name")

		switch {
		case procName == "ahci", procName == "sata":
			return "sata"
		case strings.Contains(procName, "ata"):
			return "ata"
		case procName == "virtio_scsi":
			return "virtio"
		}
	}

	return ""
}

func isScsiHost(host int, typ string) bool {
	path := filepath.Join("/sys/class", typ+"_host", "host"+strconv.Itoa(host))

	st, err := os.Stat(path)

	return err == nil && st.IsDir()
}

func readScsiHostAttribute(host int, typ, attr string) string {
	path := filepath.Join("/sys/class", typ+"_host", "host"+strconv.Itoa(host), attr)

	contents, _ := os.ReadFile(path) //nolint:errcheck

	return string(bytes.TrimSpace(contents))
}

func scsiHasAttribute(devicePath, attribute string) bool {
	path := filepath.Join("/sys/bus/scsi/devices", devicePath, attribute)

	_, err := os.Stat(path)

	return err == nil
}

func scsiPathContains(devicePath, what string) bool {
	path := filepath.Join("/sys/bus/scsi/devices", devicePath)

	dest, _ := os.Readlink(path) //nolint:errcheck

	return strings.Contains(dest, what)
}

func readSysFsFile(path string) string {
	contents, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return string(bytes.TrimSpace(contents))
}
