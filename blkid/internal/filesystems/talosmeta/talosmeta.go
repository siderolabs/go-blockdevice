// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package talosmeta probes Talos META partition.
package talosmeta

import (
	"encoding/binary"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/utils"
)

// META constants, from talos/internal/pkg/meta/internal/adv/talos.
const (
	magic1 uint32 = 0x5a4b3c2d
	magic2 uint32 = 0xa5b4c3d2
	length        = 256 * 1024
)

var metaMagic = magic.Magic{
	Offset: 0,
	Value:  binary.BigEndian.AppendUint32(nil, magic1),
}

// Probe for the filesystem.
type Probe struct{}

// Magic returns the magic value for the filesystem.
func (p *Probe) Magic() []*magic.Magic {
	return []*magic.Magic{
		&metaMagic,
	}
}

// Name returns the name of the filesystem.
func (p *Probe) Name() string {
	return "talosmeta"
}

// Probe runs the further inspection and returns the result if successful.
func (p *Probe) Probe(r probe.Reader, _ magic.Magic) (*probe.Result, error) {
	buf := make([]byte, 4)

	for _, offset := range []int64{0, length} {
		if err := utils.ReadFullAt(r, buf, offset); err != nil {
			return nil, err
		}

		if binary.BigEndian.Uint32(buf) != magic1 {
			continue
		}

		if err := utils.ReadFullAt(r, buf, offset+length-4); err != nil {
			return nil, err
		}

		if binary.BigEndian.Uint32(buf) != magic2 {
			continue
		}

		return &probe.Result{
			ProbedSize: 2 * length,
		}, nil
	}

	return nil, nil //nolint:nilnil
}
