// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package blockdevice

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"
	"unsafe"

	"github.com/talos-systems/go-retry/retry"
	"golang.org/x/sys/unix"

	"github.com/talos-systems/go-blockdevice/blockdevice/partition/gpt"
)

// Linux headers constants.
//
// Hardcoded here to avoid CGo dependency.
const (
	BLKDISCARD       = 4727
	BLKDISCARDZEROES = 4732
	BLKSECDISCARD    = 4733
	BLKZEROOUT       = 4735
)

// Fast wipe parameters.
const (
	FastWipeRange = 1024 * 1024
)

// BlockDevice represents a block device.
type BlockDevice struct {
	g *gpt.GPT

	f *os.File
}

// Open initializes and returns a block device.
// TODO(andrewrynhard): Use BLKGETSIZE ioctl to get the size.
func Open(devname string, setters ...Option) (bd *BlockDevice, err error) {
	opts := NewDefaultOptions(setters...)
	bd = &BlockDevice{}

	var f *os.File

	if f, err = os.OpenFile(devname, os.O_RDWR|unix.O_CLOEXEC, os.ModeDevice); err != nil {
		return nil, err
	}

	bd.f = f

	defer func() {
		if err != nil {
			//nolint: errcheck
			f.Close()
		}
	}()

	if opts.ExclusiveLock {
		if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
			err = fmt.Errorf("error locking device %q: %w", devname, err)

			return nil, err
		}
	}

	if opts.CreateGPT {
		var g *gpt.GPT

		g, err = gpt.New(f)
		if err != nil {
			return nil, err
		}

		if err = g.Write(); err != nil {
			return nil, err
		}

		bd.g = g
	} else {
		buf := make([]byte, 1)
		// PMBR protective entry starts at 446. The partition type is at offset
		// 4 from the start of the PMBR protective entry.
		_, err = f.ReadAt(buf, 450)
		if err != nil {
			return nil, err
		}

		// For GPT, the partition type should be 0xee (EFI GPT).
		if bytes.Equal(buf, []byte{0xee}) {
			var g *gpt.GPT
			if g, err = gpt.Open(f); err != nil {
				return nil, err
			}
			bd.g = g
		}
	}

	return bd, nil
}

// Close closes the block devices's open file.
func (bd *BlockDevice) Close() error {
	return bd.f.Close()
}

// PartitionTable returns the block device partition table.
func (bd *BlockDevice) PartitionTable() (*gpt.GPT, error) {
	if bd.g == nil {
		return nil, ErrMissingPartitionTable
	}

	return bd.g, bd.g.Read()
}

// RereadPartitionTable invokes the BLKRRPART ioctl to have the kernel read the
// partition table.
//
// NB: Rereading the partition table requires that all partitions be
// unmounted or it will fail with EBUSY.
func (bd *BlockDevice) RereadPartitionTable() error {
	// Flush the file buffers.
	// NOTE(andrewrynhard): I'm not entirely sure we need this, but
	// figured it wouldn't hurt.
	if err := bd.f.Sync(); err != nil {
		return err
	}
	// Flush the block device buffers.
	if _, _, ret := unix.Syscall(unix.SYS_IOCTL, bd.f.Fd(), unix.BLKFLSBUF, 0); ret != 0 {
		return fmt.Errorf("flush block device buffers: %v", ret)
	}

	var (
		err error
		ret syscall.Errno
	)

	// Reread the partition table.
	err = retry.Constant(5*time.Second, retry.WithUnits(50*time.Millisecond)).Retry(func() error {
		if _, _, ret = unix.Syscall(unix.SYS_IOCTL, bd.f.Fd(), unix.BLKRRPART, 0); ret == 0 {
			return nil
		}
		//nolint: exhaustive
		switch ret {
		case syscall.EBUSY:
			return retry.ExpectedError(err)
		default:
			return retry.UnexpectedError(err)
		}
	})
	if err != nil {
		return fmt.Errorf("failed to re-read partition table: %w", err)
	}

	return nil
}

// Device returns the backing file for the block device.
func (bd *BlockDevice) Device() *os.File {
	return bd.f
}

// Size returns the size of the block device in bytes.
func (bd *BlockDevice) Size() (uint64, error) {
	var devsize uint64
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, bd.f.Fd(), unix.BLKGETSIZE64, uintptr(unsafe.Pointer(&devsize))); errno != 0 {
		return 0, errno
	}

	return devsize, nil
}

// Wipe the blockdevice contents.
//
// In order of availability this tries to perform the following:
//   * secure discard (secure erase)
//   * discard with zeros
//   * zero out via ioctl
//   * zero out from userland
func (bd *BlockDevice) Wipe() (string, error) {
	size, err := bd.Size()
	if err != nil {
		return "", err
	}

	return bd.WipeRange(0, size)
}

// FastWipe the blockdevice contents.
//
// This method is much faster than Wipe(), but it doesn't guarantee
// that device will be zeroed out completely.
func (bd *BlockDevice) FastWipe() error {
	size, err := bd.Size()
	if err != nil {
		return err
	}

	// BLKDISCARD is implemented via TRIM on SSDs, it might or might not zero out device contents.
	r := [2]uint64{0, size}

	// ignoring the error here as DISCARD might be not supported by the device
	unix.Syscall(unix.SYS_IOCTL, bd.f.Fd(), BLKDISCARD, uintptr(unsafe.Pointer(&r[0]))) //nolint: errcheck

	// zero out the first N bytes of the device to clear any partition table
	wipeLength := uint64(FastWipeRange)

	if wipeLength > size {
		wipeLength = size
	}

	_, err = bd.WipeRange(0, wipeLength)

	return err
}

// WipeRange the blockdevice [start, start+length).
func (bd *BlockDevice) WipeRange(start, length uint64) (string, error) {
	var err error

	r := [2]uint64{start, length}

	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, bd.f.Fd(), BLKSECDISCARD, uintptr(unsafe.Pointer(&r[0]))); errno == 0 {
		return "blksecdiscard", nil
	}

	var zeroes int

	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, bd.f.Fd(), BLKDISCARDZEROES, uintptr(unsafe.Pointer(&zeroes))); errno == 0 && zeroes != 0 {
		if _, _, errno = unix.Syscall(unix.SYS_IOCTL, bd.f.Fd(), BLKDISCARD, uintptr(unsafe.Pointer(&r[0]))); errno == 0 {
			return "blkdiscardzeros", nil
		}
	}

	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, bd.f.Fd(), BLKZEROOUT, uintptr(unsafe.Pointer(&r[0]))); errno == 0 {
		return "blkzeroout", nil
	}

	var zero *os.File

	if zero, err = os.Open("/dev/zero"); err != nil {
		return "", err
	}

	defer zero.Close() //nolint: errcheck

	_, err = io.CopyN(bd.f, zero, int64(r[1]))

	return "writezeroes", err
}

// Reset will reset a block device given a device name.
// Simply deletes partition table on device.
func (bd *BlockDevice) Reset() error {
	g, err := bd.PartitionTable()
	if err != nil {
		return err
	}

	for _, p := range g.Partitions().Items() {
		if err = g.Delete(p); err != nil {
			return fmt.Errorf("failed to delete partition: %w", err)
		}
	}

	return g.Write()
}

// OpenPartition opens another blockdevice using a partition of this block device.
func (bd *BlockDevice) OpenPartition(label string, setters ...Option) (*BlockDevice, error) {
	g, err := bd.PartitionTable()
	if err != nil {
		return nil, err
	}

	part := g.Partitions().FindByName(label)
	if part == nil {
		return nil, os.ErrNotExist
	}

	path, err := part.Path()
	if err != nil {
		return nil, err
	}

	return Open(path, setters...)
}

// GetPartition returns partition by label if found.
func (bd *BlockDevice) GetPartition(label string) (*gpt.Partition, error) {
	g, err := bd.PartitionTable()
	if err != nil {
		return nil, err
	}

	part := g.Partitions().FindByName(label)

	if part == nil {
		return nil, os.ErrNotExist
	}

	return part, nil
}

// PartPath returns partition path by label, verifies that partition exists.
func (bd *BlockDevice) PartPath(label string) (string, error) {
	part, err := bd.GetPartition(label)
	if err != nil {
		return "", err
	}

	return part.Path()
}
