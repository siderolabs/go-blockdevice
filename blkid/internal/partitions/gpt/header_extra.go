// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gpt

import (
	"hash/crc32"
	"slices"
)

func (h Header) calculateChecksum() uint32 {
	b := slices.Clone(h[:HEADER_SIZE])

	b[16] = 0
	b[17] = 0
	b[18] = 0
	b[19] = 0

	return crc32.ChecksumIEEE(b)
}
