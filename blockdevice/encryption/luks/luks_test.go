// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package luks_test

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"golang.org/x/sys/unix"

	"github.com/siderolabs/go-blockdevice/blockdevice"
	"github.com/siderolabs/go-blockdevice/blockdevice/encryption"
	"github.com/siderolabs/go-blockdevice/blockdevice/encryption/luks"
	"github.com/siderolabs/go-blockdevice/blockdevice/partition/gpt"
	"github.com/siderolabs/go-blockdevice/blockdevice/test"
)

const (
	size = 1024 * 1024 * 512
)

type LUKSSuite struct {
	test.BlockDeviceSuite
}

func (suite *LUKSSuite) SetupTest() {
	suite.CreateBlockDevice(size)
}

func (suite *LUKSSuite) TestEncrypt() {
	bd, err := blockdevice.Open(
		suite.LoopbackDevice.Name(),
		blockdevice.WithNewGPT(true),
	)

	var _ encryption.Provider = &luks.LUKS{}

	suite.Require().NoError(err)

	g, err := bd.PartitionTable()
	suite.Require().NoError(err)

	const (
		bootSize   = 1024 * 512
		configSize = 1024 * 1024 * 32
	)

	var configPartition *gpt.Partition

	key := encryption.NewKey(0, []byte("changeme"))
	keyExtra := encryption.NewKey(1, []byte("helloworld"))

	provider := luks.New(
		luks.AESXTSPlain64Cipher,
		luks.WithIterTime(time.Millisecond*100),
		luks.WithPerfOptions(luks.PerfSameCPUCrypt),
	)

	_, err = g.Add(bootSize, gpt.WithPartitionName("boot"))
	suite.Require().NoError(err)

	configPartition, err = g.Add(
		configSize,
		gpt.WithPartitionName("config"),
	)
	suite.Require().NoError(err)
	suite.Require().NoError(g.Write())

	path, err := bd.PartPath(configPartition.Name)
	suite.Require().NoError(err)
	suite.T().Logf("unencrypted partition path %s", path)

	suite.Require().NoError(provider.Encrypt(path, key))
	encryptedPath, err := provider.Open(path, key)
	suite.Require().NoError(err)

	suite.Require().NoError(provider.AddKey(path, key, keyExtra))
	suite.Require().NoError(provider.SetKey(path, keyExtra, keyExtra))
	valid, err := provider.CheckKey(path, keyExtra)
	suite.Require().NoError(err)
	suite.Require().True(valid)

	valid, err = provider.CheckKey(path, encryption.NewKey(1, []byte("nope")))
	suite.Require().NoError(err)
	suite.Require().False(valid)

	bdEncrypted, err := blockdevice.Open(encryptedPath)
	suite.Require().NoError(err)

	part, err := bd.GetPartition("config")
	suite.Require().NoError(err)

	encrypted, err := part.Encrypted()
	suite.Require().NoError(err)
	suite.Require().True(encrypted)

	mountPath := suite.T().TempDir()

	cmd := exec.Command("mkfs.vfat", "-F", "32", "-n", part.Name, encryptedPath)
	suite.Require().NoError(cmd.Run())

	type SealedKey struct {
		SealedKey string `json:"sealed_key"`
	}

	token := &luks.Token[SealedKey]{
		UserData: SealedKey{
			SealedKey: "aaaa",
		},
		Type: "sealedkey",
	}

	err = provider.SetToken(path, 0, token)
	suite.Require().NoError(err)

	err = provider.ReadToken(path, 0, token)
	suite.Require().NoError(err)

	suite.Require().Equal(token.UserData.SealedKey, "aaaa")

	suite.Require().NoError(provider.RemoveToken(path, 0))
	suite.Require().Error(provider.ReadToken(path, 0, token))

	// create and replace token
	err = provider.SetToken(path, 0, token)
	suite.Require().NoError(err)

	token.UserData.SealedKey = "bbbb"

	err = provider.SetToken(path, 0, token)
	suite.Require().NoError(err)

	suite.Require().NoError(unix.Mount(encryptedPath, mountPath, "vfat", 0, ""))
	suite.Require().NoError(unix.Unmount(mountPath, 0))

	suite.Require().NoError(bdEncrypted.Close())

	suite.Require().NoError(provider.Close(encryptedPath))
	suite.Require().Error(provider.Close(encryptedPath))

	// second key slot
	_, err = provider.Open(path, keyExtra)
	suite.Require().NoError(err)
	suite.Require().NoError(provider.Close(encryptedPath))

	// check keyslots list
	keyslots, err := provider.ReadKeyslots(path)
	suite.Require().NoError(err)

	_, ok := keyslots.Keyslots["0"]
	suite.Require().True(ok)
	_, ok = keyslots.Keyslots["1"]
	suite.Require().True(ok)

	// remove key slot
	err = provider.RemoveKey(path, 1, key)
	suite.Require().NoError(err)
	_, err = provider.Open(path, keyExtra)
	suite.Require().Equal(err, encryption.ErrEncryptionKeyRejected)

	valid, err = provider.CheckKey(path, key)
	suite.Require().NoError(err)
	suite.Require().True(valid)

	// unhappy cases
	_, err = provider.Open(path, encryption.NewKey(0, []byte("エクスプロシオン")))
	suite.Require().Equal(err, encryption.ErrEncryptionKeyRejected)

	_, err = provider.Open("/dev/nosuchdevice", encryption.NewKey(0, []byte("エクスプロシオン")))
	suite.Require().Error(err)

	_, err = provider.Open(suite.LoopbackDevice.Name(), key)
	suite.Require().Error(err)
}

func TestLUKSSuite(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("can't run the test as non-root")
	}

	if hostname, _ := os.Hostname(); hostname == "buildkitsandbox" { //nolint:errcheck
		t.Skip("test not supported under buildkit as partition devices are not propagated from /dev")
	}

	suite.Run(t, new(LUKSSuite))
}
