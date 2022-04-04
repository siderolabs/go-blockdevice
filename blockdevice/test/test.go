// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package test contains common test code for all tests in the package.
package test

import (
	"io/ioutil"
	"os"

	"github.com/stretchr/testify/suite"

	"github.com/talos-systems/go-blockdevice/blockdevice/loopback"
)

// BlockDeviceSuite is a common base for all tests that rely on loopback device creation.
type BlockDeviceSuite struct {
	suite.Suite

	File           *os.File
	Dev            *os.File
	LoopbackDevice *os.File
}

// CreateBlockDevice creates a blockDevice.
func (suite *BlockDeviceSuite) CreateBlockDevice(size int64) {
	var err error

	suite.File, err = ioutil.TempFile("", "blockDevice")
	suite.Require().NoError(err)

	suite.Require().NoError(suite.File.Truncate(size))

	suite.LoopbackDevice, err = loopback.NextLoopDevice()
	suite.Require().NoError(err)

	suite.T().Logf("Using %s", suite.LoopbackDevice.Name())

	suite.Require().NoError(loopback.Loop(suite.LoopbackDevice, suite.File))

	suite.Require().NoError(loopback.LoopSetReadWrite(suite.LoopbackDevice))

	suite.Dev, err = os.OpenFile(suite.LoopbackDevice.Name(), os.O_RDWR, 0)
	suite.Require().NoError(err)
}

// TearDownTest implements suite.Suite.
func (suite *BlockDeviceSuite) TearDownTest() {
	if suite.Dev != nil {
		suite.Assert().NoError(suite.Dev.Close())
		suite.Dev = nil
	}

	if suite.LoopbackDevice != nil {
		suite.T().Logf("Freeing %s", suite.LoopbackDevice.Name())

		suite.Assert().NoError(loopback.Unloop(suite.LoopbackDevice))
		suite.Assert().NoError(suite.LoopbackDevice.Close())

		suite.LoopbackDevice = nil
	}

	if suite.File != nil {
		suite.Assert().NoError(suite.File.Close())
		suite.Assert().NoError(os.Remove(suite.File.Name()))
		suite.File = nil
	}
}
