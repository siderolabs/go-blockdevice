// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gpt

import (
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
)

func lastLBA(r probe.Reader) (uint64, bool) {
	sectorSize := r.GetSectorSize()
	size := r.GetSize()

	if uint64(sectorSize) > size {
		return 0, false
	}

	return (size / uint64(sectorSize)) - 1, true
}
