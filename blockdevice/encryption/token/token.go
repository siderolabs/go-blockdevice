// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package token contains token management interfaces.
package token

// Token defines luks token interface.
type Token interface {
	Bytes() ([]byte, error)
	Decode(in []byte) error
}
