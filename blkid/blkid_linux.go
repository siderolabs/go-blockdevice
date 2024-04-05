// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build linux

package blkid

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/siderolabs/go-blockdevice/v2/block"
)

// ProbePath returns the probe information for the specified path.
func ProbePath(devpath string) (*Info, error) {
	f, err := os.OpenFile(devpath, os.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}

	defer f.Close() //nolint:errcheck

	return Probe(f)
}

// Probe returns the probe information for the specified file.
func Probe(f *os.File) (*Info, error) {
	unix.Fadvise(int(f.Fd()), 0, 0, unix.FADV_RANDOM) //nolint:errcheck // best-effort: we don't care if this fails

	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat: %w", err)
	}

	info := &Info{}

	sysStat := st.Sys().(*syscall.Stat_t) //nolint:errcheck,forcetypeassert // we know it's a syscall.Stat_t

	switch sysStat.Mode & unix.S_IFMT {
	case unix.S_IFBLK:
		// block device, initialize full support
		info.BlockDevice = block.NewFromFile(f)
		info.DevNo = sysStat.Rdev

		if size, err := info.BlockDevice.GetSize(); err == nil {
			info.Size = size
		} else {
			return nil, fmt.Errorf("failed to get block device size: %w", err)
		}

		if ioSize, err := info.BlockDevice.GetIOSize(); err == nil {
			info.IOSize = ioSize
		} else {
			return nil, fmt.Errorf("failed to get block device I/O size: %w", err)
		}

		info.SectorSize = info.BlockDevice.GetSectorSize()

		info.WholeDisk = info.isWholeDisk()
	case unix.S_IFREG:
		// regular file (an image?), so use different settings
		info.Size = uint64(st.Size())
		info.IOSize = block.DefaultBlockSize
		info.SectorSize = block.DefaultBlockSize
	default:
		return nil, fmt.Errorf("unsupported file type: %s", st.Mode().Type())
	}

	if info.isPrivateDeviceMapper() {
		// don't probe device-mapper devices
		return info, nil
	}

	if info.BlockDevice != nil && info.isWholeDisk() {
		if info.BlockDevice.IsCD() && info.BlockDevice.IsCDNoMedia() {
			// don't probe CD-ROM devices without media
			return info, nil
		}
	}

	if err := info.fillProbeResult(f); err != nil {
		return nil, fmt.Errorf("failed to probe: %w", err)
	}

	return info, nil
}

func (i *Info) sysFsPath() string {
	return fmt.Sprintf("/sys/dev/block/%d:%d", unix.Major(i.DevNo), unix.Minor(i.DevNo))
}

func (i *Info) isPrivateDeviceMapper() bool {
	if i.DevNo == 0 {
		return false
	}

	sysFsPath := i.sysFsPath()

	contents, err := os.ReadFile(filepath.Join(sysFsPath, "dm", "uuid"))
	if err != nil {
		return false
	}

	// check for pattern "LVM-<uuid>-name"
	prefix, rest, ok := bytes.Cut(contents, []byte("-"))
	if !ok {
		return false
	}

	if !bytes.Equal(prefix, []byte("LVM")) {
		return false
	}

	_, _, ok = bytes.Cut(rest, []byte("-"))

	return ok
}

func (i *Info) isWholeDisk() bool {
	if i.DevNo == 0 {
		return false
	}

	sysFsPath := i.sysFsPath()

	// check if this is a partition
	_, err := os.Stat(filepath.Join(sysFsPath, "partition"))
	isPartition := err == nil

	if isPartition {
		return false
	}

	// device-mapper check
	contents, err := os.ReadFile(filepath.Join(sysFsPath, "dm", "uuid"))
	if err != nil {
		// not devmapper
		return true
	}

	return !bytes.HasPrefix(contents, []byte("part-"))
}
