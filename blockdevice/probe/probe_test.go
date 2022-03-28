// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package probe_test

import (
	"errors"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/talos-systems/go-blockdevice/blockdevice/partition/gpt"
	"github.com/talos-systems/go-blockdevice/blockdevice/probe"
	"github.com/talos-systems/go-blockdevice/blockdevice/test"
)

type ProbeSuite struct {
	test.BlockDeviceSuite
}

func (suite *ProbeSuite) SetupTest() {
	suite.CreateBlockDevice(1024 * 1024 * 1024)
}

func (suite *ProbeSuite) addPartition(name string, size uint64) *gpt.Partition {
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

	cmd := exec.Command("mkfs.vfat", "-F", "32", "-n", name, partPath)
	suite.Require().NoError(cmd.Run())

	return partition
}

func (suite *ProbeSuite) setSystemLabel(name string) {
	cmd := exec.Command("mkfs.vfat", "-F", "32", "-n", name, suite.LoopbackDevice.Name())
	suite.Require().NoError(cmd.Run())
}

func (suite *ProbeSuite) TestDevForPartitionLabel() {
	size := uint64(1024 * 1024 * 256)
	part := suite.addPartition("devpart1", size)

	dev, err := probe.DevForPartitionLabel(suite.LoopbackDevice.Name(), "devpart1")
	suite.Require().NoError(err)
	path, err := part.Path()
	suite.Require().NoError(err)
	suite.Require().Equal(path, dev.Device().Name())
}

func (suite *ProbeSuite) TestGetDevWithPartitionName() {
	size := uint64(1024 * 1024 * 512)
	part := suite.addPartition("devlabel", size)

	dev, err := probe.GetDevWithPartitionName("devlabel")
	suite.Require().NoError(err)
	devpath, err := part.Path()
	suite.Require().NoError(err)
	suite.Require().Equal(devpath, dev.Path)
}

func (suite *ProbeSuite) TestGetDevWithFileSystemLabel() {
	suite.setSystemLabel("GETLABELSYS")

	dev, err := probe.GetDevWithFileSystemLabel("GETLABELSYS")
	suite.Require().NoError(err)
	suite.Require().Equal(suite.LoopbackDevice.Name(), dev.Path)
}

func (suite *ProbeSuite) TestProbeByPartitionLabel() {
	size := uint64(1024 * 1024 * 256)
	suite.addPartition("test", size)
	suite.addPartition("test2", size)

	probed, err := probe.All(probe.WithPartitionLabel("test"))
	suite.Require().NoError(err)
	suite.Require().Equal(1, len(probed))

	suite.Require().Equal(suite.LoopbackDevice.Name(), probed[0].Device().Name())
}

func (suite *ProbeSuite) TestProbeByFilesystemLabelBlockdevice() {
	suite.setSystemLabel("FSLBABELBD")

	probed, err := probe.All(probe.WithFileSystemLabel("FSLBABELBD"))
	suite.Require().NoError(err)
	suite.Require().Equal(1, len(probed))

	suite.Require().Equal(suite.LoopbackDevice.Name(), probed[0].Device().Name())
	suite.Require().Equal(suite.LoopbackDevice.Name(), probed[0].Path)
}

func (suite *ProbeSuite) TestProbeByFilesystemLabelPartition() {
	size := uint64(1024 * 1024 * 256)
	suite.addPartition("FOO", size)
	suite.addPartition("FSLABELPART", size)

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
