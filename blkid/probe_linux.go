// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build linux

package blkid

import (
	"fmt"
	"io"
	"os"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probers"
)

func (i *Info) probe(f *os.File, offset, length uint64) error {
	if offset+length > i.Size {
		return fmt.Errorf("probing range is out of bounds: offset %d + len %d > size %d", offset, length, i.Size)
	}

	// read enough data to cover the maximum magic size
	chain := probers.Chain()

	if length < uint64(chain.MaxMagicSize()) {
		return fmt.Errorf("probing range is too small: len %d < max magic size %d", length, chain.MaxMagicSize())
	}

	magicReadSize := max(uint64(chain.MaxMagicSize()), i.IOSize)
	magicReadSize = min(magicReadSize, length)

	buf := make([]byte, magicReadSize)

	_, err := f.ReadAt(buf, int64(offset))
	if err != nil {
		return fmt.Errorf("error reading magic buffer: %w", err)
	}

	for _, matched := range chain.MagicMatches(buf) {
		res, err := matched.Probe(io.NewSectionReader(f, int64(offset), int64(length)))
		if err != nil || res == nil {
			// skip failed probes
			continue
		}

		i.Name = matched.Name()
		i.UUID = res.UUID
		i.BlockSize = res.BlockSize
		i.FilesystemBlockSize = res.FilesystemBlockSize
		i.FilesystemSize = res.FilesystemSize
		i.Label = res.Label

		break
	}

	return nil
}
