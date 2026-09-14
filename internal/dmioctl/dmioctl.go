// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package dmioctl provides the wire format of the device-mapper ioctl interface: the structures
// exchanged with /dev/mapper/control, and the assembly and parsing of the ioctl payloads.
package dmioctl

//go:generate go run ../cstruct/cstruct.go -pkg dmioctl -struct Header -input header.h -endianness NativeEndian

//go:generate go run ../cstruct/cstruct.go -pkg dmioctl -struct Spec -input spec.h -endianness NativeEndian

//go:generate go run ../cstruct/cstruct.go -pkg dmioctl -struct NameList -input namelist.h -endianness NativeEndian

// Limits of the fixed-size fields of the ioctl header, from <linux/dm-ioctl.h>.
//
// Both lengths include the terminating NUL byte.
const (
	// NameLen is the size of the device name field (DM_NAME_LEN).
	NameLen = 128
	// UUIDLen is the size of the device UUID field (DM_UUID_LEN).
	UUIDLen = 129
	// TypeLen is the size of the target type field (DM_MAX_TYPE_NAME).
	TypeLen = 16
)

// Version of the ioctl interface this package speaks (DM_VERSION_MAJOR and up).
//
// The kernel rejects a different major version, and any minor version above its own; the lowest
// minor is requested, as none of the commands used here need a newer one.
const (
	VersionMajor = 4
	VersionMinor = 0
	VersionPatch = 0
)

// ioctl encoding, mirroring the _IOWR() macro: every device-mapper command is
// _IOWR(DM_IOCTL, <command>, struct dm_ioctl).
const (
	iocWrite = 1
	iocRead  = 2

	iocTypeShift = 8
	iocSizeShift = 16
	iocDirShift  = 30

	// dmIoctlType is DM_IOCTL, the ioctl type of the device-mapper interface.
	dmIoctlType = 0xfd

	requestBase = (iocRead|iocWrite)<<iocDirShift | HEADER_SIZE<<iocSizeShift | dmIoctlType<<iocTypeShift
)

// Requests are the ioctl request numbers of the device-mapper commands used here.
//
// The numbering is the order of the command enum in <linux/dm-ioctl.h>.
const (
	VersionRequest     = requestBase // command 0
	ListDevicesRequest = requestBase | 2
	DevCreateRequest   = requestBase | 3
	DevRemoveRequest   = requestBase | 4
	DevSuspendRequest  = requestBase | 6
	TableLoadRequest   = requestBase | 9
)

// Flags of the ioctl header, from <linux/dm-ioctl.h>.
const (
	// ReadOnlyFlag marks the table being loaded read-only; the mode is a property of the table,
	// so it has to be set on every load.
	ReadOnlyFlag = 1 << 0
	// SuspendFlag suspends the device; DevSuspendRequest without it resumes the device.
	SuspendFlag = 1 << 1
	// BufferFullFlag is set by the kernel when the reply didn't fit into the payload.
	BufferFullFlag = 1 << 8
	// SkipLockfsFlag keeps the kernel from freezing the filesystem on the device while it
	// suspends it; without it, a suspend of a live device freezes the filesystem first.
	SkipLockfsFlag = 1 << 10
)
