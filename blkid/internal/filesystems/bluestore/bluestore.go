// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package bluestore probes Ceph bluestore devices.
package bluestore

import (
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
)

var blueStoreMagic = magic.Magic{
	Offset: 0,
	Value:  []byte("bluestore block device"),
}

// Probe for the bluestore.
type Probe struct{}

// Magic returns the magic value for the filesystem.
func (p *Probe) Magic() []*magic.Magic {
	return []*magic.Magic{&blueStoreMagic}
}

// Name returns the name of the filesystem.
func (p *Probe) Name() string {
	return "bluestore"
}

// Probe runs the further inspection and returns the result if successful.
func (p *Probe) Probe(probe.Reader) (*probe.Result, error) {
	return &probe.Result{}, nil
}
