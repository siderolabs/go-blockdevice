// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gpt_test

import (
	"encoding/binary"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/siderolabs/go-blockdevice/blockdevice"
	"github.com/siderolabs/go-blockdevice/blockdevice/lba"
	"github.com/siderolabs/go-blockdevice/blockdevice/loopback"
	"github.com/siderolabs/go-blockdevice/blockdevice/partition/gpt"
	"github.com/siderolabs/go-blockdevice/blockdevice/test"
)

const (
	size      = 1024 * 1024 * 1024 * 1024
	blockSize = 512
	alignment = lba.RecommendedAlignment / blockSize

	headReserved = (34 + alignment - 1) / alignment * alignment
	tailReserved = (33 + alignment - 1) / alignment * alignment
)

type GPTSuite struct {
	test.BlockDeviceSuite
}

func (suite *GPTSuite) SetupTest() {
	suite.CreateBlockDevice(size)
}

func (suite *GPTSuite) TestEmpty() {
	_, err := gpt.Open(suite.Dev)
	suite.Require().EqualError(err, gpt.ErrPartitionTableDoesNotExist.Error())
}

func (suite *GPTSuite) TestReset() {
	g, err := gpt.New(suite.Dev)
	suite.Require().NoError(err)

	_, err = g.Add(1048576, gpt.WithPartitionName("boot"))
	suite.Require().NoError(err)

	_, err = g.Add(1048576, gpt.WithPartitionName("efi"))
	suite.Require().NoError(err)

	suite.Require().NoError(g.Write())

	suite.validatePMBR(0x00)

	// re-read the partition table
	g, err = gpt.Open(suite.Dev)
	suite.Require().NoError(err)

	suite.Require().NoError(g.Read())

	for _, p := range g.Partitions().Items() {
		suite.Require().NoError(g.Delete(p))
	}

	suite.Require().NoError(g.Write())

	suite.validatePMBR(0x00)

	// re-read the partition table
	g, err = gpt.Open(suite.Dev)
	suite.Require().NoError(err)

	suite.Require().NoError(g.Read())

	suite.Assert().Empty(g.Partitions().Items())

	suite.validatePartitions()
}

func (suite *GPTSuite) TestPartitionAdd() {
	g, err := gpt.New(suite.Dev)
	suite.Require().NoError(err)

	const (
		bootSize = 1048576
		efiSize  = 512 * 1048576
	)

	_, err = g.Add(bootSize, gpt.WithPartitionName("boot"))
	suite.Require().NoError(err)

	_, err = g.Add(efiSize, gpt.WithPartitionName("efi"))
	suite.Require().NoError(err)

	_, err = g.Add(0, gpt.WithPartitionName("system"), gpt.WithMaximumSize(true))
	suite.Require().NoError(err)

	assertPartitions := func(partitions *gpt.Partitions) { //nolint: dupl
		suite.Require().Len(partitions.Items(), 3)

		partBoot := partitions.Items()[0]
		suite.Assert().EqualValues(1, partBoot.Number)
		suite.Assert().EqualValues("boot", partBoot.Name)
		suite.Assert().EqualValues(bootSize/blockSize, partBoot.Length())
		suite.Assert().EqualValues(headReserved, partBoot.FirstLBA)

		partEFI := partitions.Items()[1]
		suite.Assert().EqualValues(2, partEFI.Number)
		suite.Assert().EqualValues("efi", partEFI.Name)
		suite.Assert().EqualValues(efiSize/blockSize, partEFI.Length())
		suite.Assert().EqualValues(headReserved+bootSize/blockSize, partEFI.FirstLBA)

		partSystem := partitions.Items()[2]
		suite.Assert().EqualValues(3, partSystem.Number)
		suite.Assert().EqualValues("system", partSystem.Name)
		suite.Assert().EqualValues((size-bootSize-efiSize)/blockSize-headReserved-tailReserved, partSystem.Length())
		suite.Assert().EqualValues(headReserved+bootSize/blockSize+efiSize/blockSize, partSystem.FirstLBA)
	}

	assertPartitions(g.Partitions())

	part := g.Partitions().FindByName("system")
	suite.Require().NotNil(part)

	suite.Require().NoError(g.Write())

	// re-read the partition table
	g, err = gpt.Open(suite.Dev)
	suite.Require().NoError(err)

	suite.Require().NoError(g.Read())

	assertPartitions(g.Partitions())

	suite.validatePartitions()
}

func (suite *GPTSuite) TestRepairResize() {
	const newSize = 2 * size

	g, err := gpt.New(suite.Dev)
	suite.Require().NoError(err)

	const (
		bootSize = 1048576
		efiSize  = 512 * 1048576
	)

	_, err = g.Add(bootSize, gpt.WithPartitionName("boot"))
	suite.Require().NoError(err)

	_, err = g.Add(efiSize, gpt.WithPartitionName("efi"))
	suite.Require().NoError(err)

	_, err = g.Add(0, gpt.WithPartitionName("system"), gpt.WithMaximumSize(true))
	suite.Require().NoError(err)

	resized, err := g.Resize(g.Partitions().Items()[2])
	suite.Require().NoError(err)

	suite.Assert().False(resized)

	suite.Require().NoError(g.Write())

	// detach loopback device, resize file and attach loopback device back
	suite.Assert().NoError(suite.Dev.Close())

	if suite.LoopbackDevice != nil {
		suite.Assert().NoError(loopback.Unloop(suite.LoopbackDevice))
	}

	suite.Require().NoError(suite.File.Truncate(newSize))

	suite.LoopbackDevice, err = loopback.NextLoopDevice()
	suite.Require().NoError(err)

	suite.T().Logf("Using %s", suite.LoopbackDevice.Name())

	suite.Require().NoError(loopback.Loop(suite.LoopbackDevice, suite.File))

	suite.Require().NoError(loopback.LoopSetReadWrite(suite.LoopbackDevice))

	suite.Dev, err = os.OpenFile(suite.LoopbackDevice.Name(), os.O_RDWR, 0)
	suite.Require().NoError(err)

	// re-read the partition table
	g, err = gpt.Open(suite.Dev)
	suite.Require().NoError(err)

	suite.Require().NoError(g.Read())

	suite.Require().NoError(g.Repair())

	resized, err = g.Resize(g.Partitions().Items()[2])
	suite.Require().NoError(err)

	suite.Assert().True(resized)

	suite.Require().NoError(g.Write())

	assertPartitions := func(partitions *gpt.Partitions) { //nolint: dupl
		suite.Require().Len(partitions.Items(), 3)

		partBoot := partitions.Items()[0]
		suite.Assert().EqualValues(1, partBoot.Number)
		suite.Assert().EqualValues("boot", partBoot.Name)
		suite.Assert().EqualValues(bootSize/blockSize, partBoot.Length())
		suite.Assert().EqualValues(headReserved, partBoot.FirstLBA)

		partEFI := partitions.Items()[1]
		suite.Assert().EqualValues(2, partEFI.Number)
		suite.Assert().EqualValues("efi", partEFI.Name)
		suite.Assert().EqualValues(efiSize/blockSize, partEFI.Length())
		suite.Assert().EqualValues(headReserved+bootSize/blockSize, partEFI.FirstLBA)

		partSystem := partitions.Items()[2]
		suite.Assert().EqualValues(3, partSystem.Number)
		suite.Assert().EqualValues("system", partSystem.Name)
		suite.Assert().EqualValues((newSize-bootSize-efiSize)/blockSize-headReserved-tailReserved, partSystem.Length())
		suite.Assert().EqualValues(headReserved+bootSize/blockSize+efiSize/blockSize, partSystem.FirstLBA)
	}

	// re-read the partition table
	g, err = gpt.Open(suite.Dev)
	suite.Require().NoError(err)

	suite.Require().NoError(g.Read())

	assertPartitions(g.Partitions())

	suite.validatePartitions()
}

func (suite *GPTSuite) TestPartitionAddOutOfSpace() {
	g, err := gpt.New(suite.Dev)
	suite.Require().NoError(err)

	_, err = g.Add(size, gpt.WithPartitionName("boot"))
	suite.Require().Error(err)
	suite.Assert().EqualError(err, `requested partition size 1099511627776, available is 1099510561792 (1065984 too many bytes)`)
	suite.Assert().True(blockdevice.IsOutOfSpaceError(err))

	_, err = g.Add(size/2, gpt.WithPartitionName("boot"))
	suite.Require().NoError(err)

	_, err = g.Add(size/2, gpt.WithPartitionName("boot2"))
	suite.Require().Error(err)
	suite.Assert().EqualError(err, `requested partition size 549755813888, available is 549754747904 (1065984 too many bytes)`)
	suite.Assert().True(blockdevice.IsOutOfSpaceError(err))

	_, err = g.Add(size/2-(headReserved+tailReserved)*blockSize, gpt.WithPartitionName("boot2"))
	suite.Require().NoError(err)

	_, err = g.Add(0, gpt.WithPartitionName("boot3"), gpt.WithMaximumSize(true))
	suite.Require().Error(err)
	suite.Assert().EqualError(err, `requested partition with maximum size, but no space available`)
	suite.Assert().True(blockdevice.IsOutOfSpaceError(err))
}

func (suite *GPTSuite) TestPartitionDelete() {
	g, err := gpt.New(suite.Dev)
	suite.Require().NoError(err)

	const (
		bootSize   = 1048576
		grubSize   = 2 * bootSize
		efiSize    = 512 * 1048576
		configSize = 1048576
	)

	_, err = g.Add(bootSize, gpt.WithPartitionName("boot"))
	suite.Require().NoError(err)

	_, err = g.Add(grubSize, gpt.WithPartitionName("grub"))
	suite.Require().NoError(err)

	_, err = g.Add(efiSize, gpt.WithPartitionName("efi"))
	suite.Require().NoError(err)

	_, err = g.Add(configSize, gpt.WithPartitionName("config"))
	suite.Require().NoError(err)

	_, err = g.Add(0, gpt.WithPartitionName("system"), gpt.WithMaximumSize(true))
	suite.Require().NoError(err)

	suite.Require().NoError(g.Write())

	// re-read the partition table
	g, err = gpt.Open(suite.Dev)
	suite.Require().NoError(err)

	suite.Require().NoError(g.Read())

	err = g.Delete(g.Partitions().Items()[1])
	suite.Require().NoError(err)

	oldEFIPart := g.Partitions().Items()[2]
	err = g.Delete(oldEFIPart)
	suite.Require().NoError(err)

	// double delete should fail
	err = g.Delete(oldEFIPart)
	suite.Require().Error(err)
	suite.Require().EqualError(err, "partition not found")

	suite.Require().NoError(g.Write())

	// re-read the partition table for the second time
	g, err = gpt.Open(suite.Dev)
	suite.Require().NoError(err)

	suite.Require().NoError(g.Read())

	partitions := g.Partitions().Items()
	suite.Require().Len(partitions, 3)

	partBoot := partitions[0]
	suite.Assert().EqualValues(1, partBoot.Number)
	suite.Assert().EqualValues(bootSize/blockSize, partBoot.Length())

	partConfig := partitions[1]
	suite.Assert().EqualValues(2, partConfig.Number)
	suite.Assert().EqualValues(configSize/blockSize, partConfig.Length())

	partSystem := partitions[2]
	suite.Assert().EqualValues(3, partSystem.Number)
	suite.Assert().EqualValues((size-bootSize-efiSize-grubSize-configSize)/blockSize-headReserved-tailReserved, partSystem.Length())

	suite.validatePartitions()
}

func (suite *GPTSuite) TestPartitionInsertAt() {
	g, err := gpt.New(suite.Dev)
	suite.Require().NoError(err)

	const (
		oldBootSize = 4 * 1048576
		newBootSize = oldBootSize / 2
		grubSize    = newBootSize / 2
		configSize  = 1048576
		efiSize     = 512 * 1048576
	)

	_, err = g.Add(oldBootSize, gpt.WithPartitionName("boot"))
	suite.Require().NoError(err)

	_, err = g.Add(configSize, gpt.WithPartitionName("config"))
	suite.Require().NoError(err)

	_, err = g.Add(efiSize, gpt.WithPartitionName("efi"))
	suite.Require().NoError(err)

	_, err = g.Add(0, gpt.WithPartitionName("system"), gpt.WithMaximumSize(true))
	suite.Require().NoError(err)

	suite.Require().NoError(g.Write())

	// re-read the partition table
	g, err = gpt.New(suite.Dev)
	suite.Require().NoError(err)

	suite.Require().NoError(g.Read())

	// delete first three partitions
	err = g.Delete(g.Partitions().Items()[0])
	suite.Require().NoError(err)

	err = g.Delete(g.Partitions().Items()[1])
	suite.Require().NoError(err)

	err = g.Delete(g.Partitions().Items()[2])
	suite.Require().NoError(err)

	_, err = g.InsertAt(0, newBootSize, gpt.WithPartitionName("boot"))
	suite.Require().NoError(err)

	_, err = g.InsertAt(1, grubSize, gpt.WithPartitionName("grub"))
	suite.Require().NoError(err)

	_, err = g.InsertAt(2, configSize, gpt.WithPartitionName("config"))
	suite.Require().NoError(err)

	_, err = g.InsertAt(3, 0, gpt.WithPartitionName("efi"), gpt.WithMaximumSize(true))
	suite.Require().NoError(err)

	partitions := g.Partitions().Items()
	suite.Require().Len(partitions, 8)

	partBoot := partitions[0]
	suite.Assert().EqualValues(1, partBoot.Number)
	suite.Assert().EqualValues(newBootSize/blockSize, partBoot.Length())
	suite.Assert().EqualValues(headReserved, partBoot.FirstLBA)

	partGrub := partitions[1]
	suite.Assert().EqualValues(2, partGrub.Number)
	suite.Assert().EqualValues(grubSize/blockSize, partGrub.Length())
	suite.Assert().EqualValues(headReserved+newBootSize/blockSize, partGrub.FirstLBA)

	partConfig := partitions[2]
	suite.Assert().EqualValues(3, partConfig.Number)
	suite.Assert().EqualValues(configSize/blockSize, partConfig.Length())
	suite.Assert().EqualValues(headReserved+(newBootSize+grubSize)/blockSize, partConfig.FirstLBA)

	partEFI := partitions[3]
	suite.Assert().EqualValues(4, partEFI.Number)
	suite.Assert().EqualValues(((oldBootSize+configSize+efiSize)-(newBootSize+grubSize+configSize))/blockSize, partEFI.Length())
	suite.Assert().EqualValues(headReserved+(newBootSize+grubSize+configSize)/blockSize, partEFI.FirstLBA)

	suite.Assert().Nil(partitions[4]) // tombstones
	suite.Assert().Nil(partitions[5])
	suite.Assert().Nil(partitions[6])

	// system partition should stay unchanged
	partSystem := partitions[7]
	suite.Assert().EqualValues(5, partSystem.Number)
	suite.Assert().EqualValues((size-(oldBootSize+configSize+efiSize))/blockSize-headReserved-tailReserved, partSystem.Length())
	suite.Assert().EqualValues(headReserved+(oldBootSize+configSize+efiSize)/blockSize, partSystem.FirstLBA)

	suite.Require().NoError(g.Write())

	suite.validatePartitions()
}

func (suite *GPTSuite) TestPartitionInsertOffsetAndResize() {
	g, err := gpt.New(suite.Dev)
	suite.Require().NoError(err)

	const (
		bootSize    = 1048576
		efiSize     = 512 * 1048576
		configStart = size - (2 * blockSize * 1048576)
	)

	_, err = g.Add(bootSize, gpt.WithPartitionName("boot"))
	suite.Require().NoError(err)

	_, err = g.Add(efiSize, gpt.WithPartitionName("efi"))
	suite.Require().NoError(err)

	_, err = g.Add(bootSize, gpt.WithPartitionName("system"))
	suite.Require().NoError(err)

	_, err = g.Add(0, gpt.WithPartitionName("config"), gpt.WithOffset(configStart), gpt.WithMaximumSize(true))
	suite.Require().NoError(err)

	suite.Require().NoError(g.Write())

	// re-read the partition table
	g, err = gpt.Open(suite.Dev)
	suite.Require().NoError(err)

	suite.Require().NoError(g.Read())

	suite.Require().NoError(g.Repair())

	resized, err := g.Resize(g.Partitions().Items()[2])
	suite.Require().NoError(err)

	suite.Assert().True(resized)

	suite.Require().NoError(g.Write())

	// re-read the partition table
	g, err = gpt.New(suite.Dev)
	suite.Require().NoError(err)

	suite.Require().NoError(g.Read())

	partitions := g.Partitions().Items()
	suite.Require().Len(partitions, 4)

	partBoot := partitions[0]
	suite.Assert().EqualValues(1, partBoot.Number)
	suite.Assert().EqualValues(bootSize/blockSize, partBoot.Length())
	suite.Assert().EqualValues(headReserved, partBoot.FirstLBA)

	partEFI := partitions[1]
	suite.Assert().EqualValues(2, partEFI.Number)
	suite.Assert().EqualValues(efiSize/blockSize, partEFI.Length())
	suite.Assert().EqualValues(headReserved+bootSize/blockSize, partEFI.FirstLBA)

	partSystem := partitions[2]
	suite.Assert().EqualValues(3, partSystem.Number)
	suite.Assert().EqualValues((configStart-bootSize-efiSize)/blockSize-headReserved, int(partSystem.Length()))
	suite.Assert().EqualValues(headReserved+(bootSize+efiSize)/blockSize, partSystem.FirstLBA)
	suite.Assert().EqualValues(configStart/blockSize-1, partSystem.LastLBA)

	partConfig := partitions[3]
	suite.Assert().EqualValues(4, partConfig.Number)
	suite.Assert().EqualValues((size-configStart)/blockSize-tailReserved, partConfig.Length())
	suite.Assert().EqualValues(configStart/blockSize, int(partConfig.FirstLBA))

	suite.Require().NoError(g.Write())

	suite.validatePartitions()
}

func (suite *GPTSuite) TestPartitionGUUID() {
	g, err := gpt.New(suite.Dev)
	suite.Require().NoError(err)

	err = g.Write()
	suite.Require().NoError(err)

	suite.Assert().NotEqual(g.Header().GUUID.String(), "00000000-0000-0000-0000-000000000000")

	g, err = gpt.Open(suite.Dev)
	suite.Require().NoError(err)

	err = g.Read()
	suite.Require().NoError(err)

	suite.Assert().NotEqual(g.Header().GUUID.String(), "00000000-0000-0000-0000-000000000000")
}

func (suite *GPTSuite) TestMarkPMBRBootable() {
	g, err := gpt.New(suite.Dev, gpt.WithMarkMBRBootable(true))
	suite.Require().NoError(err)

	_, err = g.Add(1048576, gpt.WithPartitionName("boot"))
	suite.Require().NoError(err)

	_, err = g.Add(1048576, gpt.WithPartitionName("efi"))
	suite.Require().NoError(err)

	suite.Require().NoError(g.Write())

	suite.validatePMBR(0x80)

	// re-read the partition table
	g, err = gpt.Open(suite.Dev)
	suite.Require().NoError(err)

	suite.Require().NoError(g.Read())
	suite.Require().NoError(g.Write())

	suite.validatePMBR(0x80)

	suite.validatePartitions()
}

func (suite *GPTSuite) validatePMBR(flag byte) {
	buf := make([]byte, 512)

	n, err := suite.Dev.ReadAt(buf, 0)
	suite.Require().NoError(err)
	suite.Require().EqualValues(512, n)

	partition := buf[446:460]

	suite.Assert().Equal(flag, partition[0])                                                     // active flag
	suite.Assert().Equal([]byte{0x00, 0x02, 0x00}, partition[1:4])                               // CHS start
	suite.Assert().Equal(byte(0xee), partition[4])                                               // partition type
	suite.Assert().Equal([]byte{0xff, 0xff, 0xff}, partition[5:8])                               // CHS end
	suite.Assert().Equal(uint32(1), binary.LittleEndian.Uint32(partition[8:12]))                 // LBA start
	suite.Assert().Equal(uint32(size/blockSize-1), binary.LittleEndian.Uint32(partition[12:16])) // length in sectors

	suite.Assert().Equal([]byte{0x55, 0xaa}, buf[510:512]) // boot signature
}

func (suite *GPTSuite) validatePartitions() {
	for _, tool := range []string{"fdisk", "gdisk"} {
		if toolPath, _ := exec.LookPath(tool); toolPath != "" { //nolint:errcheck
			cmd := exec.Command(toolPath, "-l", suite.Dev.Name())
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			suite.Assert().NoError(cmd.Run())
		} else {
			suite.T().Logf("%s is not available", tool)
		}
	}
}

func TestGPTSuite(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("can't run the test as non-root")
	}

	suite.Run(t, new(GPTSuite))
}
