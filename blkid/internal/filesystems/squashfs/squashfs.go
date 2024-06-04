// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package squashfs probes Squash filesystems.
package squashfs

//go:generate go run ../../cstruct/cstruct.go -pkg squashfs -struct SuperBlock -input superblock.h -endianness LittleEndian

import (
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/utils"
)

var squashfsMagic1 = magic.Magic{ // big endian
	Offset: 0,
	Value:  []byte("sqsh"),
}

var squashfsMagic2 = magic.Magic{ // little endian
	Offset: 0,
	Value:  []byte("hsqs"),
}

// Probe for the filesystem.
type Probe struct{}

// Magic returns the magic value for the filesystem.
func (p *Probe) Magic() []*magic.Magic {
	return []*magic.Magic{
		&squashfsMagic1,
		&squashfsMagic2,
	}
}

// Name returns the name of the filesystem.
func (p *Probe) Name() string {
	return "squashfs"
}

// Probe runs the further inspection and returns the result if successful.
func (p *Probe) Probe(r probe.Reader, _ magic.Magic) (*probe.Result, error) {
	buf := make([]byte, SUPERBLOCK_SIZE)

	if err := utils.ReadFullAt(r, buf, 0); err != nil {
		return nil, err
	}

	sb := SuperBlock(buf)

	vermaj := sb.Get_version_major()
	if vermaj < 4 {
		return nil, nil //nolint:nilnil
	}

	res := &probe.Result{
		BlockSize:           sb.Get_block_size(),
		FilesystemBlockSize: sb.Get_block_size(),
		ProbedSize:          sb.Get_bytes_used(),
	}

	return res, nil
}
