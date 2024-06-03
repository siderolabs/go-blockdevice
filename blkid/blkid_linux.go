// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build linux

package blkid

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/siderolabs/go-blockdevice/v2/block"
)

// ProbePath returns the probe information for the specified path.
func ProbePath(devpath string, opts ...ProbeOption) (*Info, error) {
	f, err := os.OpenFile(devpath, os.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}

	defer f.Close() //nolint:errcheck

	return Probe(f, opts...)
}

// Probe returns the probe information for the specified file.
//
//nolint:cyclop
func Probe(f *os.File, opts ...ProbeOption) (*Info, error) {
	options := applyProbeOptions(opts...)

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

		info.DevNo, err = info.BlockDevice.GetDevNo()
		if err != nil {
			return nil, fmt.Errorf("failed to get device number: %w", err)
		}

		var (
			size   uint64
			ioSize uint
		)

		if size, err = info.BlockDevice.GetSize(); err == nil {
			info.Size = size
		} else {
			return nil, fmt.Errorf("failed to get block device size: %w", err)
		}

		if ioSize, err = info.BlockDevice.GetIOSize(); err == nil {
			info.IOSize = ioSize
		} else {
			return nil, fmt.Errorf("failed to get block device I/O size: %w", err)
		}

		info.SectorSize = info.BlockDevice.GetSectorSize()

		info.WholeDisk, err = info.BlockDevice.IsWholeDisk()
		if err != nil {
			return nil, fmt.Errorf("failed to check if block device is whole disk: %w", err)
		}
	case unix.S_IFREG:
		// regular file (an image?), so use different settings
		info.Size = uint64(st.Size())
		info.IOSize = block.DefaultBlockSize
		info.SectorSize = block.DefaultBlockSize
	default:
		return nil, fmt.Errorf("unsupported file type: %s", st.Mode().Type())
	}

	if info.BlockDevice != nil {
		if private, err := info.BlockDevice.IsPrivateDeviceMapper(); private && err == nil {
			// don't probe device-mapper devices
			options.Logger.Debug("skipping private device-mapper device")

			return info, nil
		}
	}

	if info.WholeDisk && info.BlockDevice.IsCD() && info.BlockDevice.IsCDNoMedia() {
		// don't probe CD-ROM devices without media
		options.Logger.Debug("skipping CD-ROM device without media")

		return info, nil
	}

	if !options.SkipLocking && info.BlockDevice != nil {
		// we need to lock the whole disk device (if probing a partition, we lock the whole disk)
		wholeDisk, err := info.BlockDevice.GetWholeDisk()
		if err != nil {
			return nil, fmt.Errorf("failed to get whole disk: %w", err)
		}

		defer wholeDisk.Close() //nolint:errcheck

		if err = wholeDisk.TryLock(false); err != nil {
			if errors.Is(err, unix.EWOULDBLOCK) {
				return nil, ErrFailedLock
			}

			return nil, fmt.Errorf("failed to lock whole disk: %w", err)
		}

		defer wholeDisk.Unlock() //nolint:errcheck
	}

	if err := info.fillProbeResult(f); err != nil {
		return nil, fmt.Errorf("failed to probe: %w", err)
	}

	return info, nil
}
