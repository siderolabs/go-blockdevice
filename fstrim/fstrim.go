// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build linux

// Package fstrim discards unused blocks of a mounted filesystem (the equivalent
// of the fstrim(8) command).
package fstrim

import (
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ErrNotSupported indicates that the filesystem (or the underlying block device)
// does not support discarding unused blocks.
var ErrNotSupported = errors.New("fstrim is not supported for this filesystem")

// FITRIM is the ioctl number to discard unused blocks of a mounted filesystem.
//
// It is _IOWR('X', 121, struct fstrim_range), where struct fstrim_range is three
// uint64 fields (24 bytes). This encoding is the same on all architectures using
// the asm-generic ioctl numbering (amd64, arm64, ...).
const FITRIM = 0xc0185879

// fstrimRange mirrors the kernel's struct fstrim_range.
type fstrimRange struct {
	Start  uint64
	Len    uint64
	Minlen uint64
}

// Fstrim discards unused blocks of the mounted filesystem at the given path.
//
// The path may be the mount point or any file or directory within the mounted
// filesystem.
//
// It returns the number of bytes discarded as reported by the kernel (the same
// figure printed by `fstrim -v`; an estimate, not a hardware-confirmed count).
//
// The error is nil on success, ErrNotSupported if the filesystem or the
// underlying device does not support discard, or another error if the operation
// failed.
func Fstrim(path string) (uint64, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return 0, err
	}

	defer f.Close() //nolint:errcheck

	return FstrimFd(f.Fd())
}

// FstrimFd discards unused blocks of the mounted filesystem identified by the
// open file descriptor.
//
// The descriptor may refer to the mount point or any file or directory within
// the mounted filesystem.
//
// See Fstrim for the returned value and errors.
func FstrimFd(fd uintptr) (uint64, error) {
	// Start at offset 0 with the maximum length: the kernel clamps the length to
	// the filesystem size, so this trims the whole filesystem.
	r := fstrimRange{
		Start:  0,
		Len:    math.MaxUint64,
		Minlen: 0,
	}

	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, FITRIM, uintptr(unsafe.Pointer(&r))); errno != 0 {
		runtime.KeepAlive(&r)

		switch errno { //nolint:exhaustive
		case unix.EOPNOTSUPP, unix.ENOTTY:
			return 0, ErrNotSupported
		default:
			return 0, fmt.Errorf("FITRIM failed: %w", errno)
		}
	}

	// on success the kernel writes the number of bytes discarded back into Len.
	trimmed := r.Len

	runtime.KeepAlive(&r)

	return trimmed, nil
}
