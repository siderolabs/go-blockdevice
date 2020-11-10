// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Disk reresents disk information obtained by reading /sys/block.
type Disk struct {
	// Size disk size in bytes.
	Size uint64
	// Model from /sys/block/*/device/model.
	Model string
	// DeviceName device name (e.g. /dev/sda).
	DeviceName string
}

// GetDisks returns list of disks by reading /sys/block.
func GetDisks() ([]*Disk, error) {
	disks := []*Disk{}

	sysblock := "/sys/block"

	devices, err := ioutil.ReadDir(sysblock)
	if err != nil {
		return nil, fmt.Errorf("failed to read /sys/block directory %w", err)
	}

	readFile := func(path string) (string, error) {
		f, e := os.Open(filepath.Join(sysblock, path))

		if e != nil {
			return "", fmt.Errorf("failed to open file %w", err)
		}

		data, e := ioutil.ReadAll(f)

		if e != nil {
			return "", fmt.Errorf("failed to read file %w", err)
		}

		return string(data), nil
	}

	for _, dev := range devices {
		skip := false
		deviceName := filepath.Base(dev.Name())

		for _, prefix := range []string{"sg", "sr", "loop", "md", "dm-"} {
			if strings.HasPrefix(deviceName, prefix) {
				skip = true

				break
			}
		}

		if skip {
			continue
		}

		var size uint64

		s, err := readFile(filepath.Join(dev.Name(), "size"))
		if err != nil {
			continue
		}

		size, err = strconv.ParseUint(strings.TrimSpace(s), 10, 64)
		if err != nil {
			continue
		}

		model, err := readFile(filepath.Join(dev.Name(), "device/model"))
		if err != nil {
			model = ""
		}

		disks = append(disks, &Disk{
			DeviceName: fmt.Sprintf("/dev/%s", deviceName),
			Size:       size,
			Model:      model,
		})
	}

	return disks, nil
}
