// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build linux

package blkid_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/freddierice/go-losetup/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/siderolabs/go-blockdevice/v2/blkid"
	"github.com/siderolabs/go-blockdevice/v2/block"
)

const MiB = 1024 * 1024

func xfsSetup(t *testing.T, path string) {
	t.Helper()

	cmd := exec.Command("mkfs.xfs", "-L", "somelabel", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())
}

func extfsSetup(t *testing.T, path string) {
	t.Helper()

	cmd := exec.Command("mkfs.ext4", "-L", "extlabel", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())
}

func vfatSetup(t *testing.T, path string) {
	t.Helper()

	cmd := exec.Command("mkfs.vfat", "-v", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())
}

func luksSetup(t *testing.T, path string) {
	t.Helper()

	cmd := exec.Command("cryptsetup", "luksFormat", "--label", "cryptlabel", "--key-file", "/dev/urandom", "--keyfile-size", "32", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())
}

func isoSetup(useJoilet bool) func(t *testing.T, path string) {
	return func(t *testing.T, path string) {
		t.Helper()

		require.NoError(t, os.Remove(path))

		contents := t.TempDir()

		f, err := os.Create(filepath.Join(contents, "fileA"))
		require.NoError(t, err)
		require.NoError(t, f.Close())

		f, err = os.Create(filepath.Join(contents, "fileB"))
		require.NoError(t, err)
		require.NoError(t, f.Close())

		require.NoError(t, os.Truncate(filepath.Join(contents, "fileA"), 1024*1024))
		require.NoError(t, os.Truncate(filepath.Join(contents, "fileB"), 1024))

		args := []string{"-o", path, "-V", "ISO label", "-input-charset", "utf-8"}
		if useJoilet {
			args = append(args, "-J", "-R")
		}

		args = append(args, contents)

		cmd := exec.Command("mkisofs", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		require.NoError(t, cmd.Run())
	}
}

//nolint:gocognit
func TestProbePath(t *testing.T) {
	for _, test := range []struct { //nolint:govet
		name string

		noLoop bool

		size  uint64
		setup func(*testing.T, string)

		expectedName  string
		expectedLabel string
		expectUUID    bool

		expectedBlockSize   []uint32
		expectedFSBlockSize []uint32
		expectedFSSize      uint64
	}{
		{
			name: "xfs",

			size:  500 * MiB,
			setup: xfsSetup,

			expectedName:  "xfs",
			expectedLabel: "somelabel",
			expectUUID:    true,

			expectedBlockSize:   []uint32{512},
			expectedFSBlockSize: []uint32{4096},
			expectedFSSize:      436 * MiB,
		},
		{
			name: "extfs",

			size:  500 * MiB,
			setup: extfsSetup,

			expectedName:  "extfs",
			expectedLabel: "extlabel",
			expectUUID:    true,

			expectedBlockSize:   []uint32{1024, 4096},
			expectedFSBlockSize: []uint32{1024, 4096},
			expectedFSSize:      500 * MiB,
		},
		{
			name: "vfat small",

			size:  100 * MiB,
			setup: vfatSetup,

			expectedName:        "vfat",
			expectedBlockSize:   []uint32{512},
			expectedFSBlockSize: []uint32{2048},
			expectedFSSize:      100 * MiB,
		},
		{
			name: "vfat big",

			size:  500 * MiB,
			setup: vfatSetup,

			expectedName:        "vfat",
			expectedBlockSize:   []uint32{512},
			expectedFSBlockSize: []uint32{8192},
			expectedFSSize:      524256768,
		},
		{
			name: "luks",

			size:  500 * MiB,
			setup: luksSetup,

			expectedName:  "luks",
			expectedLabel: "cryptlabel",
			expectUUID:    true,
		},
		{
			name:   "iso",
			noLoop: true,

			size:  0,
			setup: isoSetup(false),

			expectedName:  "iso9660",
			expectedLabel: "ISO label",

			expectedBlockSize:   []uint32{2048},
			expectedFSBlockSize: []uint32{2048},
			expectedFSSize:      0x157800,
		},
		{
			name:   "iso joilet",
			noLoop: true,

			size:  0,
			setup: isoSetup(true),

			expectedName:  "iso9660",
			expectedLabel: "ISO label",

			expectedBlockSize:   []uint32{2048},
			expectedFSBlockSize: []uint32{2048},
			expectedFSSize:      0x15b000,
		},
	} {
		for _, useLoopDevice := range []bool{false, true} {
			t.Run(fmt.Sprintf("loop=%v", useLoopDevice), func(t *testing.T) {
				t.Run(test.name, func(t *testing.T) {
					if useLoopDevice && os.Geteuid() != 0 {
						t.Skip("test requires root privileges")
					}

					if useLoopDevice && test.noLoop {
						t.Skip("test does not support loop devices")
					}

					tmpDir := t.TempDir()

					rawImage := filepath.Join(tmpDir, "image.raw")

					f, err := os.Create(rawImage)
					require.NoError(t, err)

					require.NoError(t, f.Truncate(int64(test.size)))
					require.NoError(t, f.Close())

					var probePath string

					if useLoopDevice {
						var loDev losetup.Device

						loDev, err = losetup.Attach(rawImage, 0, false)
						require.NoError(t, err)

						t.Cleanup(func() {
							assert.NoError(t, loDev.Detach())
						})

						probePath = loDev.Path()
					} else {
						probePath = rawImage
					}

					test.setup(t, probePath)

					info, err := blkid.ProbePath(probePath)
					require.NoError(t, err)

					if useLoopDevice {
						assert.NotNil(t, info.BlockDevice)
					} else {
						assert.Nil(t, info.BlockDevice)
					}

					assert.EqualValues(t, block.DefaultBlockSize, info.IOSize)

					if test.size != 0 {
						assert.EqualValues(t, test.size, info.Size)
					}

					assert.Equal(t, test.expectedName, info.Name)

					if test.expectedLabel != "" {
						require.NotNil(t, info.Label)
						assert.Equal(t, test.expectedLabel, *info.Label)
					} else {
						assert.Nil(t, info.Label)
					}

					if test.expectUUID {
						require.NotNil(t, info.UUID)
						t.Logf("UUID: %s", *info.UUID)
					} else {
						assert.Nil(t, info.UUID)
					}

					if test.expectedBlockSize != nil {
						assert.Contains(t, test.expectedBlockSize, info.BlockSize)
					}

					if test.expectedFSBlockSize != nil {
						assert.Contains(t, test.expectedFSBlockSize, info.FilesystemBlockSize)
					}

					assert.Equal(t, test.expectedFSSize, info.FilesystemSize)
				})
			})
		}
	}
}
