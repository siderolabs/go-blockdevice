// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package partitioning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/siderolabs/go-blockdevice/v2/partitioning"
)

func TestDevName(t *testing.T) {
	t.Parallel()

	for _, test := range []struct { //nolint:govet
		devname   string
		partition uint

		expected string
	}{
		{
			devname:   "/dev/sda",
			partition: 1,

			expected: "/dev/sda1",
		},
		{
			devname:   "/dev/nvme0n1",
			partition: 2,

			expected: "/dev/nvme0n1p2",
		},
	} {
		t.Run(test.devname, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.expected, partitioning.DevName(test.devname, test.partition))
		})
	}
}
