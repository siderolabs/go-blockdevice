// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package utils provides utility functions.
package utils

import (
	"hash/crc32"
	"sync"
)

var castagnoliTable = sync.OnceValue(func() *crc32.Table {
	return crc32.MakeTable(crc32.Castagnoli)
})

// CRC32c returns values compatible with Linux crc32c function.
func CRC32c(buf []byte) uint32 {
	return ^crc32.Update(0, castagnoliTable(), buf)
}

// IsPowerOf2 returns true if num is a power of 2.
func IsPowerOf2[T uint8 | uint16 | uint32 | uint64](num T) bool {
	return (num != 0 && ((num & (num - 1)) == 0))
}
