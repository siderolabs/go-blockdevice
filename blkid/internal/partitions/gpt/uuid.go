// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gpt

func guidToUUID(g []byte) []byte {
	return append(
		[]byte{
			g[3], g[2], g[1], g[0],
			g[5], g[4],
			g[7], g[6],
			g[8], g[9],
		},
		g[10:16]...,
	)
}
