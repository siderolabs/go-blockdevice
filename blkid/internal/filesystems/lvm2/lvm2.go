// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package lvm2 probes LVM2 PVs.
package lvm2

//go:generate go run ../../../../internal/cstruct/cstruct.go -pkg lvm2 -struct LVM2Header -input lvm2_header.h -endianness LittleEndian

import (
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
	"github.com/siderolabs/go-blockdevice/v2/internal/ioutil"
)

var (
	lvmMagic1 = magic.Magic{
		Offset: 0x018,
		Value:  []byte("LVM2 001"),
	}

	lvmMagic2 = magic.Magic{
		Offset: 0x218,
		Value:  []byte("LVM2 001"),
	}
)

// Probe for the filesystem.
type Probe struct{}

// Magic returns the magic value for the filesystem.
func (p *Probe) Magic() []*magic.Magic {
	return []*magic.Magic{
		&lvmMagic1,
		&lvmMagic2,
	}
}

// Name returns the name of the filesystem.
func (p *Probe) Name() string {
	return "lvm2-pv"
}

func (p *Probe) probe(r probe.Reader, offset int64) (LVM2Header, error) {
	buf := make([]byte, LVM2HEADER_SIZE)

	if err := ioutil.ReadFullAt(r, buf, offset); err != nil {
		return nil, err
	}

	hdr := LVM2Header(buf)

	if string(hdr.Get_id()) != "LABELONE" || string(hdr.Get_type()) != "LVM2 001" {
		return nil, nil
	}

	return hdr, nil
}

// Probe runs the further inspection and returns the result if successful.
func (p *Probe) Probe(r probe.Reader, _ magic.Magic) (*probe.Result, error) {
	hdr, err := p.probe(r, 0)
	if hdr == nil {
		if err != nil {
			return nil, err
		}

		hdr, err = p.probe(r, 512)
		if err != nil {
			return nil, err
		}

		if hdr == nil {
			return nil, nil //nolint:nilnil
		}
	}

	res := &probe.Result{}

	// LVM2 UUIDs aren't 16 bytes thus are treated as labels
	labelUUID := string(hdr.Get_pv_uuid())
	labelUUID = labelUUID[:6] + "-" + labelUUID[6:10] + "-" + labelUUID[10:14] +
		"-" + labelUUID[14:18] + "-" + labelUUID[18:22] +
		"-" + labelUUID[22:26] + "-" + labelUUID[26:]
	res.Label = &labelUUID

	return res, nil
}
