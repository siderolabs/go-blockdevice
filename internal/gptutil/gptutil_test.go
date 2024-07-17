// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gptutil_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/siderolabs/go-blockdevice/v2/internal/gptutil"
)

func TestGUIDToUUID(t *testing.T) {
	uuid := []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}

	guid := []byte{0x67, 0x45, 0x23, 0x01, 0xab, 0x89, 0xef, 0xcd, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}

	assert.Equal(t, uuid, gptutil.GUIDToUUID(guid))
	assert.Equal(t, guid, gptutil.GUIDToUUID(uuid))
	assert.Equal(t, uuid, gptutil.GUIDToUUID(gptutil.UUIDToGUID(uuid)))
}
