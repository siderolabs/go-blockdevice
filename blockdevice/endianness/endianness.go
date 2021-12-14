// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package endianness

import (
	"encoding/binary"
)

// ToMiddleEndian converts a byte slice representation of a UUID to a
// middle-endian byte slice representation of a UUID.
func ToMiddleEndian(data []byte) []byte {
	b := make([]byte, 16)

	// timeLow
	binary.LittleEndian.PutUint32(b, binary.BigEndian.Uint32(data[0:4]))
	// timeMid
	binary.LittleEndian.PutUint16(b[4:], binary.BigEndian.Uint16(data[4:6]))
	// timeHigh
	binary.LittleEndian.PutUint16(b[6:], binary.BigEndian.Uint16(data[6:8]))
	// clockSeqHi,clockSeqLo,node
	copy(b[8:], data[8:16])

	return b
}

// FromMiddleEndian converts a middle-endian byte slice representation of a
// UUID to a big-endian byte slice representation of a UUID.
func FromMiddleEndian(data []byte) []byte {
	b := make([]byte, 16)

	// timeLow
	binary.BigEndian.PutUint32(b, binary.LittleEndian.Uint32(data[0:4]))
	// timeMid
	binary.BigEndian.PutUint16(b[4:], binary.LittleEndian.Uint16(data[4:6]))
	// timeHigh
	binary.BigEndian.PutUint16(b[6:], binary.LittleEndian.Uint16(data[6:8]))
	// clockSeqHi,clockSeqLo,node
	copy(b[8:], data[8:16])

	return b
}
