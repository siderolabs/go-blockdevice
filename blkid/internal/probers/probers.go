// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package probers provides a list of probers for different filesystems and volume managers.
package probers

import (
	"io"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/filesystems/bluestore"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/filesystems/ext"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/filesystems/iso9660"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/filesystems/luks"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/filesystems/vfat"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/filesystems/xfs"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/result"
)

// Prober is an interface for probing filesystems and volume managers.
type Prober interface {
	// Name returns the name of the filesystem or volume manager.
	Name() string
	// Magic returns the magic value for the filesystem or volume manager.
	Magic() []*magic.Magic
	// Probe runs the further inspection and returns the result if successful.
	Probe(io.ReaderAt) (*result.Result, error)
}

// ProberChain is a list of probers.
type ProberChain []Prober

// MaxMagicSize returns the maximum size of the magic value in the chain.
func (chain ProberChain) MaxMagicSize() int {
	max := 0

	for _, prober := range chain {
		for _, magic := range prober.Magic() {
			if size := magic.BlockSize(); size >= max {
				max = size
			}
		}
	}

	return max
}

// MagicMatches returns the prober that matches the magic value in the buffer.
func (chain ProberChain) MagicMatches(buf []byte) []Prober {
	var matches []Prober

	for _, prober := range chain {
		for _, magic := range prober.Magic() {
			if magic.Matches(buf) {
				matches = append(matches, prober)

				continue
			}
		}
	}

	return matches
}

// Chain returns a list of probers for the filesystems and volume managers.
func Chain() ProberChain {
	return ProberChain{
		&xfs.Probe{},
		&ext.Probe{},
		&vfat.Probe{},
		&luks.Probe{},
		&iso9660.Probe{},
		&bluestore.Probe{},
	}
}
