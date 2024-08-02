// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package partitioning implements common partitioning functions.
package partitioning

import "strconv"

// DevName returns the devname for the partition on a disk.
func DevName(device string, part uint) string {
	result := device

	if len(result) > 0 && result[len(result)-1] >= '0' && result[len(result)-1] <= '9' {
		result += "p"
	}

	return result + strconv.FormatUint(uint64(part), 10)
}
