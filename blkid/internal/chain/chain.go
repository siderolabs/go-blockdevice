// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package chain provides a list of probers for different filesystems and volume managers.
package chain

import (
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/filesystems/bluestore"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/filesystems/btrfs"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/filesystems/ext"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/filesystems/iso9660"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/filesystems/luks"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/filesystems/lvm2"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/filesystems/squashfs"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/filesystems/swap"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/filesystems/talosmeta"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/filesystems/vfat"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/filesystems/xfs"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/filesystems/zfs"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/partitions/gpt"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
)

// Chain is a list of probers.
type Chain []probe.Prober

// MaxMagicSize returns the maximum size of the magic value in the chain.
func (chain Chain) MaxMagicSize() int {
	maxSize := 0

	for _, prober := range chain {
		for _, magic := range prober.Magic() {
			maxSize = max(maxSize, magic.BlockSize())
		}
	}

	return maxSize
}

// MagicMatches returns the prober that matches the magic value in the buffer.
func (chain Chain) MagicMatches(buf []byte) []probe.MagicMatch {
	var matches []probe.MagicMatch

	for _, prober := range chain {
		for _, magic := range prober.Magic() {
			if magic.BlockSize() > len(buf) {
				// skip, this magic cannot match
				continue
			}

			if magic.Matches(buf) {
				matches = append(matches, probe.MagicMatch{Magic: *magic, Prober: prober})

				continue
			}
		}
	}

	return matches
}

// Default returns a list of probers for the filesystems and volume managers.
func Default() Chain {
	return Chain{
		&xfs.Probe{},
		&ext.Probe4{},
		&ext.Probe3{},
		&ext.Probe2{},
		&btrfs.Probe{},
		&vfat.Probe{},
		&iso9660.Probe{},
		&squashfs.Probe{},
		&talosmeta.Probe{},
		&gpt.Probe{},
		&bluestore.Probe{},
		&luks.Probe{},
		&swap.Probe{},
		&lvm2.Probe{},
		&zfs.Probe{},
	}
}
