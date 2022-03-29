// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package lba

import (
	"os"
)

// RecommendedAlignment is recommended alignment for LBA.
const RecommendedAlignment = 1048576

// LBA represents logical block addressing.
//
//nolint:govet
type LBA struct {
	PhysicalBlockSize int64
	LogicalBlockSize  int64
	MinimalIOSize     int64
	OptimalIOSize     int64

	TotalSectors int64

	f *os.File
}

// AlignToPhysicalBlockSize aligns LBA value in LogicalBlockSize multiples to be aligned to PhysicalBlockSize.
func (l *LBA) AlignToPhysicalBlockSize(lba uint64, roundUp bool) uint64 {
	physToLogical := uint64(l.PhysicalBlockSize / l.LogicalBlockSize)
	minIOToLogical := uint64(l.MinimalIOSize / l.LogicalBlockSize)
	recommended := uint64(RecommendedAlignment / l.LogicalBlockSize)

	// find max ratio
	ratio := physToLogical
	if minIOToLogical > ratio {
		ratio = minIOToLogical
	}

	if recommended > ratio {
		ratio = recommended
	}

	if ratio <= 1 {
		return lba
	}

	if roundUp {
		return (lba + ratio - 1) / ratio * ratio
	}

	return lba / ratio * ratio
}
