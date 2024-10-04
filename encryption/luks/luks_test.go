// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package luks_test

import (
	"context"
	"errors"
	randv2 "math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/freddierice/go-losetup/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/siderolabs/go-blockdevice/v2/block"
	"github.com/siderolabs/go-blockdevice/v2/encryption"
	"github.com/siderolabs/go-blockdevice/v2/encryption/luks"
	"github.com/siderolabs/go-blockdevice/v2/partitioning"
	"github.com/siderolabs/go-blockdevice/v2/partitioning/gpt"
)

const (
	size = 1024 * 1024 * 512
)

func testEncrypt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	tmpDir := t.TempDir()

	rawImage := filepath.Join(tmpDir, "image.raw")

	f, err := os.Create(rawImage)
	require.NoError(t, err)

	require.NoError(t, f.Truncate(int64(size)))
	require.NoError(t, f.Close())

	loDev := losetupAttachHelper(t, rawImage, false)

	t.Cleanup(func() {
		assert.NoError(t, loDev.Detach())
	})

	devPath := loDev.Path()

	blkdev, err := block.NewFromPath(devPath, block.OpenForWrite())
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, blkdev.Close())
	})

	require.NoError(t, blkdev.Lock(true))

	t.Cleanup(func() {
		assert.NoError(t, blkdev.Unlock())
	})

	gptdev, err := gpt.DeviceFromBlockDevice(blkdev)
	require.NoError(t, err)

	partitions, err := gpt.New(gptdev)
	require.NoError(t, err)

	const (
		bootSize   = 1024 * 512
		configSize = 1024 * 1024 * 32
	)

	_, _, err = partitions.AllocatePartition(bootSize, "boot", uuid.MustParse("C12A7328-F81F-11D2-BA4B-00A0C93EC93B"))
	require.NoError(t, err)

	_, _, err = partitions.AllocatePartition(configSize, "config", uuid.MustParse("C12A7328-F81F-11D2-BA4B-00A0C93EC93B"))
	require.NoError(t, err)

	require.NoError(t, partitions.Write())

	var _ encryption.Provider = &luks.LUKS{}

	key := encryption.NewKey(0, []byte("changeme"))
	keyExtra := encryption.NewKey(1, []byte("helloworld"))

	provider := luks.New(
		luks.AESXTSPlain64Cipher,
		luks.WithIterTime(time.Millisecond*100),
		luks.WithPerfOptions(luks.PerfSameCPUCrypt),
	)

	path := partitioning.DevName(devPath, 2)
	mappedName := filepath.Base(path) + "-encrypted"

	t.Logf("unencrypted partition path %s", path)

	isOpen, _, err := provider.IsOpen(ctx, path, mappedName)
	require.NoError(t, err)
	require.False(t, isOpen)

	require.NoError(t, provider.Encrypt(ctx, path, key))

	isOpen, _, err = provider.IsOpen(ctx, path, mappedName)
	require.NoError(t, err)
	require.False(t, isOpen)

	encryptedPath, err := provider.Open(ctx, path, mappedName, key)
	require.NoError(t, err)

	isOpen, isOpenPath, err := provider.IsOpen(ctx, path, mappedName)
	require.NoError(t, err)
	require.True(t, isOpen)
	require.Equal(t, encryptedPath, isOpenPath)

	require.NoError(t, provider.Resize(ctx, encryptedPath, key))

	require.NoError(t, provider.AddKey(ctx, path, key, keyExtra))
	require.NoError(t, provider.SetKey(ctx, path, keyExtra, keyExtra))

	valid, err := provider.CheckKey(ctx, path, keyExtra)
	require.NoError(t, err)
	require.True(t, valid)

	valid, err = provider.CheckKey(ctx, path, encryption.NewKey(1, []byte("nope")))
	require.NoError(t, err)
	require.False(t, valid)

	mountPath := t.TempDir()

	cmd := exec.Command("mkfs.vfat", "-F", "32", "-n", "config", encryptedPath)
	require.NoError(t, cmd.Run())

	type SealedKey struct {
		SealedKey string `json:"sealed_key"`
	}

	token := &luks.Token[SealedKey]{
		UserData: SealedKey{
			SealedKey: "aaaa",
		},
		Type: "sealedkey",
	}

	err = provider.SetToken(ctx, path, 0, token)
	require.NoError(t, err)

	err = provider.ReadToken(ctx, path, 0, token)
	require.NoError(t, err)

	require.Equal(t, token.UserData.SealedKey, "aaaa")

	require.NoError(t, provider.RemoveToken(ctx, path, 0))
	require.Error(t, provider.ReadToken(ctx, path, 0, token))

	// create and replace token
	err = provider.SetToken(ctx, path, 0, token)
	require.NoError(t, err)

	token.UserData.SealedKey = "bbbb"

	err = provider.SetToken(ctx, path, 0, token)
	require.NoError(t, err)

	require.NoError(t, unix.Mount(encryptedPath, mountPath, "vfat", 0, ""))
	require.NoError(t, unix.Unmount(mountPath, 0))

	require.NoError(t, provider.Close(ctx, encryptedPath))

	isOpen, _, err = provider.IsOpen(ctx, path, mappedName)
	require.NoError(t, err)
	require.False(t, isOpen)

	require.Error(t, provider.Close(ctx, encryptedPath))

	// second key slot
	encryptedPath, err = provider.Open(ctx, path, mappedName, keyExtra)
	require.NoError(t, err)
	require.NoError(t, provider.Close(ctx, encryptedPath))

	// check keyslots list
	keyslots, err := provider.ReadKeyslots(path)
	require.NoError(t, err)

	_, ok := keyslots.Keyslots["0"]
	require.True(t, ok)
	_, ok = keyslots.Keyslots["1"]
	require.True(t, ok)

	// remove key slot
	err = provider.RemoveKey(ctx, path, 1, key)
	require.NoError(t, err)
	_, err = provider.Open(ctx, path, mappedName, keyExtra)
	require.Equal(t, err, encryption.ErrEncryptionKeyRejected)

	valid, err = provider.CheckKey(ctx, path, key)
	require.NoError(t, err)
	require.True(t, valid)

	// unhappy cases
	_, err = provider.Open(ctx, path, mappedName, encryption.NewKey(0, []byte("エクスプロシオン")))
	require.Equal(t, err, encryption.ErrEncryptionKeyRejected)

	_, err = provider.Open(ctx, "/dev/nosuchdevice", mappedName, encryption.NewKey(0, []byte("エクスプロシオン")))
	require.Error(t, err)

	_, err = provider.Open(ctx, loDev.Path(), mappedName, key)
	require.Error(t, err)
}

func TestLUKS(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("can't run the test as non-root")
	}

	if hostname, _ := os.Hostname(); hostname == "buildkitsandbox" { //nolint:errcheck
		t.Skip("test not supported under buildkit as partition devices are not propagated from /dev")
	}

	t.Run("Encrypt", testEncrypt)
}

func losetupAttachHelper(t *testing.T, rawImage string, readonly bool) losetup.Device {
	t.Helper()

	for range 10 {
		loDev, err := losetup.Attach(rawImage, 0, readonly)
		if err != nil {
			if errors.Is(err, unix.EBUSY) {
				spraySleep := max(randv2.ExpFloat64(), 2.0)

				t.Logf("retrying after %v seconds", spraySleep)

				time.Sleep(time.Duration(spraySleep * float64(time.Second)))

				continue
			}
		}

		require.NoError(t, err)

		return loDev
	}

	t.Fatal("failed to attach loop device") //nolint:revive

	panic("unreachable")
}
