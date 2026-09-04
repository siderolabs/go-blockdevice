// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package sysfs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/siderolabs/go-blockdevice/v2/block/internal/sysfs"
)

const mpathUUID = "mpath-36005076810800567c8000000000009fb"

// buildSysFs materializes a fake sysfs tree: files are created (with their parent directories),
// then symlinks are created pointing to the given (relative) targets.
//
// It returns the root of the tree.
func buildSysFs(t *testing.T, files, links map[string]string) string {
	t.Helper()

	root := t.TempDir()

	for path, contents := range files {
		full := filepath.Join(root, path)

		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(contents), 0o644))
	}

	for path, target := range links {
		full := filepath.Join(root, path)

		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.Symlink(target, full))
	}

	return root
}

//nolint:maintidx
func TestPartitionDevices(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		// contents of the fake sysfs tree, relative to its root
		files map[string]string
		// symlinks of the fake sysfs tree, relative to its root
		links map[string]string

		expected map[uint]string
		// expected result of KernelPartitionDevices, when it differs from PartitionDevices
		expectedKernel map[uint]string

		name string
		// the device to resolve the partitions for, relative to the root of the tree
		device string
	}{
		{
			name: "nvme disk with kernel partitions",
			files: map[string]string{
				"nvme0n1/size":                 "1000215216\n",
				"nvme0n1/ro":                   "0\n",
				"nvme0n1/queue/rotational":     "0\n",
				"nvme0n1/power/runtime_status": "active\n",
				"nvme0n1/nvme0n1p1/partition":  "1\n",
				"nvme0n1/nvme0n1p1/start":      "2048\n",
				"nvme0n1/nvme0n1p2/partition":  "2\n",
				"nvme0n1/nvme0n1p5/partition":  "5\n",
			},
			links: map[string]string{
				"nvme0n1/subsystem": "../../../class/block",
			},
			device: "nvme0n1",

			expected: map[uint]string{
				1: "nvme0n1p1",
				2: "nvme0n1p2",
				5: "nvme0n1p5",
			},
		},
		{
			name: "sata disk with kernel partitions",
			files: map[string]string{
				"sda/size":      "3907029168\n",
				"sda/sda1/size": "204800\n",
				// no 'partition' attribute: not a partition
				"sda/holders/.keep":  "",
				"sda/slaves/.keep":   "",
				"sda/sda2/partition": "2\n",
			},
			device: "sda",

			expected: map[uint]string{
				2: "sda2",
			},
		},
		{
			name: "disk with no partitions",
			files: map[string]string{
				"sdb/size":  "3907029168\n",
				"sdb/ro":    "0\n",
				"sdb/queue": "",
			},
			device: "sdb",

			expected: map[uint]string{},
		},
		{
			name: "partition itself has no partitions",
			files: map[string]string{
				"sda/sda1/partition": "1\n",
				"sda/sda1/start":     "2048\n",
				"sda/sda1/size":      "204800\n",
			},
			device: "sda/sda1",

			expected: map[uint]string{},
		},
		{
			name: "malformed partition attribute is ignored",
			files: map[string]string{
				"sda/sda1/partition": "one\n",
				"sda/sda2/partition": "\n",
				"sda/sda3/partition": "0\n",
				"sda/sda4/partition": "+4\n",
				"sda/sda5/partition": "0x5\n",
				"sda/sda6/partition": "6\n",
			},
			device: "sda",

			expected: map[uint]string{
				6: "sda6",
			},
		},
		{
			name: "device-mapper multipath device with partition maps",
			files: map[string]string{
				"dm-0/dm/name": "mpatha\n",
				"dm-0/dm/uuid": mpathUUID + "\n",
				"dm-0/size":    "3907029168\n",
				"dm-1/dm/name": "mpatha-part1\n",
				"dm-1/dm/uuid": "part1-" + mpathUUID + "\n",
				"dm-2/dm/name": "mpatha-part12\n",
				"dm-2/dm/uuid": "part12-" + mpathUUID + "\n",
			},
			links: map[string]string{
				"dm-0/holders/dm-1": "../../dm-1",
				"dm-0/holders/dm-2": "../../dm-2",
				"dm-1/slaves/dm-0":  "../../dm-0",
				"dm-2/slaves/dm-0":  "../../dm-0",
			},
			device: "dm-0",

			expected: map[uint]string{
				1:  "dm-1",
				12: "dm-2",
			},
			expectedKernel: map[uint]string{},
		},
		{
			name: "device-mapper holders which are not partition maps are ignored",
			files: map[string]string{
				"dm-0/dm/name": "mpatha\n",
				"dm-0/dm/uuid": mpathUUID + "\n",
				// a partition map of this device
				"dm-1/dm/uuid": "part1-" + mpathUUID + "\n",
				// an LVM logical volume stacked on top of this device
				"dm-2/dm/uuid": "LVM-AbCdEf0001000200030004000500060007-lvname\n",
				// a dm-crypt device stacked on top of this device
				"dm-3/dm/uuid": "CRYPT-LUKS2-abcdef1234567890abcdef1234567890-cryptname\n",
				// a partition map of some *other* device with the very same partition number
				"dm-4/dm/uuid": "part1-mpath-36005076810800567c8000000000009f8\n",
				// the 'part-N-' spelling is not what kpartx writes, and doesn't reference this device
				"dm-5/dm/uuid": "part-2-" + mpathUUID + "\n",
				// a non device-mapper holder: a Linux software RAID array assembled on top of this device
				"md0/md/level": "raid1\n",
			},
			links: map[string]string{
				"dm-0/holders/dm-1": "../../dm-1",
				"dm-0/holders/dm-2": "../../dm-2",
				"dm-0/holders/dm-3": "../../dm-3",
				"dm-0/holders/dm-4": "../../dm-4",
				"dm-0/holders/dm-5": "../../dm-5",
				"dm-0/holders/md0":  "../../md0",
			},
			device: "dm-0",

			expected: map[uint]string{
				1: "dm-1",
			},
			expectedKernel: map[uint]string{},
		},
		{
			name: "device-mapper device without holders",
			files: map[string]string{
				"dm-0/dm/name": "mpatha\n",
				"dm-0/dm/uuid": mpathUUID + "\n",
				// kernel partitions never exist for a device-mapper device, but make sure they are
				// not picked up even if something looking like one is there
				"dm-0/dm-0p1/partition": "1\n",
			},
			device: "dm-0",

			expected: map[uint]string{},
			expectedKernel: map[uint]string{
				1: "dm-0p1",
			},
		},
		{
			name: "device-mapper partition map itself",
			files: map[string]string{
				"dm-1/dm/name": "mpatha-part1\n",
				"dm-1/dm/uuid": "part1-" + mpathUUID + "\n",
			},
			links: map[string]string{
				"dm-1/slaves/dm-0": "../../dm-0",
			},
			device: "dm-1",

			expected: map[uint]string{},
		},
		{
			name: "kpartx maps over a non device-mapper device are ignored",
			files: map[string]string{
				// kpartx over a loop device creates maps with the UUID 'part<N>-devnode_<major>:<minor>_<...>',
				// but the loop device carries real kernel partitions, and those are the ones to use
				"loop0/loop0p1/partition": "1\n",
				"dm-0/dm/uuid":            "part1-devnode_7:0_QY0zjmTfMLI9XApVhtCsQAcxbcvMHK6b\n",
			},
			links: map[string]string{
				"loop0/holders/dm-0": "../../dm-0",
			},
			device: "loop0",

			expected: map[uint]string{
				1: "loop0p1",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			devicePath := filepath.Join(buildSysFs(t, test.files, test.links), test.device)

			devices, err := sysfs.PartitionDevices(devicePath)
			require.NoError(t, err)

			assert.Equal(t, test.expected, devices)

			expectedKernel := test.expectedKernel
			if expectedKernel == nil {
				expectedKernel = test.expected
			}

			kernelDevices, err := sysfs.KernelPartitionDevices(devicePath)
			require.NoError(t, err)

			assert.Equal(t, expectedKernel, kernelDevices)
		})
	}
}

func TestPartitionDevicesMissingDevice(t *testing.T) {
	t.Parallel()

	devicePath := filepath.Join(t.TempDir(), "sda")

	_, err := sysfs.PartitionDevices(devicePath)
	assert.ErrorIs(t, err, os.ErrNotExist)

	_, err = sysfs.KernelPartitionDevices(devicePath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestParseDeviceMapperPartitionUUID(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		uuid string

		expectedParentUUID string
		expectedPartNo     uint
		expectedOk         bool
	}{
		{
			name: "mpath partition map",
			uuid: "part1-" + mpathUUID,

			expectedPartNo:     1,
			expectedParentUUID: mpathUUID,
			expectedOk:         true,
		},
		{
			name: "mpath partition map, two digits",
			uuid: "part12-" + mpathUUID,

			expectedPartNo:     12,
			expectedParentUUID: mpathUUID,
			expectedOk:         true,
		},
		{
			name: "kpartx over a loop device",
			uuid: "part3-devnode_7:0_QY0zjmTfMLI9XApVhtCsQAcxbcvMHK6b",

			expectedPartNo:     3,
			expectedParentUUID: "devnode_7:0_QY0zjmTfMLI9XApVhtCsQAcxbcvMHK6b",
			expectedOk:         true,
		},
		{
			name: "mpath whole device",
			uuid: mpathUUID,
		},
		{
			name: "LVM logical volume",
			uuid: "LVM-AbCdEf0001000200030004000500060007-lvname",
		},
		{
			name: "dm-crypt device",
			uuid: "CRYPT-LUKS2-abcdef1234567890abcdef1234567890-cryptname",
		},
		{
			name: "'part-N-' spelling is not what kpartx writes",
			uuid: "part-1-" + mpathUUID,
		},
		{
			name: "no partition number",
			uuid: "part-" + mpathUUID,
		},
		{
			name: "no parent UUID",
			uuid: "part1-",
		},
		{
			name: "no separator",
			uuid: "part1",
		},
		{
			name: "partition number zero",
			uuid: "part0-" + mpathUUID,
		},
		{
			name: "signed partition number",
			uuid: "part+1-" + mpathUUID,
		},
		{
			name: "device name, not a UUID",
			uuid: "partition1-foo",
		},
		{
			name: "empty",
			uuid: "",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			partNo, parentUUID, ok := sysfs.ParseDeviceMapperPartitionUUID(test.uuid)

			assert.Equal(t, test.expectedOk, ok)
			assert.Equal(t, test.expectedPartNo, partNo)
			assert.Equal(t, test.expectedParentUUID, parentUUID)
		})
	}
}
