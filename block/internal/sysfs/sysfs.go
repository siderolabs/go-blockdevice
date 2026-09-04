// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package sysfs implements reading block device attributes the kernel exposes in sysfs.
//
// Every function takes the sysfs path of the device it operates on (e.g. /sys/dev/block/8:0),
// so that it can be pointed at a tree other than /sys.
package sysfs

import (
	"bytes"
	"os"
)

// ReadFile returns the contents of a sysfs attribute with the trailing whitespace trimmed.
//
// An attribute which can't be read (most often, one which isn't there for this device) reads as empty.
func ReadFile(path string) string {
	contents, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return string(bytes.TrimSpace(contents))
}
