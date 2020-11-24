// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package lba

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// NewLBA initializes and returns an `LBA`.
func NewLBA(f *os.File) (lba *LBA, err error) {
	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat disk error: %w", err)
	}

	var psize int64
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), unix.BLKPBSZGET, uintptr(unsafe.Pointer(&psize))); errno != 0 {
		if st.Mode().IsRegular() {
			// Not a device, assume default block size.
			psize = 512
		} else {
			return nil, errors.New("BLKPBSZGET failed")
		}
	}

	var lsize int64
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), unix.BLKSSZGET, uintptr(unsafe.Pointer(&lsize))); errno != 0 {
		if st.Mode().IsRegular() {
			// Not a device, assume default block size.
			lsize = 512
		} else {
			return nil, errors.New("BLKSSZGET failed")
		}
	}

	// Seek to the end to get the size.
	size, err := f.Seek(0, 2)
	if err != nil {
		return nil, err
	}

	// Reset by seeking to the beginning.
	_, err = f.Seek(0, 0)
	if err != nil {
		return nil, err
	}

	tsize := size / lsize

	lba = &LBA{
		PhysicalBlockSize: psize,
		LogicalBlockSize:  lsize,
		TotalSectors:      tsize,
		f:                 f,
	}

	return lba, nil
}

// ReadAt reads from a file in units of LBA.
func (l *LBA) ReadAt(lba, off, length int64) (b []byte, err error) {
	b = make([]byte, length)

	off = lba*l.LogicalBlockSize + off

	n, err := l.f.ReadAt(b, off)
	if err != nil {
		return nil, err
	}

	if n != len(b) {
		return nil, fmt.Errorf("expected to read %d bytes, read %d", len(b), n)
	}

	return b, nil
}

// WriteAt writes to a file in units of LBA.
func (l *LBA) WriteAt(lba, off int64, b []byte) (err error) {
	off = lba*l.LogicalBlockSize + off

	n, err := l.f.WriteAt(b, off)
	if err != nil {
		return err
	}

	if n != len(b) {
		return fmt.Errorf("expected to write %d bytes, read %d", len(b), n)
	}

	return nil
}
