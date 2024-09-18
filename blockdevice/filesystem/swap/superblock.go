// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package swap

import (
	"encoding/binary"
	"syscall"
)

const (
	// Magic is the swap magic signature.
	Magic     = "SWAPSPACE2"
	magicSize = len(Magic)
)

// SuperBlock represents the swap super block.
type SuperBlock struct {
	Varsion    uint32
	LastPage   uint32
	NrBadPages uint32
	UUID       [0x10]uint8 // 1036
	Label      [0x10]uint8 // 1052
	Padding    [0x75]uint32
	BadPages   [1]uint32
}

// Is implements the SuperBlocker interface.
func (sb *SuperBlock) Is() bool {
	return sb.Varsion == binary.BigEndian.Uint32([]byte{1, 0, 0, 0})
}

// Offset implements the SuperBlocker interface.
func (sb *SuperBlock) Offset() int64 {
	return 0x400
}

// OffsetMagic implements the SuperBlocker interface.
func (sb *SuperBlock) OffsetMagic() int64 {
	return int64(syscall.Getpagesize()) - int64(magicSize)
}

// Type implements the SuperBlocker interface.
func (sb *SuperBlock) Type() string {
	return "swap"
}

// Encrypted implements the SuperBlocker interface.
func (sb *SuperBlock) Encrypted() bool {
	return false
}
