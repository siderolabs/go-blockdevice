// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package luks_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/siderolabs/go-blockdevice/v2/encryption/luks"
)

func TestTokenMarshal(t *testing.T) {
	type SealedKey struct {
		SealedKey string `json:"sealed_key"`
	}

	token := &luks.Token[SealedKey]{
		UserData: SealedKey{
			SealedKey: "aaaa",
		},
		Type: "sealedkey",
	}

	b, err := token.Bytes()
	require.NoError(t, err)

	assert.Equal(t, `{"UserData":{"sealed_key":"aaaa"},"type":"sealedkey","keyslots":[]}`, string(b))

	var token2 luks.Token[SealedKey]

	require.NoError(t, token2.Decode(b))

	assert.Equal(t, token, &token2)
}
