// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package ioutil provides IO utility functions.
package ioutil

import (
	"io"
)

// ReadFullAt is io.ReadFull for io.ReaderAt.
func ReadFullAt(r io.ReaderAt, buf []byte, offset int64) error {
	for n := 0; n < len(buf); {
		m, err := r.ReadAt(buf[n:], offset)

		n += m
		offset += int64(m)

		if err != nil {
			if err == io.EOF && n == len(buf) {
				return nil
			}

			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}

			return err
		}
	}

	return nil
}
