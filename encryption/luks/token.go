// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package luks

import "encoding/json"

// Token defines LUKS2 token.
type Token[UserData any] struct {
	// UserData has a strange JSON tag, but this keeps it backwards compatible with v1 library.
	UserData UserData `json:"UserData"`
	Type     string   `json:"type"`
}

// Bytes encodes token into bytes.
func (t *Token[UserData]) Bytes() ([]byte, error) {
	return json.Marshal(struct {
		*Token[UserData]
		KeySlots []string `json:"keyslots"`
	}{Token: t, KeySlots: []string{}})
}

// Decode reads token data from bytes.
func (t *Token[UserData]) Decode(in []byte) error {
	return json.Unmarshal(in, &t)
}
