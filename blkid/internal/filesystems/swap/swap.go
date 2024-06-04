// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package swap probes Linux swapspaces.
package swap

// TODO: is it little or host endian?
//go:generate go run ../../cstruct/cstruct.go -pkg swap -struct SwapHeader -input swap_header.h -endianness LittleEndian

import (
	"bytes"

	"github.com/google/uuid"
	"github.com/siderolabs/go-pointer"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/utils"
)

var (
	swapMagic1 = magic.Magic{
		Offset: 0xff6,
		Value:  []byte("SWAP-SPACE"),
	}

	swapMagic2 = magic.Magic{
		Offset: 0xff6,
		Value:  []byte("SWAPSPACE2"),
	}

	swapMagic3 = magic.Magic{
		Offset: 0x1ff6,
		Value:  []byte("SWAP-SPACE"),
	}

	swapMagic4 = magic.Magic{
		Offset: 0x1ff6,
		Value:  []byte("SWAPSPACE2"),
	}

	swapMagic5 = magic.Magic{
		Offset: 0x3ff6,
		Value:  []byte("SWAP-SPACE"),
	}

	swapMagic6 = magic.Magic{
		Offset: 0x3ff6,
		Value:  []byte("SWAPSPACE2"),
	}

	swapMagic7 = magic.Magic{
		Offset: 0x7ff6,
		Value:  []byte("SWAP-SPACE"),
	}

	swapMagic8 = magic.Magic{
		Offset: 0x7ff6,
		Value:  []byte("SWAPSPACE2"),
	}

	swapMagic9 = magic.Magic{
		Offset: 0xfff6,
		Value:  []byte("SWAP-SPACE"),
	}

	swapMagic10 = magic.Magic{
		Offset: 0xfff6,
		Value:  []byte("SWAPSPACE2"),
	}
)

// Probe for the filesystem.
type Probe struct{}

// Magic returns the magic value for the filesystem.
func (p *Probe) Magic() []*magic.Magic {
	return []*magic.Magic{
		&swapMagic1,
		&swapMagic2,
		&swapMagic3,
		&swapMagic4,
		&swapMagic5,
		&swapMagic6,
		&swapMagic7,
		&swapMagic8,
		&swapMagic9,
		&swapMagic10,
	}
}

// Name returns the name of the filesystem.
func (p *Probe) Name() string {
	return "swap"
}

// Probe runs the further inspection and returns the result if successful.
func (p *Probe) Probe(r probe.Reader, m magic.Magic) (*probe.Result, error) {
	buf := make([]byte, SWAPHEADER_SIZE)

	if err := utils.ReadFullAt(r, buf, 1024); err != nil {
		return nil, err
	}

	hdr := SwapHeader(buf)

	if hdr.Get_version() != 1 || hdr.Get_lastpage() == 0 {
		return nil, nil //nolint:nilnil
	}

	res := &probe.Result{}

	lbl := hdr.Get_volume()
	if lbl[0] != 0 {
		idx := bytes.IndexByte(lbl, 0)
		if idx == -1 {
			idx = len(lbl)
		}

		res.Label = pointer.To(string(lbl[:idx]))
	}

	fsUUID, err := uuid.FromBytes(hdr.Get_uuid())
	if err == nil {
		res.UUID = &fsUUID
	}

	// https://github.com/util-linux/util-linux/blob/c0207d354ee47fb56acfa64b03b5b559bb301280/libblkid/src/superblocks/swap.c#L47
	pageSize := m.Offset + len(m.Value)
	res.BlockSize = uint32(pageSize)
	res.FilesystemBlockSize = uint32(pageSize)
	res.ProbedSize = uint64(pageSize) * uint64(hdr.Get_lastpage())

	return res, nil
}
