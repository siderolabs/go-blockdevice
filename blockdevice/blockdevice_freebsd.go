// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package blockdevice

import (
	"fmt"
	"os"

	"github.com/talos-systems/go-blockdevice/blockdevice/partition/gpt"
)

// BlockDevice represents a block device.
type BlockDevice struct {
	table *gpt.GPT

	f *os.File
}

const (
	// ReadonlyMode readonly mode.
	ReadonlyMode = os.O_RDONLY
	// DefaultMode read write.
	DefaultMode = os.O_RDWR
)

// Open initializes and returns a block device.
// TODO(andrewrynhard): Use BLKGETSIZE ioctl to get the size.
func Open(devname string, setters ...Option) (bd *BlockDevice, err error) {
	return nil, fmt.Errorf("not implemented")
}

// Close closes the block devices's open file.
func (bd *BlockDevice) Close() error {
	return fmt.Errorf("not implemented")
}

// PartitionTable returns the block device partition table.
func (bd *BlockDevice) PartitionTable() (*gpt.GPT, error) {
	return nil, fmt.Errorf("not implemented")
}

// RereadPartitionTable invokes the BLKRRPART ioctl to have the kernel read the
// partition table.
//
// NB: Rereading the partition table requires that all partitions be
// unmounted or it will fail with EBUSY.
func (bd *BlockDevice) RereadPartitionTable() error {
	return fmt.Errorf("not implemented")
}

// Device returns the backing file for the block device.
func (bd *BlockDevice) Device() *os.File {
	return nil
}

// Size returns the size of the block device in bytes.
func (bd *BlockDevice) Size() (uint64, error) {
	return 0, fmt.Errorf("not implemented")
}

// Reset will reset a block device given a device name.
// Simply deletes partition table on device.
func (bd *BlockDevice) Reset() error {
	return fmt.Errorf("not implemented")
}

// Wipe the blockdevice contents.
func (bd *BlockDevice) Wipe() error {
	return fmt.Errorf("not implemented")
}

// OpenPartition opens another blockdevice using a partition of this block device.
func (bd *BlockDevice) OpenPartition(label string, setters ...Option) (*BlockDevice, error) {
	return nil, fmt.Errorf("not implemented")
}

// GetPartition returns partition by label if found.
func (bd *BlockDevice) GetPartition(label string) (*gpt.Partition, error) {
	return nil, fmt.Errorf("not implemented")
}

// PartPath returns partition path by label, verifies that partition exists.
func (bd *BlockDevice) PartPath(label string) (string, error) {
	return "", fmt.Errorf("not implemented")
}
