// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gpt_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/suite"

	bdtable "github.com/talos-systems/go-blockdevice/blockdevice/table"
	"github.com/talos-systems/go-blockdevice/blockdevice/table/gpt"
	"github.com/talos-systems/go-blockdevice/blockdevice/table/gpt/partition"
)

const (
	size      = 1024 * 1024 * 1024 * 1024
	blockSize = 512

	headReserved = 33
	tailReserved = 34
)

type GPTSuite struct {
	suite.Suite

	f *os.File
}

func (suite *GPTSuite) SetupTest() {
	var err error

	suite.f, err = ioutil.TempFile("", "blockdevice")
	suite.Require().NoError(err)

	suite.Require().NoError(suite.f.Truncate(size))
}

func (suite *GPTSuite) TearDownTest() {
	suite.Assert().NoError(suite.f.Close())
	suite.Assert().NoError(os.Remove(suite.f.Name()))
}

func (suite *GPTSuite) TestPartitionAdd() {
	table, err := gpt.NewGPT("/dev/null", suite.f)
	suite.Require().NoError(err)

	_, err = table.New()
	suite.Require().NoError(err)

	const (
		bootSize = 1048576
		efiSize  = 512 * 1048576
	)

	_, err = table.Add(bootSize, partition.WithPartitionName("boot"))
	suite.Require().NoError(err)

	_, err = table.Add(efiSize, partition.WithPartitionName("efi"))
	suite.Require().NoError(err)

	_, err = table.Add(0, partition.WithPartitionName("system"), partition.WithMaximumSize(true))
	suite.Require().NoError(err)

	assertPartitions := func(partitions []bdtable.Partition) {
		suite.Require().Len(partitions, 3)

		partBoot := partitions[0]
		suite.Assert().EqualValues(1, partBoot.No())
		suite.Assert().EqualValues(bootSize/blockSize, partBoot.Length())
		suite.Assert().EqualValues(headReserved+1, partBoot.Start()) // first usable LBA

		partEFI := partitions[1]
		suite.Assert().EqualValues(2, partEFI.No())
		suite.Assert().EqualValues(efiSize/blockSize, partEFI.Length())
		suite.Assert().EqualValues(headReserved+1+bootSize/blockSize, partEFI.Start())

		partSystem := partitions[2]
		suite.Assert().EqualValues(3, partSystem.No())
		suite.Assert().EqualValues((size-bootSize-efiSize)/blockSize-headReserved-tailReserved, partSystem.Length())
		suite.Assert().EqualValues(headReserved+1+bootSize/blockSize+efiSize/blockSize, partSystem.Start())
	}

	assertPartitions(table.Partitions())

	suite.Require().NoError(table.Write())

	// re-read the partition table
	table, err = gpt.NewGPT("/dev/null", suite.f)
	suite.Require().NoError(err)

	suite.Require().NoError(table.Read())

	assertPartitions(table.Partitions())
}

func TestGPTSuite(t *testing.T) {
	suite.Run(t, new(GPTSuite))
}
