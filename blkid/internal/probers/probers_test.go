// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package probers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probers"
)

func TestMaxMagicSize(t *testing.T) {
	assert.Equal(t, 32774, probers.Chain().MaxMagicSize())
}
