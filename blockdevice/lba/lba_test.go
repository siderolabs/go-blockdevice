// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package lba_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/siderolabs/go-blockdevice/blockdevice/lba"
)

func TestAlignToRecommended(t *testing.T) {
	l := lba.LBA{ //nolint: exhaustivestruct
		PhysicalBlockSize: 512,
		LogicalBlockSize:  512,
	}

	assert.EqualValues(t, 0, l.AlignToPhysicalBlockSize(0, true))
	assert.EqualValues(t, 2048, l.AlignToPhysicalBlockSize(1, true))
	assert.EqualValues(t, 2048, l.AlignToPhysicalBlockSize(2, true))
	assert.EqualValues(t, 2048, l.AlignToPhysicalBlockSize(3, true))
	assert.EqualValues(t, 2048, l.AlignToPhysicalBlockSize(4, true))
	assert.EqualValues(t, 2048, l.AlignToPhysicalBlockSize(2048, true))
	assert.EqualValues(t, 4096, l.AlignToPhysicalBlockSize(2049, true))
	assert.EqualValues(t, 2048, l.AlignToPhysicalBlockSize(2049, false))
}

func TestAlignToPhysicalBlockSize(t *testing.T) {
	l := lba.LBA{ //nolint: exhaustivestruct
		PhysicalBlockSize: 2 * 1048576,
		LogicalBlockSize:  512,
	}

	assert.EqualValues(t, 0, l.AlignToPhysicalBlockSize(0, true))
	assert.EqualValues(t, 4096, l.AlignToPhysicalBlockSize(1, true))
	assert.EqualValues(t, 4096, l.AlignToPhysicalBlockSize(2, true))
	assert.EqualValues(t, 4096, l.AlignToPhysicalBlockSize(3, true))
	assert.EqualValues(t, 4096, l.AlignToPhysicalBlockSize(4, true))
	assert.EqualValues(t, 4096, l.AlignToPhysicalBlockSize(4096, true))
	assert.EqualValues(t, 8192, l.AlignToPhysicalBlockSize(4097, true))
	assert.EqualValues(t, 4096, l.AlignToPhysicalBlockSize(4097, false))
}

func TestAlignToMinIOkSize(t *testing.T) {
	l := lba.LBA{ //nolint: exhaustivestruct
		MinimalIOSize:     4 * 1048576,
		PhysicalBlockSize: 512,
		LogicalBlockSize:  512,
	}

	assert.EqualValues(t, 0, l.AlignToPhysicalBlockSize(0, true))
	assert.EqualValues(t, 8192, l.AlignToPhysicalBlockSize(1, true))
	assert.EqualValues(t, 8192, l.AlignToPhysicalBlockSize(2, true))
	assert.EqualValues(t, 8192, l.AlignToPhysicalBlockSize(3, true))
	assert.EqualValues(t, 8192, l.AlignToPhysicalBlockSize(4, true))
	assert.EqualValues(t, 8192, l.AlignToPhysicalBlockSize(8, true))
	assert.EqualValues(t, 8192, l.AlignToPhysicalBlockSize(8192, true))
	assert.EqualValues(t, 16384, l.AlignToPhysicalBlockSize(8193, true))
	assert.EqualValues(t, 8192, l.AlignToPhysicalBlockSize(8193, false))
}
