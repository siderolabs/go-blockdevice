// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package lba

import (
	"fmt"
	"os"
)

// NewLBA initializes and returns an `LBA`.
func NewLBA(f *os.File) (lba *LBA, err error) {
	return nil, fmt.Errorf("not implemented")
}

// ReadAt reads from a file in units of LBA.
func (l *LBA) ReadAt(lba, off, length int64) (b []byte, err error) {
	return nil, fmt.Errorf("not implemented")
}

// WriteAt writes to a file in units of LBA.
func (l *LBA) WriteAt(lba, off int64, b []byte) (err error) {
	return fmt.Errorf("not implemented")
}
