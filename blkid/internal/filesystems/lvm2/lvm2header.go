// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Code generated by "cstruct -pkg lvm2 -struct LVM2Header -input lvm2_header.h -endianness LittleEndian"; DO NOT EDIT.

package lvm2

import "encoding/binary"

var _ = binary.LittleEndian

// LVM2Header is a byte slice representing the lvm2_header.h C header.
type LVM2Header []byte

// Get_id returns LABELONE.
func (s LVM2Header) Get_id() []byte {
	return s[0:8]
}

// Put_id sets LABELONE.
func (s LVM2Header) Put_id(v []byte) {
	copy(s[0:8], v)
}

// Get_sector_xl returns Sector number of this label.
func (s LVM2Header) Get_sector_xl() uint64 {
	return binary.LittleEndian.Uint64(s[8:16])
}

// Put_sector_xl sets Sector number of this label.
func (s LVM2Header) Put_sector_xl(v uint64) {
	binary.LittleEndian.PutUint64(s[8:16], v)
}

// Get_crc_xl returns From next field to end of sector.
func (s LVM2Header) Get_crc_xl() uint32 {
	return binary.LittleEndian.Uint32(s[16:20])
}

// Put_crc_xl sets From next field to end of sector.
func (s LVM2Header) Put_crc_xl(v uint32) {
	binary.LittleEndian.PutUint32(s[16:20], v)
}

// Get_offset_xl returns Offset from start of struct to contents.
func (s LVM2Header) Get_offset_xl() uint32 {
	return binary.LittleEndian.Uint32(s[20:24])
}

// Put_offset_xl sets Offset from start of struct to contents.
func (s LVM2Header) Put_offset_xl(v uint32) {
	binary.LittleEndian.PutUint32(s[20:24], v)
}

// Get_type returns LVM2 001.
func (s LVM2Header) Get_type() []byte {
	return s[24:32]
}

// Put_type sets LVM2 001.
func (s LVM2Header) Put_type(v []byte) {
	copy(s[24:32], v)
}

// Get_pv_uuid returns pv_uuid.
func (s LVM2Header) Get_pv_uuid() []byte {
	return s[32:64]
}

// Put_pv_uuid sets pv_uuid.
func (s LVM2Header) Put_pv_uuid(v []byte) {
	copy(s[32:64], v)
}

// LVM2HEADER_SIZE is the size of the LVM2Header struct.
const LVM2HEADER_SIZE = 64