// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gpt

import (
	"os"

	"github.com/siderolabs/go-blockdevice/v2/block"
)

type deviceWrapper struct {
	*os.File
	*block.Device

	size uint64
}

func (wrapper *deviceWrapper) GetSize() uint64 {
	return wrapper.size
}

// DeviceFromBlockDevice creates a new Device from a block.Device.
func DeviceFromBlockDevice(dev *block.Device) (Device, error) {
	size, err := dev.GetSize()
	if err != nil {
		return nil, err
	}

	return &deviceWrapper{
		File:   dev.File(),
		Device: dev,
		size:   size,
	}, nil
}
