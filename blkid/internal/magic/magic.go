// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package magic implements the magic number detection for files and block devices.
package magic

import "bytes"

// Magic defines a filesystem/volume manager/etc magic value.
type Magic struct {
	// Value to search for.
	Value []byte

	// Offset in the file where the magic value is located.
	Offset int
}

// Matches returns true if the magic value is found at the specified offset in the buffer.
func (magic *Magic) Matches(buf []byte) bool {
	if len(buf) < magic.Offset+len(magic.Value) {
		return false
	}

	return bytes.Equal(buf[magic.Offset:magic.Offset+len(magic.Value)], magic.Value)
}

// BlockSize returns the size of the buffer that needs to be read from the disk to detect the magic value.
func (magic *Magic) BlockSize() int {
	return magic.Offset + len(magic.Value)
}
