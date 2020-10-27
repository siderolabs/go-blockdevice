// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package blkpg

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/talos-systems/go-retry/retry"
	"golang.org/x/sys/unix"

	"github.com/talos-systems/go-blockdevice/blockdevice/lba"
	"github.com/talos-systems/go-blockdevice/blockdevice/table"
	"github.com/talos-systems/go-blockdevice/blockdevice/util"
)

// InformKernelOfAdd invokes the BLKPG_ADD_PARTITION ioctl.
func InformKernelOfAdd(f *os.File, partition table.Partition) error {
	return inform(f, partition, unix.BLKPG_ADD_PARTITION)
}

// InformKernelOfResize invokes the BLKPG_RESIZE_PARTITION ioctl.
func InformKernelOfResize(f *os.File, partition table.Partition) error {
	return inform(f, partition, unix.BLKPG_RESIZE_PARTITION)
}

// InformKernelOfDelete invokes the BLKPG_DEL_PARTITION ioctl.
func InformKernelOfDelete(f *os.File, partition table.Partition) error {
	return inform(f, partition, unix.BLKPG_DEL_PARTITION)
}

func inform(f *os.File, partition table.Partition, op int32) (err error) {
	var (
		start  int64
		length int64
	)

	switch op {
	case unix.BLKPG_DEL_PARTITION:
		start = 0
		length = 0
	default:
		var l *lba.LogicalBlockAddresser

		if l, err = lba.New(f); err != nil {
			return err
		}

		blocksize := int64(l.LogicalBlockSize)

		start = partition.Start() * blocksize
		length = partition.Length() * blocksize
	}

	data := &unix.BlkpgPartition{
		Start:  start,
		Length: length,
		Pno:    partition.No(),
	}

	arg := &unix.BlkpgIoctlArg{
		Op:      op,
		Datalen: int32(unsafe.Sizeof(*data)),
		Data:    (*byte)(unsafe.Pointer(data)),
	}

	err = retry.Constant(10*time.Second, retry.WithUnits(500*time.Millisecond)).Retry(func() error {
		_, _, errno := syscall.Syscall(
			syscall.SYS_IOCTL,
			f.Fd(),
			unix.BLKPG,
			uintptr(unsafe.Pointer(arg)),
		)

		if errno != 0 {
			switch errno { //nolint: exhaustive
			case unix.EBUSY:
				return retry.ExpectedError(errno)
			default:
				return retry.UnexpectedError(errno)
			}
		}

		if err = f.Sync(); err != nil {
			return retry.UnexpectedError(err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to inform kernel: %w", err)
	}

	return nil
}

// GetKernelPartitions returns kernel partition table state.
func GetKernelPartitions(f *os.File, devPath string) ([]KernelPartition, error) {
	result := []KernelPartition{}

	for i := 1; i <= 256; i++ {
		partName := util.PartName(devPath, i)
		partPath := filepath.Join("/sys/block", filepath.Base(devPath), partName)

		_, err := os.Stat(partPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}

			return nil, err
		}

		startS, err := ioutil.ReadFile(filepath.Join(partPath, "start"))
		if err != nil {
			return nil, err
		}

		sizeS, err := ioutil.ReadFile(filepath.Join(partPath, "size"))
		if err != nil {
			return nil, err
		}

		start, err := strconv.ParseInt(strings.TrimSpace(string(startS)), 10, 64)
		if err != nil {
			return nil, err
		}

		size, err := strconv.ParseInt(strings.TrimSpace(string(sizeS)), 10, 64)
		if err != nil {
			return nil, err
		}

		result = append(result, KernelPartition{
			No:     i,
			Start:  start,
			Length: size,
		})
	}

	return result, nil
}
