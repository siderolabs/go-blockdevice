// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gpt_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/talos-systems/go-blockdevice/blockdevice"
	"github.com/talos-systems/go-blockdevice/blockdevice/loopback"
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

	f              *os.File
	dev            *os.File
	loopbackDevice *os.File
}

func (suite *GPTSuite) SetupTest() {
	var err error

	suite.f, err = ioutil.TempFile("", "blockdevice")
	suite.Require().NoError(err)

	suite.Require().NoError(suite.f.Truncate(size))

	suite.loopbackDevice, err = loopback.NextLoopDevice()
	suite.Require().NoError(err)

	suite.T().Logf("Using %s", suite.loopbackDevice.Name())

	suite.Require().NoError(loopback.Loop(suite.loopbackDevice, suite.f))

	suite.Require().NoError(loopback.LoopSetReadWrite(suite.loopbackDevice))

	suite.dev, err = os.OpenFile(suite.loopbackDevice.Name(), os.O_RDWR, 0)
	suite.Require().NoError(err)
}

func (suite *GPTSuite) TearDownTest() {
	suite.Assert().NoError(suite.dev.Close())

	if suite.loopbackDevice != nil {
		suite.Assert().NoError(loopback.Unloop(suite.loopbackDevice))
	}

	suite.Assert().NoError(suite.f.Close())
	suite.Assert().NoError(os.Remove(suite.f.Name()))
}

func (suite *GPTSuite) TestPartitionAdd() {
	table, err := gpt.NewGPT(suite.loopbackDevice.Name(), suite.dev)
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
		suite.Assert().EqualValues("boot", partBoot.Label())
		suite.Assert().EqualValues(bootSize/blockSize, partBoot.Length())
		suite.Assert().EqualValues(headReserved+1, partBoot.Start()) // first usable LBA

		partEFI := partitions[1]
		suite.Assert().EqualValues(2, partEFI.No())
		suite.Assert().EqualValues("efi", partEFI.Label())
		suite.Assert().EqualValues(efiSize/blockSize, partEFI.Length())
		suite.Assert().EqualValues(headReserved+1+bootSize/blockSize, partEFI.Start())

		partSystem := partitions[2]
		suite.Assert().EqualValues(3, partSystem.No())
		suite.Assert().EqualValues("system", partSystem.Label())
		suite.Assert().EqualValues((size-bootSize-efiSize)/blockSize-headReserved-tailReserved, partSystem.Length())
		suite.Assert().EqualValues(headReserved+1+bootSize/blockSize+efiSize/blockSize, partSystem.Start())
	}

	assertPartitions(table.Partitions())

	suite.Require().NoError(table.Write())

	// re-read the partition table
	table, err = gpt.NewGPT(suite.loopbackDevice.Name(), suite.dev)
	suite.Require().NoError(err)

	suite.Require().NoError(table.Read())

	assertPartitions(table.Partitions())
}

func (suite *GPTSuite) TestPartitionAddOutOfSpace() {
	table, err := gpt.NewGPT(suite.loopbackDevice.Name(), suite.dev)
	suite.Require().NoError(err)

	_, err = table.New()
	suite.Require().NoError(err)

	_, err = table.Add(size, partition.WithPartitionName("boot"))
	suite.Require().Error(err)
	suite.Assert().EqualError(err, `requested partition size 1099511627776, available is 1099511592960 (34816 too many bytes)`)
	suite.Assert().True(blockdevice.IsOutOfSpaceError(err))

	_, err = table.Add(size/2, partition.WithPartitionName("boot"))
	suite.Require().NoError(err)

	_, err = table.Add(size/2, partition.WithPartitionName("boot2"))
	suite.Require().Error(err)
	suite.Assert().EqualError(err, `requested partition size 549755813888, available is 549755779072 (34816 too many bytes)`)
	suite.Assert().True(blockdevice.IsOutOfSpaceError(err))

	_, err = table.Add(size/2-(headReserved+tailReserved)*blockSize, partition.WithPartitionName("boot2"))
	suite.Require().NoError(err)

	_, err = table.Add(0, partition.WithPartitionName("boot3"), partition.WithMaximumSize(true))
	suite.Require().Error(err)
	suite.Assert().EqualError(err, `requested partition with maximum size, but no space available`)
	suite.Assert().True(blockdevice.IsOutOfSpaceError(err))
}

func (suite *GPTSuite) TestPartitionDelete() {
	table, err := gpt.NewGPT(suite.loopbackDevice.Name(), suite.dev)
	suite.Require().NoError(err)

	_, err = table.New()
	suite.Require().NoError(err)

	const (
		bootSize   = 1048576
		grubSize   = 2 * bootSize
		efiSize    = 512 * 1048576
		configSize = blockSize
	)

	_, err = table.Add(bootSize, partition.WithPartitionName("boot"))
	suite.Require().NoError(err)

	_, err = table.Add(grubSize, partition.WithPartitionName("grub"))
	suite.Require().NoError(err)

	_, err = table.Add(efiSize, partition.WithPartitionName("efi"))
	suite.Require().NoError(err)

	_, err = table.Add(configSize, partition.WithPartitionName("config"))
	suite.Require().NoError(err)

	_, err = table.Add(0, partition.WithPartitionName("system"), partition.WithMaximumSize(true))
	suite.Require().NoError(err)

	suite.Require().NoError(table.Write())

	// re-read the partition table
	table, err = gpt.NewGPT(suite.loopbackDevice.Name(), suite.dev)
	suite.Require().NoError(err)

	suite.Require().NoError(table.Read())

	err = table.Delete(table.Partitions()[1])
	suite.Require().NoError(err)

	oldEFIPart := table.Partitions()[2]
	err = table.Delete(oldEFIPart)
	suite.Require().NoError(err)

	// double delete should fail
	err = table.Delete(oldEFIPart)
	suite.Require().Error(err)
	suite.Require().EqualError(err, "partition not found")

	suite.Require().NoError(table.Write())

	// re-read the partition table for the second time
	table, err = gpt.NewGPT(suite.loopbackDevice.Name(), suite.dev)
	suite.Require().NoError(err)

	suite.Require().NoError(table.Read())

	partitions := table.Partitions()
	suite.Require().Len(partitions, 3)

	partBoot := partitions[0]
	suite.Assert().EqualValues(1, partBoot.No())
	suite.Assert().EqualValues(bootSize/blockSize, partBoot.Length())

	partConfig := partitions[1]
	suite.Assert().EqualValues(2, partConfig.No())
	suite.Assert().EqualValues(configSize/blockSize, partConfig.Length())

	partSystem := partitions[2]
	suite.Assert().EqualValues(3, partSystem.No())
	suite.Assert().EqualValues((size-bootSize-efiSize-grubSize-configSize)/blockSize-headReserved-tailReserved, partSystem.Length())
}

func (suite *GPTSuite) TestPartitionInsertAt() {
	table, err := gpt.NewGPT(suite.loopbackDevice.Name(), suite.dev)
	suite.Require().NoError(err)

	_, err = table.New()
	suite.Require().NoError(err)

	const (
		oldBootSize = 1048576
		newBootSize = oldBootSize / 2
		grubSize    = newBootSize / 2
		configSize  = blockSize
		efiSize     = 512 * 1048576
	)

	_, err = table.Add(oldBootSize, partition.WithPartitionName("boot"))
	suite.Require().NoError(err)

	_, err = table.Add(configSize, partition.WithPartitionName("config"))
	suite.Require().NoError(err)

	_, err = table.Add(efiSize, partition.WithPartitionName("efi"))
	suite.Require().NoError(err)

	_, err = table.Add(0, partition.WithPartitionName("system"), partition.WithMaximumSize(true))
	suite.Require().NoError(err)

	suite.Require().NoError(table.Write())

	// re-read the partition table
	table, err = gpt.NewGPT(suite.loopbackDevice.Name(), suite.dev)
	suite.Require().NoError(err)

	suite.Require().NoError(table.Read())

	// delete first three partitions
	err = table.Delete(table.Partitions()[0])
	suite.Require().NoError(err)

	err = table.Delete(table.Partitions()[1])
	suite.Require().NoError(err)

	err = table.Delete(table.Partitions()[2])
	suite.Require().NoError(err)

	_, err = table.InsertAt(0, newBootSize, partition.WithPartitionName("boot"))
	suite.Require().NoError(err)

	_, err = table.InsertAt(1, grubSize, partition.WithPartitionName("grub"))
	suite.Require().NoError(err)

	_, err = table.InsertAt(2, configSize, partition.WithPartitionName("config"))
	suite.Require().NoError(err)

	_, err = table.InsertAt(3, 0, partition.WithPartitionName("efi"), partition.WithMaximumSize(true))
	suite.Require().NoError(err)

	partitions := table.Partitions()
	suite.Require().Len(partitions, 8)

	partBoot := partitions[0]
	suite.Assert().EqualValues(1, partBoot.No())
	suite.Assert().EqualValues(newBootSize/blockSize, partBoot.Length())
	suite.Assert().EqualValues(headReserved+1, partBoot.Start()) // first usable LBA

	partGrub := partitions[1]
	suite.Assert().EqualValues(2, partGrub.No())
	suite.Assert().EqualValues(grubSize/blockSize, partGrub.Length())
	suite.Assert().EqualValues(headReserved+1+newBootSize/blockSize, partGrub.Start())

	partConfig := partitions[2]
	suite.Assert().EqualValues(3, partConfig.No())
	suite.Assert().EqualValues(configSize/blockSize, partConfig.Length())
	suite.Assert().EqualValues(headReserved+1+(newBootSize+grubSize)/blockSize, partConfig.Start())

	partEFI := partitions[3]
	suite.Assert().EqualValues(4, partEFI.No())
	suite.Assert().EqualValues(((oldBootSize+configSize+efiSize)-(newBootSize+grubSize+configSize))/blockSize, partEFI.Length())
	suite.Assert().EqualValues(headReserved+1+(newBootSize+grubSize+configSize)/blockSize, partEFI.Start())

	suite.Assert().Nil(partitions[4]) // tombstones
	suite.Assert().Nil(partitions[5])
	suite.Assert().Nil(partitions[6])

	// system partition should stay unchanged
	partSystem := partitions[7]
	suite.Assert().EqualValues(5, partSystem.No())
	suite.Assert().EqualValues((size-(oldBootSize+configSize+efiSize))/blockSize-headReserved-tailReserved, partSystem.Length())
	suite.Assert().EqualValues(headReserved+1+(oldBootSize+configSize+efiSize)/blockSize, partSystem.Start())

	suite.Require().NoError(table.Write())
}

func TestGPTSuite(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("can't run the test as non-root")
	}

	suite.Run(t, new(GPTSuite))
}
