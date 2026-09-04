// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package sysfs

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// PartitionDevices returns a map of partition number to the kernel device name of the partition device
// for the device at the given sysfs path.
//
// For a device-mapper device, kernel partitions never exist (the kernel sets GENHD_FL_NO_PART on every
// device-mapper gendisk), so partitions are separate device-mapper devices carrying the UUID
// "part<N>-<parent UUID>" (as created by kpartx); they are enumerated via holders/.
// For any other device, kernel partitions are enumerated (see KernelPartitionDevices).
func PartitionDevices(sysFsPath string) (map[uint]string, error) {
	// only a device-mapper device gets device-mapper partition maps: a non-dm device might have
	// kpartx maps layered on top of it (UUID "part<N>-devnode_<major>:<minor>_..."), but it also has
	// real kernel partitions, and those are the devices to use.
	if dmUUID := ReadFile(filepath.Join(sysFsPath, "dm", "uuid")); dmUUID != "" {
		return deviceMapperPartitionDevices(sysFsPath, dmUUID)
	}

	return KernelPartitionDevices(sysFsPath)
}

// KernelPartitionDevices returns the kernel partitions of the device at the given sysfs path:
// sub-directories carrying a 'partition' attribute.
func KernelPartitionDevices(sysFsPath string) (map[uint]string, error) {
	contents, err := os.ReadDir(sysFsPath)
	if err != nil {
		return nil, err
	}

	result := map[uint]string{}

	for _, entry := range contents {
		if !entry.IsDir() {
			continue
		}

		partNo, ok := parsePartitionNumber(ReadFile(filepath.Join(sysFsPath, entry.Name(), "partition")))
		if !ok {
			continue
		}

		result[partNo] = entry.Name()
	}

	return result, nil
}

// deviceMapperPartitionDevices returns the device-mapper partition maps of the device-mapper device
// with the given UUID: holders carrying the UUID "part<N>-<parent UUID>".
func deviceMapperPartitionDevices(sysFsPath, dmUUID string) (map[uint]string, error) {
	holdersPath := filepath.Join(sysFsPath, "holders")

	contents, err := os.ReadDir(holdersPath)
	if err != nil {
		if os.IsNotExist(err) {
			// no holders at all, so no partition maps
			return map[uint]string{}, nil
		}

		return nil, err
	}

	result := map[uint]string{}

	for _, entry := range contents {
		holderUUID := ReadFile(filepath.Join(holdersPath, entry.Name(), "dm", "uuid"))
		if holderUUID == "" {
			continue
		}

		partNo, parentUUID, ok := ParseDeviceMapperPartitionUUID(holderUUID)
		if !ok || parentUUID != dmUUID {
			// not a partition map, or a partition map of some other device
			continue
		}

		result[partNo] = entry.Name()
	}

	return result, nil
}

// ParseDeviceMapperPartitionUUID parses a device-mapper partition map UUID in the form
// "part<N>-<parent UUID>", as created by kpartx, returning the partition number and the UUID of the
// parent device.
func ParseDeviceMapperPartitionUUID(uuid string) (uint, string, bool) {
	rest, ok := strings.CutPrefix(uuid, "part")
	if !ok {
		return 0, "", false
	}

	partNoStr, parentUUID, ok := strings.Cut(rest, "-")
	if !ok || parentUUID == "" {
		return 0, "", false
	}

	partNo, ok := parsePartitionNumber(partNoStr)
	if !ok {
		return 0, "", false
	}

	return partNo, parentUUID, true
}

// parsePartitionNumber parses a partition number, rejecting anything which is not a positive decimal number.
func parsePartitionNumber(s string) (uint, bool) {
	partNo, err := strconv.ParseUint(s, 10, 32)
	if err != nil || partNo == 0 {
		return 0, false
	}

	return uint(partNo), true
}
