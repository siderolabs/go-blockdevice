// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package utils_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/utils"
)

func TestCRC32c(t *testing.T) {
	buf := []byte("hello, world")
	assert.Equal(t, uint32(0x96665be0), utils.CRC32c(buf))
}

func TestIsPowerOf2(t *testing.T) {
	assert.True(t, utils.IsPowerOf2(uint32(2)))
	assert.True(t, utils.IsPowerOf2(uint32(1<<16)))
	assert.False(t, utils.IsPowerOf2(uint32(0)))
	assert.False(t, utils.IsPowerOf2(uint32(3)))
}
