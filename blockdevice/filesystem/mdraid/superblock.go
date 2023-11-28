// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package mdraid

import (
	"encoding/binary"
)

const (
	// Magic is the mdraid magic signature.
	Magic = 0xa92b4efc
)

// SuperBlock represents the swap super block.
// https://raid.wiki.kernel.org/index.php/RAID_superblock_formats
type SuperBlock struct {
	Magic   [4]uint8
	Version [4]uint8
	Feature uint32
	_       uint32
	UUID    [16]uint8
	Name    [32]uint8
	CTime   [2]uint32
	Level   uint32
}

// Is implements the SuperBlocker interface.
func (sb *SuperBlock) Is() bool {
	// if binary.LittleEndian.Uint32(sb.Magic[:]) == Magic {
	// 	log.Printf("Data: %+v", sb)
	// 	log.Printf("Version: %d", binary.LittleEndian.Uint32(sb.Version[:]))
	// 	log.Printf("Name: %s", bytes.Trim(sb.Name[:], "\x00"))
	// 	uid, _ := uuid.FromBytes(sb.UUID[:])
	// 	log.Printf("UUID: %s", uid.String())
	// }

	return binary.LittleEndian.Uint32(sb.Version[:]) == 1 && binary.LittleEndian.Uint32(sb.Magic[:]) == Magic
}

// Offset implements the SuperBlocker interface.
func (sb *SuperBlock) Offset() int64 {
	return 0x1000
}

// Type implements the SuperBlocker interface.
func (sb *SuperBlock) Type() string {
	return "mdraid"
}

// Encrypted implements the SuperBlocker interface.
func (sb *SuperBlock) Encrypted() bool {
	return false
}
