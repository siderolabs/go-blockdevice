// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package block_test

import (
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/freddierice/go-losetup/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/siderolabs/go-blockdevice/v2/block"
)

func TestDeviceWipe(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("skipping test; must be root")
	}

	tmpDir := t.TempDir()

	rawImage := filepath.Join(tmpDir, "image.raw")

	f, err := os.Create(rawImage)
	require.NoError(t, err)

	require.NoError(t, f.Truncate(int64(2*GiB)))

	t.Cleanup(func() {
		require.NoError(t, f.Close())
	})

	var loDev losetup.Device

	loDev, err = losetup.Attach(rawImage, 0, false)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, loDev.Detach())
	})

	devPath := loDev.Path()

	devWhole, err := block.NewFromPath(devPath, block.OpenForWrite())
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, devWhole.Close())
	})

	magic := make([]byte, 1024)

	_, err = io.ReadFull(rand.Reader, magic)
	require.NoError(t, err)

	_, err = f.WriteAt(magic, 0)
	require.NoError(t, err)

	_, err = f.WriteAt(magic, 10*MiB)
	require.NoError(t, err)

	method, err := devWhole.Wipe()
	require.NoError(t, err)

	t.Logf("wipe method: %s", method)

	assertZeroed(t, f, 0, 1024)
	assertZeroed(t, f, 10*MiB, 1024)

	_, err = f.WriteAt(magic, 0)
	require.NoError(t, err)

	_, err = f.WriteAt(magic, 2*GiB-1024)
	require.NoError(t, err)

	require.NoError(t, devWhole.FastWipe())

	assertZeroed(t, f, 0, 1024)
	assertZeroed(t, f, 2*GiB-1024, 1024)
}

func assertZeroed(t *testing.T, f *os.File, offset, length int64) { //nolint:unparam
	t.Helper()

	buf := make([]byte, length)

	_, err := f.ReadAt(buf, offset)
	require.NoError(t, err)

	for i, b := range buf {
		if b != 0 {
			t.Fatalf("expected zero at offset %d, got %d", offset+int64(i), b)
		}
	}
}
