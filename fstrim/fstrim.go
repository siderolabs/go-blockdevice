// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build linux

// Package fstrim discards unused blocks of a mounted filesystem (the equivalent
// of the fstrim(8) command).
package fstrim

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"time"
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
	return fitrim(fd, 0, math.MaxUint64, 0)
}

// Options configures a chunked trim, see FstrimChunked.
type Options struct {
	// ChunkSize is the size (in bytes) of the filesystem range trimmed by a single FITRIM call.
	//
	// Zero trims the whole filesystem with a single call (same as Fstrim).
	//
	// For ext4 a multiple of the block group size (128 MiB with 4 KiB blocks) keeps
	// the kernel "already trimmed" tracking effective across runs.
	ChunkSize uint64
	// MinLength is the minimum contiguous free range (in bytes) to discard.
	//
	// The kernel raises it to the device discard granularity; smaller free ranges are skipped.
	MinLength uint64
	// Delay is the delay between consecutive FITRIM calls, giving other I/O a chance to proceed.
	//
	// The delay is skipped after a chunk which discarded nothing.
	Delay time.Duration
}

// FstrimChunked discards unused blocks of the mounted filesystem at the given path,
// splitting the filesystem into ranges of opts.ChunkSize bytes, and issuing a separate FITRIM call
// for each range with opts.Delay delay in between.
//
// This bounds the amount of discards the kernel issues back-to-back, and allows
// the operation to be canceled via the context between the chunks (a single FITRIM
// call can't be interrupted).
//
// The chunking is based on the filesystem size reported by statfs(2), which matches the
// FITRIM address space for filesystems mapping it directly to the device (ext4, XFS, f2fs, ...).
// For btrfs the FITRIM range is a logical address space which is not bounded by the filesystem
// size, so the final chunk might cover most of the filesystem in a single call.
//
// It returns the total number of bytes discarded as reported by the kernel. If the context
// is canceled, the number of bytes discarded so far is returned along with the context error.
//
// See Fstrim for the other returned errors.
func FstrimChunked(ctx context.Context, path string, opts Options) (uint64, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return 0, err
	}

	defer f.Close() //nolint:errcheck

	return FstrimFdChunked(ctx, f.Fd(), opts)
}

// FstrimFdChunked discards unused blocks of the mounted filesystem identified by the
// open file descriptor in chunks.
//
// See FstrimChunked for the details, and Fstrim for the returned value and errors.
func FstrimFdChunked(ctx context.Context, fd uintptr, opts Options) (uint64, error) {
	if opts.ChunkSize == 0 {
		if err := ctx.Err(); err != nil {
			return 0, err
		}

		return fitrim(fd, 0, math.MaxUint64, opts.MinLength)
	}

	// statfs size is only an estimate of the FITRIM address space (e.g. XFS and ext4 exclude
	// the log/metadata overhead, so it's an underestimate), so it's used only to decide where
	// to stop chunking: the last call always covers everything up to the end of the address space.
	var st unix.Statfs_t

	if err := unix.Fstatfs(int(fd), &st); err != nil {
		return 0, fmt.Errorf("failed to statfs: %w", err)
	}

	// f_blocks is in units of f_frsize (the kernel defaults it to f_bsize if the filesystem doesn't set it).
	blockSize := uint64(st.Frsize)
	if blockSize == 0 {
		blockSize = uint64(st.Bsize)
	}

	fsSize := st.Blocks * blockSize

	var total uint64

	for start := uint64(0); ; start += opts.ChunkSize {
		// a single FITRIM call can't be interrupted, so check for cancellation before issuing it
		if err := ctx.Err(); err != nil {
			return total, err
		}

		length := opts.ChunkSize
		last := start+opts.ChunkSize >= fsSize

		if last {
			length = math.MaxUint64 - start
		}

		trimmed, err := fitrim(fd, start, length, opts.MinLength)
		if err != nil {
			return total, err
		}

		total += trimmed

		if last {
			return total, nil
		}

		// nothing was discarded, so there's no I/O to yield to
		if trimmed == 0 {
			continue
		}

		if err = sleep(ctx, opts.Delay); err != nil {
			return total, err
		}
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// fitrim issues a single FITRIM ioctl for the given range, returning the number of bytes discarded.
func fitrim(fd uintptr, start, length, minlen uint64) (uint64, error) {
	r := fstrimRange{
		Start:  start,
		Len:    length,
		Minlen: minlen,
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
