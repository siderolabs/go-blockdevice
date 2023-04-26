// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package probe_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/siderolabs/go-blockdevice/blockdevice"
	"github.com/siderolabs/go-blockdevice/blockdevice/partition/gpt"
	"github.com/siderolabs/go-blockdevice/blockdevice/probe"
	"github.com/siderolabs/go-blockdevice/blockdevice/test"
)

type ProbeSuite struct {
	test.BlockDeviceSuite
}

func (suite *ProbeSuite) SetupTest() {
	suite.CreateBlockDevice(1024 * 1024 * 1024)
}

func (suite *ProbeSuite) addPartition(name string, size uint64, fatBits int) *gpt.Partition {
	var (
		g   *gpt.GPT
		err error
	)

	g, err = gpt.Open(suite.Dev)
	if errors.Is(err, gpt.ErrPartitionTableDoesNotExist) {
		g, err = gpt.New(suite.Dev)
	} else if err == nil {
		err = g.Read()
	}

	suite.Require().NoError(err)

	partition, err := g.Add(size, gpt.WithPartitionName(name))
	suite.Require().NoError(err)

	suite.Require().NoError(g.Write())

	partPath, err := partition.Path()
	suite.Require().NoError(err)

	cmd := exec.Command("mkfs.vfat", "-F", strconv.Itoa(fatBits), "-n", name, partPath)
	suite.Require().NoError(cmd.Run())

	return partition
}

func (suite *ProbeSuite) setSystemLabelVFAT(name string, fatBits int) {
	cmd := exec.Command("mkfs.vfat", "-F", strconv.Itoa(fatBits), "-n", name, suite.LoopbackDevice.Name())
	suite.Require().NoError(cmd.Run())
}

func (suite *ProbeSuite) setSystemLabelEXT4(name string) {
	cmd := exec.Command("mkfs.ext4", "-L", name, suite.LoopbackDevice.Name())
	suite.Require().NoError(cmd.Run())
}

func (suite *ProbeSuite) TestBlockDeviceWithSymlinkResolves() {
	// Create a symlink to the block device
	symlink := suite.Dev.Name() + ".link"
	suite.Require().NoError(os.Symlink(suite.Dev.Name(), symlink))

	defer os.Remove(symlink) //nolint:errcheck

	bd, err := blockdevice.Open(symlink)
	suite.Require().NoError(err)
	suite.Require().Equal(suite.Dev.Name(), bd.Device().Name())
}

func (suite *ProbeSuite) TestDevForPartitionLabel() {
	part12 := suite.addPartition("devpart12", 1024*1024, 12)
	part32 := suite.addPartition("devpart32", 1024*1024*256, 32)

	for _, tc := range []struct {
		part  *gpt.Partition
		label string
	}{
		{
			label: "devpart12",
			part:  part12,
		},
		{
			label: "devpart32",
			part:  part32,
		},
	} {
		suite.T().Run(tc.label, func(t *testing.T) {
			dev, err := probe.DevForPartitionLabel(suite.LoopbackDevice.Name(), tc.label)
			suite.Require().NoError(err)

			path, err := tc.part.Path()
			suite.Require().NoError(err)
			suite.Require().Equal(path, dev.Device().Name())
		})
	}
}

func (suite *ProbeSuite) TestGetDevWithPartitionName() {
	size := uint64(1024 * 1024 * 512)
	part := suite.addPartition("devlabel", size, 32)

	dev, err := probe.GetDevWithPartitionName("devlabel")
	suite.Require().NoError(err)
	devpath, err := part.Path()
	suite.Require().NoError(err)
	suite.Require().Equal(devpath, dev.Path)
}

func (suite *ProbeSuite) testGetDevWithFileSystemLabel(fatBits int) {
	label := fmt.Sprintf("LABELSYS%d", fatBits)

	suite.setSystemLabelVFAT(label, fatBits)

	dev, err := probe.GetDevWithFileSystemLabel(label)
	suite.Require().NoError(err)
	suite.Require().Equal(suite.LoopbackDevice.Name(), dev.Path)
}

func (suite *ProbeSuite) TestGetDevWithFileSystemLabel16() {
	suite.testGetDevWithFileSystemLabel(16)
}

func (suite *ProbeSuite) TestGetDevWithFileSystemLabel32() {
	suite.testGetDevWithFileSystemLabel(32)
}

func (suite *ProbeSuite) TestProbeByPartitionLabel() {
	suite.addPartition("test", 1024*1024, 12)
	suite.addPartition("test2", 1024*1024*256, 32)

	probed, err := probe.All(probe.WithPartitionLabel("test"), probe.WithSingleResult())
	suite.Require().NoError(err)
	suite.Require().Equal(1, len(probed))

	suite.Require().Equal(suite.LoopbackDevice.Name(), probed[0].Device().Name())
}

func (suite *ProbeSuite) TestProbeByFilesystemLabelBlockdeviceVFAT() {
	suite.setSystemLabelVFAT("FSLBABELBD", 32)

	probed, err := probe.All(probe.WithFileSystemLabel("FSLBABELBD"))
	suite.Require().NoError(err)
	suite.Require().Equal(1, len(probed))

	suite.Require().Equal(suite.LoopbackDevice.Name(), probed[0].Device().Name())
	suite.Require().Equal(suite.LoopbackDevice.Name(), probed[0].Path)
}

func (suite *ProbeSuite) TestProbeByFilesystemLabelBlockdeviceEXT4() {
	suite.setSystemLabelEXT4("EXTBD")

	probed, err := probe.All(probe.WithFileSystemLabel("EXTBD"))
	suite.Require().NoError(err)
	suite.Require().Equal(1, len(probed))

	suite.Require().Equal(suite.LoopbackDevice.Name(), probed[0].Device().Name())
	suite.Require().Equal(suite.LoopbackDevice.Name(), probed[0].Path)
}

func (suite *ProbeSuite) TestProbeByFilesystemLabelPartition() {
	suite.addPartition("FOO", 1024*1024*256, 16)
	suite.addPartition("FSLABELPART", 1024*1024*2, 12)

	probed, err := probe.All(probe.WithFileSystemLabel("FSLABELPART"))
	suite.Require().NoError(err)
	suite.Require().Equal(1, len(probed))

	suite.Require().Equal(suite.LoopbackDevice.Name(), probed[0].Device().Name())
	suite.Require().Equal(suite.LoopbackDevice.Name()+"p2", probed[0].Path)
}

func TestProbe(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("can't run the test as non-root")
	}

	if hostname, _ := os.Hostname(); hostname == "buildkitsandbox" { //nolint: errcheck
		t.Skip("test not supported under buildkit as partition devices are not propagated from /dev")
	}

	suite.Run(t, new(ProbeSuite))
}
