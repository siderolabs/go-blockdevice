// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package block

func isPowerOf2[T uint8 | uint16 | uint32 | uint64](num T) bool {
	return (num != 0 && ((num & (num - 1)) == 0))
}
