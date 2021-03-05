// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package lba_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/talos-systems/go-blockdevice/blockdevice/lba"
)

func TestAlignToPhysicalBlockSize(t *testing.T) {
	l := lba.LBA{ //nolint: exhaustivestruct
		PhysicalBlockSize: 4096,
		LogicalBlockSize:  512,
	}

	assert.EqualValues(t, 0, l.AlignToPhysicalBlockSize(0))
	assert.EqualValues(t, 8, l.AlignToPhysicalBlockSize(1))
	assert.EqualValues(t, 8, l.AlignToPhysicalBlockSize(2))
	assert.EqualValues(t, 8, l.AlignToPhysicalBlockSize(3))
	assert.EqualValues(t, 8, l.AlignToPhysicalBlockSize(4))
	assert.EqualValues(t, 8, l.AlignToPhysicalBlockSize(8))
	assert.EqualValues(t, 16, l.AlignToPhysicalBlockSize(9))
}
