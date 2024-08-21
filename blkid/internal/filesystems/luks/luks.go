// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package luks probes LUKS encrypted filesystems.
package luks

//go:generate go run ../../../../internal/cstruct/cstruct.go -pkg luks -struct Luks2Header -input luks2_header.h -endianness BigEndian

import (
	"bytes"

	"github.com/google/uuid"
	"github.com/siderolabs/go-pointer"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
)

var luksMagic = magic.Magic{
	Offset: 0,
	Value:  []byte("LUKS\xba\xbe"),
}

// Probe for the filesystem.
type Probe struct{}

// Magic returns the magic value for the filesystem.
func (p *Probe) Magic() []*magic.Magic {
	return []*magic.Magic{&luksMagic}
}

// Name returns the name of the filesystem.
func (p *Probe) Name() string {
	return "luks"
}

// Probe runs the further inspection and returns the result if successful.
func (p *Probe) Probe(r probe.Reader, _ magic.Magic) (*probe.Result, error) {
	buf := make([]byte, LUKS2HEADER_SIZE)

	if _, err := r.ReadAt(buf, 0); err != nil {
		return nil, err
	}

	hdr := Luks2Header(buf)

	if hdr.Get_version() != 2 {
		return nil, nil //nolint:nilnil
	}

	res := &probe.Result{}

	lbl := hdr.Get_label()
	if lbl[0] != 0 {
		idx := bytes.IndexByte(lbl, 0)
		if idx == -1 {
			idx = len(lbl)
		}

		res.Label = pointer.To(string(lbl[:idx]))
	}

	uuidStr := hdr.Get_uuid()
	if uuidStr[0] != 0 {
		idx := bytes.IndexByte(uuidStr, 0)
		if idx == -1 {
			idx = len(uuidStr)
		}

		uuid, err := uuid.ParseBytes(uuidStr[:idx])
		if err == nil {
			res.UUID = pointer.To(uuid)
		}
	}

	return res, nil
}
