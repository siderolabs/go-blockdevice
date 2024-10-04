// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Code generated by "cstruct -pkg gptstructs -struct Entry -input entry.h -endianness LittleEndian"; DO NOT EDIT.

package gptstructs

import "encoding/binary"

var _ = binary.LittleEndian

// Entry is a byte slice representing the entry.h C header.
type Entry []byte

// Get_partition_type_guid returns type UUID.
func (s Entry) Get_partition_type_guid() []byte {
	return s[0:16]
}

// Put_partition_type_guid sets type UUID.
func (s Entry) Put_partition_type_guid(v []byte) {
	copy(s[0:16], v)
}

// Get_unique_partition_guid returns partition UUID.
func (s Entry) Get_unique_partition_guid() []byte {
	return s[16:32]
}

// Put_unique_partition_guid sets partition UUID.
func (s Entry) Put_unique_partition_guid(v []byte) {
	copy(s[16:32], v)
}

// Get_starting_lba returns starting_lba.
func (s Entry) Get_starting_lba() uint64 {
	return binary.LittleEndian.Uint64(s[32:40])
}

// Put_starting_lba sets starting_lba.
func (s Entry) Put_starting_lba(v uint64) {
	binary.LittleEndian.PutUint64(s[32:40], v)
}

// Get_ending_lba returns ending_lba.
func (s Entry) Get_ending_lba() uint64 {
	return binary.LittleEndian.Uint64(s[40:48])
}

// Put_ending_lba sets ending_lba.
func (s Entry) Put_ending_lba(v uint64) {
	binary.LittleEndian.PutUint64(s[40:48], v)
}

// Get_attributes returns attributes.
func (s Entry) Get_attributes() uint64 {
	return binary.LittleEndian.Uint64(s[48:56])
}

// Put_attributes sets attributes.
func (s Entry) Put_attributes(v uint64) {
	binary.LittleEndian.PutUint64(s[48:56], v)
}

// Get_partition_name returns partition_name.
func (s Entry) Get_partition_name() []byte {
	return s[56:128]
}

// Put_partition_name sets partition_name.
func (s Entry) Put_partition_name(v []byte) {
	copy(s[56:128], v)
}

// ENTRY_SIZE is the size of the Entry struct.
const ENTRY_SIZE = 128