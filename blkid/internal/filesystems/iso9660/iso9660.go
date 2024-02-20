// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package iso9660 probes ISO9660 filesystems.
package iso9660

//go:generate go run ../../cstruct/cstruct.go -pkg iso9660 -struct VolumeDescriptor -input volume.h -endianness NativeEndian

import (
	"io"
	"strings"

	"github.com/siderolabs/go-pointer"
	"golang.org/x/text/encoding/unicode"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/result"
)

const (
	superblockOffset = 0x8000
)

var isoMagic = magic.Magic{
	Offset: superblockOffset + 1,
	Value:  []byte("CD001"),
}

// Probe for the filesystem.
type Probe struct{}

// Magic returns the magic value for the filesystem.
func (p *Probe) Magic() []*magic.Magic {
	return []*magic.Magic{&isoMagic}
}

// Name returns the name of the filesystem.
func (p *Probe) Name() string {
	return "iso9660"
}

const (
	vdMax           = 16
	vdEnd           = 0xff
	vdBootRecord    = 0
	vdPrimary       = 1
	vdSupplementary = 2

	sectorSize = 2048
)

func isonum16(b []byte) uint16 {
	return uint16(b[0]) | uint16(b[1])<<8
}

// Probe runs the further inspection and returns the result if successful.
func (p *Probe) Probe(r io.ReaderAt) (*result.Result, error) {
	var pvd, joilet VolumeDescriptor

vdLoop:
	for i := range vdMax {
		buf := make([]byte, VOLUMEDESCRIPTOR_SIZE)

		if _, err := r.ReadAt(buf, superblockOffset+sectorSize*int64(i)); err != nil {
			break
		}

		vd := VolumeDescriptor(buf)

		switch vd.Get_vd_type() {
		case vdEnd:
			break vdLoop
		case vdBootRecord:
			// skip
		case vdPrimary:
			pvd = vd
		case vdSupplementary:
			joilet = vd
		}

		if pvd != nil && joilet != nil {
			break
		}
	}

	if pvd == nil {
		return nil, nil //nolint:nilnil
	}

	logicalBlockSize := isonum16(pvd.Get_logical_block_size())
	spaceSize := isonum16(pvd.Get_space_size())

	res := &result.Result{
		BlockSize:           uint32(logicalBlockSize),
		FilesystemBlockSize: uint32(logicalBlockSize),
		FilesystemSize:      uint64(spaceSize) * uint64(logicalBlockSize),
	}

	if joilet != nil {
		lblBytes := joilet.Get_volume_id()

		if label, err := unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder().Bytes(lblBytes); err == nil {
			res.Label = pointer.To(strings.TrimRight(string(label), " "))
		}
	}

	if res.Label == nil {
		lblBytes := pvd.Get_volume_id()
		res.Label = pointer.To(strings.TrimRight(string(lblBytes), " "))
	}

	return res, nil
}
