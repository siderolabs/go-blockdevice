// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package partitioning implements common partitioning functions.
package partitioning

import "strconv"

// DevName returns the devname for the partition on a disk.
//
// Deprecated: this composes the partition device name from the disk device name, which only holds for
// devices with kernel partitions: a device-mapper device has no partition device nodes at all, and its
// partitions are separate device-mapper devices with unrelated names. Use block.Device.GetPartitionDevName
// to resolve the partition device from the kernel instead.
//
// It stays valid for disk images, which have no device nodes to resolve.
func DevName(device string, part uint) string {
	result := device

	if len(result) > 0 && result[len(result)-1] >= '0' && result[len(result)-1] <= '9' {
		result += "p"
	}

	return result + strconv.FormatUint(uint64(part), 10)
}
