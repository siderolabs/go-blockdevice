// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package disk_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/talos-systems/go-blockdevice/blockdevice/util/disk"
)

type DisksSuite struct {
	suite.Suite
}

func (suite *DisksSuite) TestDisks() {
	disks, err := disk.List()
	suite.Require().NoError(err)

	if len(disks) > 0 {
		for _, d := range disks {
			suite.Require().NotEmpty(d.DeviceName)
			suite.Require().NotEmpty(d.Model)
		}
	}
}

func (suite *DisksSuite) TestDisk() {
	if os.Getuid() != 0 {
		suite.T().Skip("can't run the test as non-root")
	}

	hostname, _ := os.Hostname() //nolint: errcheck

	if hostname == "buildkitsandbox" {
		suite.T().Skip("test not supported under buildkit as partition devices are not propagated from /dev")
	}

	disks, err := disk.List()
	suite.Require().NoError(err)

	if len(disks) > 0 {
		d := disks[0]

		suite.Require().NotEmpty(d.Model)

		d, err = disk.Find(disk.WithName("*"))

		suite.Require().NoError(err)
		suite.Require().NotNil(d)
	}
}

func (suite *DisksSuite) TestDiskMatcher() {
	hdd := &disk.Disk{
		Model: "WDC  WDS100T2B0B",
		Size:  1e+9,
		WWID:  "naa.5044cca67bddsd",
		UUID:  "fake-uuid-string-whatever",
	}

	sdCard := &disk.Disk{
		Serial: "0xeb791622",
		Name:   "SC32G",
		Size:   1e+8,
	}

	sdCard2 := &disk.Disk{
		Serial: "0xeb791633",
		Name:   "SC32G",
		Size:   1e+8,
	}

	tests := []struct {
		disk     *disk.Disk
		matchers []disk.Matcher
		match    bool
	}{
		{
			disk: hdd,
			matchers: []disk.Matcher{
				disk.WithWWID(hdd.WWID),
			},
			match: true,
		},
		{
			disk: sdCard2,
			matchers: []disk.Matcher{
				disk.WithWWID(sdCard2.Name),
				disk.WithWWID(sdCard.Serial),
			},
			match: false,
		},
		{
			disk: hdd,
			matchers: []disk.Matcher{
				disk.WithModel("WDC*"),
			},
			match: true,
		},
		{
			disk: hdd,
			matchers: []disk.Matcher{
				disk.WithModel("WDC*100*"),
			},
			match: true,
		},
		{
			disk: hdd,
			matchers: []disk.Matcher{
				disk.WithModel("*WDC*"),
			},
			match: true,
		},
		{
			disk: hdd,
			matchers: []disk.Matcher{
				disk.WithModel("WDC*101*"),
			},
			match: false,
		},
		{
			disk: hdd,
			matchers: []disk.Matcher{
				disk.WithUUID(hdd.UUID),
			},
			match: true,
		},
	}

	for i, test := range tests {
		matched := disk.Match(test.disk, test.matchers...)
		suite.Require().Equal(test.match, matched, fmt.Sprintf("test %d", i))
	}
}

func TestDisksSuite(t *testing.T) {
	suite.Run(t, new(DisksSuite))
}
