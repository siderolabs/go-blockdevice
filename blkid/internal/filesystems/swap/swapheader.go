// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Code generated by "cstruct -pkg swap -struct SwapHeader -input swap_header.h -endianness LittleEndian"; DO NOT EDIT.

package swap

import "encoding/binary"

var _ = binary.LittleEndian

// SwapHeader is a byte slice representing the swap_header.h C header.
type SwapHeader []byte

// Get_version returns version.
func (s SwapHeader) Get_version() uint32 {
	return binary.LittleEndian.Uint32(s[0:4])
}

// Put_version sets version.
func (s SwapHeader) Put_version(v uint32) {
	binary.LittleEndian.PutUint32(s[0:4], v)
}

// Get_lastpage returns lastpage.
func (s SwapHeader) Get_lastpage() uint32 {
	return binary.LittleEndian.Uint32(s[4:8])
}

// Put_lastpage sets lastpage.
func (s SwapHeader) Put_lastpage(v uint32) {
	binary.LittleEndian.PutUint32(s[4:8], v)
}

// Get_nr_badpages returns nr_badpages.
func (s SwapHeader) Get_nr_badpages() uint32 {
	return binary.LittleEndian.Uint32(s[8:12])
}

// Put_nr_badpages sets nr_badpages.
func (s SwapHeader) Put_nr_badpages(v uint32) {
	binary.LittleEndian.PutUint32(s[8:12], v)
}

// Get_uuid returns uuid.
func (s SwapHeader) Get_uuid() []byte {
	return s[12:28]
}

// Put_uuid sets uuid.
func (s SwapHeader) Put_uuid(v []byte) {
	copy(s[12:28], v)
}

// Get_volume returns volume.
func (s SwapHeader) Get_volume() []byte {
	return s[28:44]
}

// Put_volume sets volume.
func (s SwapHeader) Put_volume(v []byte) {
	copy(s[28:44], v)
}

// SWAPHEADER_SIZE is the size of the SwapHeader struct.
const SWAPHEADER_SIZE = 516
