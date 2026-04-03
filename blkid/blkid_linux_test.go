// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build linux

package blkid_test

import (
	"crypto/rand"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	randv2 "math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/freddierice/go-losetup/v2"
	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"
	"github.com/siderolabs/gen/xslices"
	"github.com/siderolabs/go-pointer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"golang.org/x/sys/unix"

	"github.com/siderolabs/go-blockdevice/v2/blkid"
	"github.com/siderolabs/go-blockdevice/v2/block"
	"github.com/siderolabs/go-blockdevice/v2/partitioning/gpt"
)

const (
	MiB = 1024 * 1024
	GiB = 1024 * MiB
)

func xfsSetup(t *testing.T, path string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "mkfs.xfs", "--unsupported", "-L", "somelabel", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())
}

func ext2Setup(t *testing.T, path string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "mkfs.ext2", "-L", "extlabel", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())
}

func ext3Setup(t *testing.T, path string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "mkfs.ext3", "-L", "extlabel", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())
}

func ext4Setup(t *testing.T, path string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "mkfs.ext4", "-L", "extlabel", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())
}

func vfatSetup(bits int) func(t *testing.T, path string) {
	return func(t *testing.T, path string) {
		t.Helper()

		cmd := exec.CommandContext(t.Context(), "mkfs.vfat", "-F", strconv.Itoa(bits), "-n", "TALOS_V1", "-v", path)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		require.NoError(t, cmd.Run())
	}
}

func luksSetup(t *testing.T, path string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "cryptsetup", "luksFormat", "--label", "cryptlabel", "--key-file", "/dev/urandom", "--keyfile-size", "32", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())
}

func fixedImageSetup(source string) func(t *testing.T, path string) {
	return func(t *testing.T, path string) {
		t.Helper()

		in, err := os.Open(source)
		require.NoError(t, err)

		t.Cleanup(func() {
			require.NoError(t, in.Close())
		})

		out, err := os.OpenFile(path, os.O_RDWR, 0)
		require.NoError(t, err)

		zr, err := zstd.NewReader(in)
		require.NoError(t, err)

		_, err = io.Copy(out, zr)
		require.NoError(t, err)

		require.NoError(t, out.Close())
	}
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

		cmd := exec.CommandContext(t.Context(), "mkisofs", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		require.NoError(t, cmd.Run())
	}
}

func swapSetup(t *testing.T, path string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "mkswap", "--label", "swaplabel", "-p", "8192", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())
}

func swapSetup2(t *testing.T, path string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "mkswap", "--label", "swapswap", "-p", "4096", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())
}

func lvm2Setup(t *testing.T, path string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "pvcreate", "-v", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())
}

func squashfsSetup(t *testing.T, path string) {
	t.Helper()

	contents := t.TempDir()

	f, err := os.Create(filepath.Join(contents, "fileA"))
	require.NoError(t, err)

	_, err = io.Copy(f, io.LimitReader(rand.Reader, 1024*1024))
	require.NoError(t, err)

	require.NoError(t, f.Close())

	f, err = os.Create(filepath.Join(contents, "fileB"))
	require.NoError(t, err)

	_, err = io.Copy(f, io.LimitReader(rand.Reader, 1024))
	require.NoError(t, err)

	require.NoError(t, f.Close())

	cmd := exec.CommandContext(t.Context(), "mksquashfs", contents, path, "-all-root", "-noappend", "-no-progress", "-no-compression")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())
}

func talosmetaSetup(t *testing.T, path string) {
	t.Helper()

	f, err := os.OpenFile(path, os.O_RDWR, 0)
	require.NoError(t, err)

	metaSlice := make([]byte, 256*1024)
	binary.BigEndian.PutUint32(metaSlice, 0x5a4b3c2d)
	binary.BigEndian.PutUint32(metaSlice[len(metaSlice)-4:], 0xa5b4c3d2)

	_, err = f.Write(metaSlice)
	require.NoError(t, err)

	_, err = f.Write(metaSlice)
	require.NoError(t, err)

	require.NoError(t, f.Close())
}

//nolint:gocognit,maintidx
func TestProbePathFilesystems(t *testing.T) {
	for _, test := range []struct { //nolint:govet
		name string

		noLoop   bool
		loopOnly bool

		size  uint64
		setup func(*testing.T, string)

		expectedName       string
		expectedLabel      string
		expectedLabelRegex *regexp.Regexp
		expectUUID         bool

		expectedBlockSize   []uint32
		expectedFSBlockSize []uint32
		expectedFSSize      uint64
		expectedSignatures  []blkid.SignatureRange
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
			expectedSignatures: []blkid.SignatureRange{
				{Offset: 0, Size: 4},
			},
		},
		{
			name: "ext2",

			size:  500 * MiB,
			setup: ext2Setup,

			expectedName:  "ext2",
			expectedLabel: "extlabel",
			expectUUID:    true,

			expectedBlockSize:   []uint32{1024, 4096},
			expectedFSBlockSize: []uint32{1024, 4096},
			expectedFSSize:      500 * MiB,
			expectedSignatures: []blkid.SignatureRange{
				{Offset: 1080, Size: 2},
			},
		},
		{
			name: "ext3",

			size:  500 * MiB,
			setup: ext3Setup,

			expectedName:  "ext3",
			expectedLabel: "extlabel",
			expectUUID:    true,

			expectedBlockSize:   []uint32{1024, 4096},
			expectedFSBlockSize: []uint32{1024, 4096},
			expectedFSSize:      500 * MiB,
			expectedSignatures: []blkid.SignatureRange{
				{Offset: 1080, Size: 2},
			},
		},
		{
			name: "ext4",

			size:  500 * MiB,
			setup: ext4Setup,

			expectedName:  "ext4",
			expectedLabel: "extlabel",
			expectUUID:    true,

			expectedBlockSize:   []uint32{1024, 4096},
			expectedFSBlockSize: []uint32{1024, 4096},
			expectedFSSize:      500 * MiB,
			expectedSignatures: []blkid.SignatureRange{
				{Offset: 1080, Size: 2},
			},
		},
		{
			name: "vfat small",

			size:  100 * MiB,
			setup: vfatSetup(16),

			expectedName:        "vfat",
			expectedBlockSize:   []uint32{512},
			expectedFSBlockSize: []uint32{2048},
			expectedFSSize:      100 * MiB,
			expectedSignatures: []blkid.SignatureRange{
				{Offset: 54, Size: 8},
			},
			expectedLabel: "TALOS_V1",
		},
		{
			name: "vfat big",

			size:  500 * MiB,
			setup: vfatSetup(32),

			expectedName:        "vfat",
			expectedBlockSize:   []uint32{512},
			expectedFSBlockSize: []uint32{4096},
			expectedFSSize:      524256768,
			expectedSignatures: []blkid.SignatureRange{
				{Offset: 82, Size: 8},
			},
			expectedLabel: "TALOS_V1",
		},
		{
			name: "luks",

			size:  500 * MiB,
			setup: luksSetup,

			expectedName:  "luks",
			expectedLabel: "cryptlabel",
			expectUUID:    true,
			expectedSignatures: []blkid.SignatureRange{
				{Offset: 0, Size: 6},
			},
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
			expectedSignatures: []blkid.SignatureRange{
				{Offset: 32769, Size: 5},
			},
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
			expectedSignatures: []blkid.SignatureRange{
				{Offset: 32769, Size: 5},
			},
		},
		{
			name: "swap 8k",

			size:  500 * MiB,
			setup: swapSetup,

			expectedName:  "swap",
			expectedLabel: "swaplabel",
			expectUUID:    true,

			expectedBlockSize:   []uint32{8192},
			expectedFSBlockSize: []uint32{8192},
			expectedFSSize:      524279808,
			expectedSignatures: []blkid.SignatureRange{
				{Offset: 8182, Size: 10},
			},
		},
		{
			name: "swap 4k",

			size:  500 * MiB,
			setup: swapSetup2,

			expectedName:  "swap",
			expectedLabel: "swapswap",
			expectUUID:    true,

			expectedBlockSize:   []uint32{4096},
			expectedFSBlockSize: []uint32{4096},
			expectedFSSize:      524283904,
			expectedSignatures: []blkid.SignatureRange{
				{Offset: 4086, Size: 10},
			},
		},
		{
			name: "swap 200 MiB",

			size:  200 * MiB,
			setup: swapSetup,

			expectedName:  "swap",
			expectedLabel: "swaplabel",
			expectUUID:    true,

			expectedBlockSize:   []uint32{8192},
			expectedFSBlockSize: []uint32{8192},
			expectedFSSize:      209707008,
			expectedSignatures: []blkid.SignatureRange{
				{Offset: 8182, Size: 10},
			},
		},
		{
			name:     "lvm2-pv",
			loopOnly: true,

			size:  500 * MiB,
			setup: lvm2Setup,

			expectedName:       "lvm2-pv",
			expectedLabelRegex: regexp.MustCompile(`(?m)^[0-9a-zA-Z]{6}-[0-9a-zA-Z]{4}-[0-9a-zA-Z]{4}-[0-9a-zA-Z]{4}-[0-9a-zA-Z]{4}-[0-9a-zA-Z]{4}-[0-9a-zA-Z]{6}$`),
			expectedSignatures: []blkid.SignatureRange{
				{Offset: 536, Size: 8},
			},
		},
		{
			name:   "zfs",
			noLoop: true,

			size:  0,
			setup: fixedImageSetup("testdata/zfs.img.zst"),

			expectedName:       "zfs",
			expectedLabelRegex: regexp.MustCompile(`^[0-9a-f]{16}$`),
			expectedSignatures: zfsSignatures,
		},
		{
			name:   "iso fixed image",
			noLoop: true,

			size:  0,
			setup: fixedImageSetup("testdata/user-data-20000.iso.zst"),

			expectedName:  "iso9660",
			expectedLabel: "cidata",

			expectedBlockSize:   []uint32{2048},
			expectedFSBlockSize: []uint32{2048},
			expectedFSSize:      0xd000,
			expectedSignatures: []blkid.SignatureRange{
				{Offset: 32769, Size: 5},
			},
		},
		{
			name:   "iso fixed image with zero byte in label",
			noLoop: true,

			size:  0,
			setup: fixedImageSetup("testdata/user-data.iso.zst"),

			expectedName:  "iso9660",
			expectedLabel: "cidata",

			expectedBlockSize:   []uint32{2048},
			expectedFSBlockSize: []uint32{2048},
			expectedFSSize:      0x15800,
			expectedSignatures: []blkid.SignatureRange{
				{Offset: 32769, Size: 5},
			},
		},
		{
			name:   "squashfs",
			noLoop: true,

			size:  0,
			setup: squashfsSetup,

			expectedName: "squashfs",

			expectedBlockSize:   []uint32{0x20000},
			expectedFSBlockSize: []uint32{0x20000},
			expectedFSSize:      0x1005e8,
			expectedSignatures: []blkid.SignatureRange{
				{Offset: 0, Size: 4},
			},
		},
		{
			name: "talosmeta",

			size:  2 * 256 * 1024,
			setup: talosmetaSetup,

			expectedName: "talosmeta",

			expectedFSSize: 2 * 256 * 1024,
			expectedSignatures: []blkid.SignatureRange{
				{Offset: 0, Size: 4},
			},
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

					if !useLoopDevice && test.loopOnly {
						t.Skip("test does not support running without loop devices")
					}

					tmpDir := t.TempDir()

					rawImage := filepath.Join(tmpDir, "image.raw")

					f, err := os.Create(rawImage)
					require.NoError(t, err)

					require.NoError(t, f.Truncate(int64(test.size)))
					require.NoError(t, f.Close())

					var probePath string

					if useLoopDevice {
						loDev := losetupAttachHelper(t, rawImage, false)

						t.Cleanup(func() {
							assert.NoError(t, loDev.Detach())
						})

						probePath = loDev.Path()
					} else {
						probePath = rawImage
					}

					test.setup(t, probePath)

					logger := zaptest.NewLogger(t)

					info, err := blkid.ProbePath(probePath, blkid.WithProbeLogger(logger))
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

					switch {
					case test.expectedLabel != "":
						require.NotNil(t, info.Label)
						assert.Equal(t, test.expectedLabel, *info.Label)
					case test.expectedLabelRegex != nil:
						require.NotNil(t, info.Label)
						assert.True(t, test.expectedLabelRegex.MatchString(*info.Label))
					default:
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

					assert.Equal(t, test.expectedFSSize, info.ProbedSize)

					assert.Equal(t, test.expectedSignatures, info.SignatureRanges)

					// now try wiping if using loop
					if !useLoopDevice {
						return
					}

					fastWipeBySignatures(t, probePath, info)

					info, err = blkid.ProbePath(probePath, blkid.WithProbeLogger(logger))
					require.NoError(t, err)

					assert.Empty(t, info.Name)
				})
			})
		}
	}
}

func setupGPT(t *testing.T, path string) {
	t.Helper()

	script := strings.TrimSpace(`
label: gpt
label-id: DDDA0816-8B53-47BF-A813-9EBB1F73AAA2
size=      204800, type=C12A7328-F81F-11D2-BA4B-00A0C93EC93B, uuid=3C047FF8-E35C-4918-A061-B4C1E5A291E5, name="EFI"
size=        2048, type=21686148-6449-6E6F-744E-656564454649, uuid=942D2017-052E-4216-B4E4-2110507E4CD4, name="BIOS", attrs="LegacyBIOSBootable"
size=     2048000, type=0FC63DAF-8483-4772-8E79-3D69D8477DE4, uuid=E8516F6B-F03E-45AE-8D9D-9958456EE7E4, name="BOOT"
size=        2048, type=0FC63DAF-8483-4772-8E79-3D69D8477DE4, uuid=CE6B2D56-7A70-4546-926C-7A9B41607347, name="META"
size=      204800, type=0FC63DAF-8483-4772-8E79-3D69D8477DE4, uuid=7F5FCD6C-A703-40D2-8796-E5CF7F3A9EB5, name="STATE"
                   type=0FC63DAF-8483-4772-8E79-3D69D8477DE4, uuid=0F06E81A-E78D-426B-A078-30A01AAB3FB7, name="EPHEMERAL"
`)

	cmd := exec.CommandContext(t.Context(), "sfdisk", path)
	cmd.Stdin = strings.NewReader(script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())
}

func setupOverwrittenGPT(t *testing.T, path string) {
	t.Helper()

	// first, setup GPT covering whole disk
	setupGPT(t, path)

	// now, create a small GPT image
	tmpDir := t.TempDir()

	rawImage := filepath.Join(tmpDir, "image.raw")

	f, err := os.Create(rawImage)
	require.NoError(t, err)

	require.NoError(t, f.Truncate(512*MiB))
	require.NoError(t, f.Close())

	script := strings.TrimSpace(`
label: gpt
label-id: DDDA0816-8B53-47BF-A813-9EBB1F73AAA2
size=      204800, type=C12A7328-F81F-11D2-BA4B-00A0C93EC93B, uuid=3C047FF8-E35C-4918-A061-B4C1E5A291E5, name="TEST1"
					type=0FC63DAF-8483-4772-8E79-3D69D8477DE4, uuid=0F06E81A-E78D-426B-A078-30A01AAB3FB7, name="TEST2"
	`)

	cmd := exec.CommandContext(t.Context(), "sfdisk", rawImage)
	cmd.Stdin = strings.NewReader(script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())

	// now, copy over small image into the destination
	in, err := os.Open(rawImage)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, in.Close())
	})

	out, err := os.OpenFile(path, os.O_WRONLY, 0)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, out.Close())
	})

	_, err = io.Copy(out, in)
	require.NoError(t, err)
}

func corruptGPT(f func(*testing.T, string)) func(*testing.T, string) {
	return func(t *testing.T, path string) {
		t.Helper()

		f(t, path)

		buf := make([]byte, 512)

		f, err := os.OpenFile(path, os.O_RDWR, 0)
		require.NoError(t, err)

		_, err = f.ReadAt(buf, 512)
		require.NoError(t, err)

		buf[9] ^= 0xff // flip bits

		_, err = f.WriteAt(buf, 512)
		require.NoError(t, err)

		require.NoError(t, f.Close())
	}
}

func gptOverwritesFilesystem(f func(*testing.T, string)) func(*testing.T, string) {
	return func(t *testing.T, path string) {
		t.Helper()

		f(t, path)

		setupGPT(t, path)
	}
}

func wipe1MB(f func(*testing.T, string)) func(*testing.T, string) {
	return func(t *testing.T, path string) {
		t.Helper()

		f(t, path)

		f, err := os.OpenFile(path, os.O_RDWR, 0)
		require.NoError(t, err)

		_, err = f.Write(make([]byte, 1*MiB))
		require.NoError(t, err)

		require.NoError(t, f.Close())
	}
}

var expectedParts = []blkid.NestedProbeResult{
	{
		NestedResult: blkid.NestedResult{
			PartitionUUID:   pointer.To(uuid.MustParse("3C047FF8-E35C-4918-A061-B4C1E5A291E5")),
			PartitionType:   pointer.To(uuid.MustParse("C12A7328-F81F-11D2-BA4B-00A0C93EC93B")),
			PartitionLabel:  pointer.To("EFI"),
			PartitionIndex:  1,
			PartitionOffset: 1 * MiB,
			PartitionSize:   100 * MiB,
		},
	},
	{
		NestedResult: blkid.NestedResult{
			PartitionUUID:   pointer.To(uuid.MustParse("942D2017-052E-4216-B4E4-2110507E4CD4")),
			PartitionType:   pointer.To(uuid.MustParse("21686148-6449-6E6F-744E-656564454649")),
			PartitionLabel:  pointer.To("BIOS"),
			PartitionIndex:  2,
			PartitionOffset: 101 * MiB,
			PartitionSize:   1 * MiB,
		},
	},
	{
		NestedResult: blkid.NestedResult{
			PartitionUUID:   pointer.To(uuid.MustParse("E8516F6B-F03E-45AE-8D9D-9958456EE7E4")),
			PartitionType:   pointer.To(uuid.MustParse("0FC63DAF-8483-4772-8E79-3D69D8477DE4")),
			PartitionLabel:  pointer.To("BOOT"),
			PartitionIndex:  3,
			PartitionOffset: 102 * MiB,
			PartitionSize:   1000 * MiB,
		},
	},
	{
		NestedResult: blkid.NestedResult{
			PartitionUUID:   pointer.To(uuid.MustParse("CE6B2D56-7A70-4546-926C-7A9B41607347")),
			PartitionType:   pointer.To(uuid.MustParse("0FC63DAF-8483-4772-8E79-3D69D8477DE4")),
			PartitionLabel:  pointer.To("META"),
			PartitionIndex:  4,
			PartitionOffset: 1102 * MiB,
			PartitionSize:   1 * MiB,
		},
	},
	{
		NestedResult: blkid.NestedResult{
			PartitionUUID:   pointer.To(uuid.MustParse("7F5FCD6C-A703-40D2-8796-E5CF7F3A9EB5")),
			PartitionType:   pointer.To(uuid.MustParse("0FC63DAF-8483-4772-8E79-3D69D8477DE4")),
			PartitionLabel:  pointer.To("STATE"),
			PartitionIndex:  5,
			PartitionOffset: 1103 * MiB,
			PartitionSize:   100 * MiB,
		},
	},
	{
		NestedResult: blkid.NestedResult{
			PartitionUUID:   pointer.To(uuid.MustParse("0F06E81A-E78D-426B-A078-30A01AAB3FB7")),
			PartitionType:   pointer.To(uuid.MustParse("0FC63DAF-8483-4772-8E79-3D69D8477DE4")),
			PartitionLabel:  pointer.To("EPHEMERAL"),
			PartitionIndex:  6,
			PartitionOffset: 1203 * MiB,
			PartitionSize:   844 * MiB,
		},
	},
}

//nolint:gocognit
func TestProbePathGPT(t *testing.T) {
	for _, test := range []struct { //nolint:govet
		name string

		size  uint64
		setup func(*testing.T, string)

		expectedSize       uint64
		expectedUUID       uuid.UUID
		expectedParts      []blkid.NestedProbeResult
		expectedSignatures []blkid.SignatureRange
	}{
		{
			name: "good GPT",

			size:  2 * GiB,
			setup: setupGPT,

			expectedSize:  2 * GiB,
			expectedUUID:  uuid.MustParse("DDDA0816-8B53-47BF-A813-9EBB1F73AAA2"),
			expectedParts: expectedParts,
			expectedSignatures: []blkid.SignatureRange{
				{
					Offset: 0,
					Size:   1024,
				},
				{
					Offset: 2147483136,
					Size:   512,
				},
			},
		},
		{
			name: "GPT overwrites ZFS",

			size:  2 * GiB,
			setup: gptOverwritesFilesystem(fixedImageSetup("testdata/zfs.img.zst")),

			expectedSize:  2 * GiB,
			expectedUUID:  uuid.MustParse("DDDA0816-8B53-47BF-A813-9EBB1F73AAA2"),
			expectedParts: expectedParts,
			expectedSignatures: []blkid.SignatureRange{
				{
					Offset: 0,
					Size:   1024,
				},
				{
					Offset: 2147483136,
					Size:   512,
				},
			},
		},
		{
			name: "corrupted GPT",

			size:  2 * GiB,
			setup: corruptGPT(setupGPT),

			expectedSize:  2 * GiB,
			expectedUUID:  uuid.MustParse("DDDA0816-8B53-47BF-A813-9EBB1F73AAA2"),
			expectedParts: expectedParts,
			expectedSignatures: []blkid.SignatureRange{
				{
					Offset: 0,
					Size:   1024,
				},
				{
					Offset: 2147483136,
					Size:   512,
				},
			},
		},
		{
			name: "overwritten GPT",

			size:  2 * GiB,
			setup: setupOverwrittenGPT,

			expectedSize: 512 * MiB,
			expectedUUID: uuid.MustParse("DDDA0816-8B53-47BF-A813-9EBB1F73AAA2"),
			expectedParts: []blkid.NestedProbeResult{
				{
					NestedResult: blkid.NestedResult{
						PartitionUUID:   pointer.To(uuid.MustParse("3C047FF8-E35C-4918-A061-B4C1E5A291E5")),
						PartitionType:   pointer.To(uuid.MustParse("C12A7328-F81F-11D2-BA4B-00A0C93EC93B")),
						PartitionLabel:  pointer.To("TEST1"),
						PartitionIndex:  1,
						PartitionOffset: 1 * MiB,
						PartitionSize:   100 * MiB,
					},
				},
				{
					NestedResult: blkid.NestedResult{
						PartitionUUID:   pointer.To(uuid.MustParse("0F06E81A-E78D-426B-A078-30A01AAB3FB7")),
						PartitionType:   pointer.To(uuid.MustParse("0FC63DAF-8483-4772-8E79-3D69D8477DE4")),
						PartitionLabel:  pointer.To("TEST2"),
						PartitionIndex:  2,
						PartitionOffset: 101 * MiB,
						PartitionSize:   410 * MiB,
					},
				},
			},
			expectedSignatures: []blkid.SignatureRange{
				{
					Offset: 0,
					Size:   1024,
				},
				{
					Offset: 536870400,
					Size:   512,
				},
			},
		},
	} {
		for _, useLoopDevice := range []bool{false, true} {
			t.Run(fmt.Sprintf("loop=%v", useLoopDevice), func(t *testing.T) {
				t.Run(test.name, func(t *testing.T) {
					if useLoopDevice && os.Geteuid() != 0 {
						t.Skip("test requires root privileges")
					}

					tmpDir := t.TempDir()

					rawImage := filepath.Join(tmpDir, "image.raw")

					f, err := os.Create(rawImage)
					require.NoError(t, err)

					require.NoError(t, f.Truncate(int64(test.size)))
					require.NoError(t, f.Close())

					var probePath string

					if useLoopDevice {
						loDev := losetupAttachHelper(t, rawImage, false)

						t.Cleanup(func() {
							assert.NoError(t, loDev.Detach())
						})

						probePath = loDev.Path()
					} else {
						probePath = rawImage
					}

					test.setup(t, probePath)

					logger := zaptest.NewLogger(t)

					info, err := blkid.ProbePath(probePath, blkid.WithProbeLogger(logger))
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

					assert.Equal(t, "gpt", info.Name)
					assert.EqualValues(t, block.DefaultBlockSize, info.BlockSize)
					assert.Equal(t, test.expectedSize-1*MiB-33*block.DefaultBlockSize, info.ProbedSize)

					require.NotNil(t, info.UUID)
					assert.Equal(t, test.expectedUUID, *info.UUID)

					assert.Equal(t, test.expectedSignatures, info.SignatureRanges)

					assert.Equal(t, test.expectedParts, info.Parts)

					// now try wiping if using loop
					if !useLoopDevice {
						return
					}

					fastWipeBySignatures(t, probePath, info)

					info, err = blkid.ProbePath(probePath, blkid.WithProbeLogger(logger))
					require.NoError(t, err)

					assert.Empty(t, info.Name)
					assert.Empty(t, info.Parts)
				})
			})
		}
	}
}

func TestProbeHalfWipedGPT(t *testing.T) {
	// wiping first 1MB (first header) should not detect GPT
	for _, useLoopDevice := range []bool{false, true} {
		t.Run(fmt.Sprintf("loop=%v", useLoopDevice), func(t *testing.T) {
			if useLoopDevice && os.Geteuid() != 0 {
				t.Skip("test requires root privileges")
			}

			tmpDir := t.TempDir()

			rawImage := filepath.Join(tmpDir, "image.raw")

			f, err := os.Create(rawImage)
			require.NoError(t, err)

			require.NoError(t, f.Truncate(int64(2*GiB)))
			require.NoError(t, f.Close())

			var probePath string

			if useLoopDevice {
				loDev := losetupAttachHelper(t, rawImage, false)

				t.Cleanup(func() {
					assert.NoError(t, loDev.Detach())
				})

				probePath = loDev.Path()
			} else {
				probePath = rawImage
			}

			wipe1MB(setupGPT)(t, probePath)

			logger := zaptest.NewLogger(t)

			info, err := blkid.ProbePath(probePath, blkid.WithProbeLogger(logger))
			require.NoError(t, err)

			assert.Empty(t, info.Name)
		})
	}
}

func setupNestedGPT(t *testing.T, path string) {
	t.Helper()

	setupGPT(t, path)

	require.NoError(t, exec.CommandContext(t.Context(), "partprobe", path).Run())

	vfatSetup(16)(t, path+"p1")
	ext4Setup(t, path+"p3")
	xfsSetup(t, path+"p6")
}

func TestProbePathNested(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test requires root privileges")
	}

	skipUnderBuildkit(t)

	for _, test := range []struct { //nolint:govet
		name string

		size  uint64
		setup func(*testing.T, string)

		expectedUUID       uuid.UUID
		expectedParts      []blkid.NestedProbeResult
		expectedSignatures []blkid.SignatureRange
	}{
		{
			name: "good GPT, ext4fs, xfs, vfat, none",

			size:  2 * GiB,
			setup: setupNestedGPT,

			expectedUUID:  uuid.MustParse("DDDA0816-8B53-47BF-A813-9EBB1F73AAA2"),
			expectedParts: expectedParts,
			expectedSignatures: []blkid.SignatureRange{
				{
					Offset: 0,
					Size:   1024,
				},
				{
					Offset: 2147483136,
					Size:   512,
				},
				{
					Offset: 1048630,
					Size:   8,
				},
				{
					Offset: 106955832,
					Size:   2,
				},
				{
					Offset: 1261436928,
					Size:   4,
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			rawImage := filepath.Join(tmpDir, "image.raw")

			f, err := os.Create(rawImage)
			require.NoError(t, err)

			require.NoError(t, f.Truncate(int64(test.size)))
			require.NoError(t, f.Close())

			loDev := losetupAttachHelper(t, rawImage, false)

			t.Cleanup(func() {
				assert.NoError(t, loDev.Detach())
			})

			probePath := loDev.Path()

			test.setup(t, probePath)

			logger := zaptest.NewLogger(t)

			info, err := blkid.ProbePath(probePath, blkid.WithProbeLogger(logger))
			require.NoError(t, err)

			assert.NotNil(t, info.BlockDevice)

			assert.EqualValues(t, block.DefaultBlockSize, info.IOSize)

			if test.size != 0 {
				assert.EqualValues(t, test.size, info.Size)
			}

			assert.Equal(t, "gpt", info.Name)
			assert.EqualValues(t, block.DefaultBlockSize, info.BlockSize)
			assert.Equal(t, test.size-1*MiB-33*block.DefaultBlockSize, info.ProbedSize)

			require.NotNil(t, info.UUID)
			assert.Equal(t, test.expectedUUID, *info.UUID)

			assert.Equal(t, test.expectedSignatures, info.SignatureRanges)

			// extract only partition information and compare it separately
			partitionsOnly := xslices.Map(info.Parts, func(p blkid.NestedProbeResult) blkid.NestedProbeResult {
				return blkid.NestedProbeResult{
					NestedResult: p.NestedResult,
				}
			})

			assert.Equal(t, test.expectedParts, partitionsOnly)

			// EFI: vfat
			assert.Equal(t, "vfat", info.Parts[0].Name)
			assert.EqualValues(t, 512, info.Parts[0].BlockSize)
			assert.EqualValues(t, 2048, info.Parts[0].FilesystemBlockSize)
			assert.EqualValues(t, 0x63f9c00, info.Parts[0].ProbedSize)

			// empty
			assert.Equal(t, blkid.ProbeResult{}, info.Parts[1].ProbeResult)

			// BOOT: ext4
			assert.Equal(t, "ext4", info.Parts[2].Name)
			assert.Contains(t, []uint32{1024, 4096}, info.Parts[2].BlockSize)
			assert.Contains(t, []uint32{1024, 4096}, info.Parts[2].FilesystemBlockSize)
			assert.EqualValues(t, 1000*MiB, info.Parts[2].ProbedSize)

			// empty
			assert.Equal(t, blkid.ProbeResult{}, info.Parts[3].ProbeResult)
			assert.Equal(t, blkid.ProbeResult{}, info.Parts[4].ProbeResult)

			// EPHEMERAL: xfs
			assert.Equal(t, "xfs", info.Parts[5].Name)
			assert.EqualValues(t, 512, info.Parts[5].BlockSize)
			assert.EqualValues(t, 4096, info.Parts[5].FilesystemBlockSize)
			assert.EqualValues(t, 0x30c00000, info.Parts[5].ProbedSize)
		})
	}
}

func setupOurGPT(t *testing.T, path string, createFilesystems bool) {
	t.Helper()

	blk, err := block.NewFromPath(path, block.OpenForWrite())
	require.NoError(t, err)

	require.NoError(t, blk.Lock(true))
	require.NoError(t, blk.FastWipe())

	gptdev, err := gpt.DeviceFromBlockDevice(blk)
	require.NoError(t, err)

	// 	label-id: DDDA0816-8B53-47BF-A813-9EBB1F73AAA2
	// size=      204800, type=C12A7328-F81F-11D2-BA4B-00A0C93EC93B, uuid=3C047FF8-E35C-4918-A061-B4C1E5A291E5, name="EFI"
	// size=        2048, type=21686148-6449-6E6F-744E-656564454649, uuid=942D2017-052E-4216-B4E4-2110507E4CD4, name="BIOS", attrs="LegacyBIOSBootable"
	// size=     2048000, type=0FC63DAF-8483-4772-8E79-3D69D8477DE4, uuid=E8516F6B-F03E-45AE-8D9D-9958456EE7E4, name="BOOT"
	// size=        2048, type=0FC63DAF-8483-4772-8E79-3D69D8477DE4, uuid=CE6B2D56-7A70-4546-926C-7A9B41607347, name="META"
	// size=      204800, type=0FC63DAF-8483-4772-8E79-3D69D8477DE4, uuid=7F5FCD6C-A703-40D2-8796-E5CF7F3A9EB5, name="STATE"
	//                    type=0FC63DAF-8483-4772-8E79-3D69D8477DE4, uuid=0F06E81A-E78D-426B-A078-30A01AAB3FB7, name="EPHEMERAL"

	part, err := gpt.New(gptdev, gpt.WithDiskGUID(uuid.MustParse("DDDA0816-8B53-47BF-A813-9EBB1F73AAA2")))
	require.NoError(t, err)

	_, _, err = part.AllocatePartition(204800*512, "EFI", uuid.MustParse("C12A7328-F81F-11D2-BA4B-00A0C93EC93B"), gpt.WithUniqueGUID(uuid.MustParse("3C047FF8-E35C-4918-A061-B4C1E5A291E5")))
	require.NoError(t, err)

	_, _, err = part.AllocatePartition(2048*512, "BIOS", uuid.MustParse("21686148-6449-6E6F-744E-656564454649"), gpt.WithUniqueGUID(uuid.MustParse("942D2017-052E-4216-B4E4-2110507E4CD4")))
	require.NoError(t, err)

	_, _, err = part.AllocatePartition(2048000*512, "BOOT", uuid.MustParse("0FC63DAF-8483-4772-8E79-3D69D8477DE4"), gpt.WithUniqueGUID(uuid.MustParse("E8516F6B-F03E-45AE-8D9D-9958456EE7E4")))
	require.NoError(t, err)

	_, _, err = part.AllocatePartition(2048*512, "META", uuid.MustParse("0FC63DAF-8483-4772-8E79-3D69D8477DE4"), gpt.WithUniqueGUID(uuid.MustParse("CE6B2D56-7A70-4546-926C-7A9B41607347")))
	require.NoError(t, err)

	_, _, err = part.AllocatePartition(204800*512, "STATE", uuid.MustParse("0FC63DAF-8483-4772-8E79-3D69D8477DE4"), gpt.WithUniqueGUID(uuid.MustParse("7F5FCD6C-A703-40D2-8796-E5CF7F3A9EB5")))
	require.NoError(t, err)

	_, _, err = part.AllocatePartition(part.LargestContiguousAllocatable(), "EPHEMERAL", uuid.MustParse("0FC63DAF-8483-4772-8E79-3D69D8477DE4"),
		gpt.WithUniqueGUID(uuid.MustParse("0F06E81A-E78D-426B-A078-30A01AAB3FB7")))
	require.NoError(t, err)

	require.NoError(t, part.Write())

	if createFilesystems {
		vfatSetup(16)(t, path+"p1")
		xfsSetup(t, path+"p3")
		xfsSetup(t, path+"p5")
		xfsSetup(t, path+"p6")
	}

	require.NoError(t, blk.Unlock())
	require.NoError(t, blk.Close())
}

func TestProbePathOurGPT(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test requires root privileges")
	}

	skipUnderBuildkit(t)

	for _, test := range []struct { //nolint:govet
		name string

		size  uint64
		setup func(*testing.T, string, bool)

		expectedUUID       uuid.UUID
		expectedParts      []blkid.NestedProbeResult
		expectedSignatures []blkid.SignatureRange
	}{
		{
			name: "good GPT, ext4fs, xfs, vfat, none",

			size:  2 * GiB,
			setup: setupOurGPT,

			expectedUUID:  uuid.MustParse("DDDA0816-8B53-47BF-A813-9EBB1F73AAA2"),
			expectedParts: expectedParts,
			expectedSignatures: []blkid.SignatureRange{
				{
					Offset: 512,
					Size:   512,
				},
				{
					Offset: 2147483136,
					Size:   512,
				},
				{
					Offset: 1048630,
					Size:   8,
				},
				{
					Offset: 106955832,
					Size:   2,
				},
				{
					Offset: 1261436928,
					Size:   4,
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			rawImage := filepath.Join(tmpDir, "image.raw")

			f, err := os.Create(rawImage)
			require.NoError(t, err)

			require.NoError(t, f.Truncate(int64(test.size)))
			require.NoError(t, f.Close())

			loDev := losetupAttachHelper(t, rawImage, false)

			t.Cleanup(func() {
				assert.NoError(t, loDev.Detach())
			})

			probePath := loDev.Path()

			test.setup(t, probePath, true)

			logger := zaptest.NewLogger(t)

			info, err := blkid.ProbePath(probePath, blkid.WithProbeLogger(logger))
			require.NoError(t, err)

			assert.NotNil(t, info.BlockDevice)

			assert.EqualValues(t, block.DefaultBlockSize, info.IOSize)

			if test.size != 0 {
				assert.EqualValues(t, test.size, info.Size)
			}

			assert.Equal(t, "gpt", info.Name)
			assert.EqualValues(t, block.DefaultBlockSize, info.BlockSize)
			assert.Equal(t, test.size-1*MiB-33*block.DefaultBlockSize, info.ProbedSize)

			require.NotNil(t, info.UUID)
			assert.Equal(t, test.expectedUUID, *info.UUID)

			// extract only partition information and compare it separately
			partitionsOnly := xslices.Map(info.Parts, func(p blkid.NestedProbeResult) blkid.NestedProbeResult {
				return blkid.NestedProbeResult{
					NestedResult: p.NestedResult,
				}
			})

			assert.Equal(t, test.expectedParts, partitionsOnly)

			// EFI: vfat
			assert.Equal(t, "vfat", info.Parts[0].Name)
			assert.EqualValues(t, 512, info.Parts[0].BlockSize)
			assert.EqualValues(t, 2048, info.Parts[0].FilesystemBlockSize)
			assert.EqualValues(t, 0x63f9c00, info.Parts[0].ProbedSize)

			// empty
			assert.Equal(t, blkid.ProbeResult{}, info.Parts[1].ProbeResult)

			// BOOT: xfs
			assert.Equal(t, "xfs", info.Parts[2].Name)
			assert.EqualValues(t, 512, info.Parts[2].BlockSize)
			assert.EqualValues(t, 4096, info.Parts[2].FilesystemBlockSize)
			assert.EqualValues(t, 0x3a800000, info.Parts[2].ProbedSize)

			// empty META
			assert.Equal(t, blkid.ProbeResult{}, info.Parts[3].ProbeResult)

			// STATE: xfs
			assert.Equal(t, "xfs", info.Parts[4].Name)
			assert.EqualValues(t, 512, info.Parts[4].BlockSize)
			assert.EqualValues(t, 4096, info.Parts[4].FilesystemBlockSize)
			assert.EqualValues(t, 0x57fd000, info.Parts[4].ProbedSize)

			// EPHEMERAL: xfs
			assert.Equal(t, "xfs", info.Parts[5].Name)
			assert.EqualValues(t, 512, info.Parts[5].BlockSize)
			assert.EqualValues(t, 4096, info.Parts[5].FilesystemBlockSize)
			assert.EqualValues(t, 0x30c00000, info.Parts[5].ProbedSize)
		})
	}
}

func fastWipeBySignatures(t *testing.T, path string, info *blkid.Info) {
	t.Helper()

	blk, err := block.NewFromPath(path, block.OpenForWrite())
	require.NoError(t, err)

	require.NoError(t, blk.Lock(true))

	require.NoError(t, blk.FastWipe(xslices.Map(info.SignatureRanges,
		func(r blkid.SignatureRange) block.Range {
			return block.Range(r)
		},
	)...))

	require.NoError(t, blk.Unlock())
	require.NoError(t, blk.Close())
}

func TestProbeWithWipeRanges(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test requires root privileges")
	}

	skipUnderBuildkit(t)

	tmpDir := t.TempDir()

	rawImage := filepath.Join(tmpDir, "image.raw")

	f, err := os.Create(rawImage)
	require.NoError(t, err)

	require.NoError(t, f.Truncate(int64(2*GiB)))
	require.NoError(t, f.Close())

	loDev := losetupAttachHelper(t, rawImage, false)

	t.Cleanup(func() {
		assert.NoError(t, loDev.Detach())
	})

	probePath := loDev.Path()

	// setup initial GPT and filesystems
	setupOurGPT(t, probePath, true)

	logger := zaptest.NewLogger(t)

	info, err := blkid.ProbePath(probePath, blkid.WithProbeLogger(logger))
	require.NoError(t, err)

	assert.Equal(t, "gpt", info.Name)
	assert.Len(t, info.Parts, 6)

	for _, idx := range []int{0, 2, 4, 5} {
		assert.NotEmpty(t, info.Parts[idx].Name)
	}

	// wipe by probed ranges
	fastWipeBySignatures(t, probePath, info)

	// probe again
	info, err = blkid.ProbePath(probePath, blkid.WithProbeLogger(logger))
	require.NoError(t, err)

	assert.Empty(t, info.Name)
	assert.Empty(t, info.Parts)

	// re-create partitions but without filesystems
	setupOurGPT(t, probePath, false)

	info, err = blkid.ProbePath(probePath, blkid.WithProbeLogger(logger))
	require.NoError(t, err)

	assert.Equal(t, "gpt", info.Name)
	assert.Len(t, info.Parts, 6)

	// filesystem should not be detected
	for _, idx := range []int{0, 2, 4, 5} {
		assert.Empty(t, info.Parts[idx].Name)
	}
}

func losetupAttachHelper(t *testing.T, rawImage string, readonly bool) losetup.Device { //nolint:unparam
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

func skipUnderBuildkit(t *testing.T) {
	t.Helper()

	if hostname, _ := os.Hostname(); hostname == "buildkitsandbox" { //nolint: errcheck
		t.Skip("test not supported under buildkit as partition devices are not propagated from /dev")
	}
}

var zfsSignatures = []blkid.SignatureRange{
	{Offset: 131072, Size: 8},
	{Offset: 132096, Size: 8},
	{Offset: 133120, Size: 8},
	{Offset: 134144, Size: 8},
	{Offset: 135168, Size: 8},
	{Offset: 136192, Size: 8},
	{Offset: 137216, Size: 8},
	{Offset: 138240, Size: 8},
	{Offset: 139264, Size: 8},
	{Offset: 140288, Size: 8},
	{Offset: 141312, Size: 8},
	{Offset: 142336, Size: 8},
	{Offset: 143360, Size: 8},
	{Offset: 144384, Size: 8},
	{Offset: 145408, Size: 8},
	{Offset: 146432, Size: 8},
	{Offset: 147456, Size: 8},
	{Offset: 148480, Size: 8},
	{Offset: 149504, Size: 8},
	{Offset: 150528, Size: 8},
	{Offset: 151552, Size: 8},
	{Offset: 152576, Size: 8},
	{Offset: 153600, Size: 8},
	{Offset: 154624, Size: 8},
	{Offset: 155648, Size: 8},
	{Offset: 156672, Size: 8},
	{Offset: 157696, Size: 8},
	{Offset: 158720, Size: 8},
	{Offset: 159744, Size: 8},
	{Offset: 160768, Size: 8},
	{Offset: 161792, Size: 8},
	{Offset: 162816, Size: 8},
	{Offset: 163840, Size: 8},
	{Offset: 164864, Size: 8},
	{Offset: 165888, Size: 8},
	{Offset: 166912, Size: 8},
	{Offset: 167936, Size: 8},
	{Offset: 168960, Size: 8},
	{Offset: 169984, Size: 8},
	{Offset: 171008, Size: 8},
	{Offset: 172032, Size: 8},
	{Offset: 173056, Size: 8},
	{Offset: 174080, Size: 8},
	{Offset: 175104, Size: 8},
	{Offset: 176128, Size: 8},
	{Offset: 177152, Size: 8},
	{Offset: 178176, Size: 8},
	{Offset: 179200, Size: 8},
	{Offset: 180224, Size: 8},
	{Offset: 181248, Size: 8},
	{Offset: 182272, Size: 8},
	{Offset: 183296, Size: 8},
	{Offset: 184320, Size: 8},
	{Offset: 185344, Size: 8},
	{Offset: 186368, Size: 8},
	{Offset: 187392, Size: 8},
	{Offset: 188416, Size: 8},
	{Offset: 189440, Size: 8},
	{Offset: 190464, Size: 8},
	{Offset: 191488, Size: 8},
	{Offset: 192512, Size: 8},
	{Offset: 193536, Size: 8},
	{Offset: 194560, Size: 8},
	{Offset: 195584, Size: 8},
	{Offset: 196608, Size: 8},
	{Offset: 197632, Size: 8},
	{Offset: 198656, Size: 8},
	{Offset: 199680, Size: 8},
	{Offset: 200704, Size: 8},
	{Offset: 201728, Size: 8},
	{Offset: 202752, Size: 8},
	{Offset: 203776, Size: 8},
	{Offset: 204800, Size: 8},
	{Offset: 205824, Size: 8},
	{Offset: 206848, Size: 8},
	{Offset: 207872, Size: 8},
	{Offset: 208896, Size: 8},
	{Offset: 209920, Size: 8},
	{Offset: 210944, Size: 8},
	{Offset: 211968, Size: 8},
	{Offset: 212992, Size: 8},
	{Offset: 214016, Size: 8},
	{Offset: 215040, Size: 8},
	{Offset: 216064, Size: 8},
	{Offset: 217088, Size: 8},
	{Offset: 218112, Size: 8},
	{Offset: 219136, Size: 8},
	{Offset: 220160, Size: 8},
	{Offset: 221184, Size: 8},
	{Offset: 222208, Size: 8},
	{Offset: 223232, Size: 8},
	{Offset: 224256, Size: 8},
	{Offset: 225280, Size: 8},
	{Offset: 226304, Size: 8},
	{Offset: 227328, Size: 8},
	{Offset: 228352, Size: 8},
	{Offset: 229376, Size: 8},
	{Offset: 230400, Size: 8},
	{Offset: 231424, Size: 8},
	{Offset: 232448, Size: 8},
	{Offset: 233472, Size: 8},
	{Offset: 234496, Size: 8},
	{Offset: 235520, Size: 8},
	{Offset: 236544, Size: 8},
	{Offset: 237568, Size: 8},
	{Offset: 238592, Size: 8},
	{Offset: 239616, Size: 8},
	{Offset: 240640, Size: 8},
	{Offset: 241664, Size: 8},
	{Offset: 242688, Size: 8},
	{Offset: 243712, Size: 8},
	{Offset: 244736, Size: 8},
	{Offset: 245760, Size: 8},
	{Offset: 246784, Size: 8},
	{Offset: 247808, Size: 8},
	{Offset: 248832, Size: 8},
	{Offset: 249856, Size: 8},
	{Offset: 250880, Size: 8},
	{Offset: 251904, Size: 8},
	{Offset: 252928, Size: 8},
	{Offset: 253952, Size: 8},
	{Offset: 254976, Size: 8},
	{Offset: 256000, Size: 8},
	{Offset: 257024, Size: 8},
	{Offset: 258048, Size: 8},
	{Offset: 259072, Size: 8},
	{Offset: 260096, Size: 8},
	{Offset: 261120, Size: 8},
	{Offset: 393216, Size: 8},
	{Offset: 394240, Size: 8},
	{Offset: 395264, Size: 8},
	{Offset: 396288, Size: 8},
	{Offset: 397312, Size: 8},
	{Offset: 398336, Size: 8},
	{Offset: 399360, Size: 8},
	{Offset: 400384, Size: 8},
	{Offset: 401408, Size: 8},
	{Offset: 402432, Size: 8},
	{Offset: 403456, Size: 8},
	{Offset: 404480, Size: 8},
	{Offset: 405504, Size: 8},
	{Offset: 406528, Size: 8},
	{Offset: 407552, Size: 8},
	{Offset: 408576, Size: 8},
	{Offset: 409600, Size: 8},
	{Offset: 410624, Size: 8},
	{Offset: 411648, Size: 8},
	{Offset: 412672, Size: 8},
	{Offset: 413696, Size: 8},
	{Offset: 414720, Size: 8},
	{Offset: 415744, Size: 8},
	{Offset: 416768, Size: 8},
	{Offset: 417792, Size: 8},
	{Offset: 418816, Size: 8},
	{Offset: 419840, Size: 8},
	{Offset: 420864, Size: 8},
	{Offset: 421888, Size: 8},
	{Offset: 422912, Size: 8},
	{Offset: 423936, Size: 8},
	{Offset: 424960, Size: 8},
	{Offset: 425984, Size: 8},
	{Offset: 427008, Size: 8},
	{Offset: 428032, Size: 8},
	{Offset: 429056, Size: 8},
	{Offset: 430080, Size: 8},
	{Offset: 431104, Size: 8},
	{Offset: 432128, Size: 8},
	{Offset: 433152, Size: 8},
	{Offset: 434176, Size: 8},
	{Offset: 435200, Size: 8},
	{Offset: 436224, Size: 8},
	{Offset: 437248, Size: 8},
	{Offset: 438272, Size: 8},
	{Offset: 439296, Size: 8},
	{Offset: 440320, Size: 8},
	{Offset: 441344, Size: 8},
	{Offset: 442368, Size: 8},
	{Offset: 443392, Size: 8},
	{Offset: 444416, Size: 8},
	{Offset: 445440, Size: 8},
	{Offset: 446464, Size: 8},
	{Offset: 447488, Size: 8},
	{Offset: 448512, Size: 8},
	{Offset: 449536, Size: 8},
	{Offset: 450560, Size: 8},
	{Offset: 451584, Size: 8},
	{Offset: 452608, Size: 8},
	{Offset: 453632, Size: 8},
	{Offset: 454656, Size: 8},
	{Offset: 455680, Size: 8},
	{Offset: 456704, Size: 8},
	{Offset: 457728, Size: 8},
	{Offset: 458752, Size: 8},
	{Offset: 459776, Size: 8},
	{Offset: 460800, Size: 8},
	{Offset: 461824, Size: 8},
	{Offset: 462848, Size: 8},
	{Offset: 463872, Size: 8},
	{Offset: 464896, Size: 8},
	{Offset: 465920, Size: 8},
	{Offset: 466944, Size: 8},
	{Offset: 467968, Size: 8},
	{Offset: 468992, Size: 8},
	{Offset: 470016, Size: 8},
	{Offset: 471040, Size: 8},
	{Offset: 472064, Size: 8},
	{Offset: 473088, Size: 8},
	{Offset: 474112, Size: 8},
	{Offset: 475136, Size: 8},
	{Offset: 476160, Size: 8},
	{Offset: 477184, Size: 8},
	{Offset: 478208, Size: 8},
	{Offset: 479232, Size: 8},
	{Offset: 480256, Size: 8},
	{Offset: 481280, Size: 8},
	{Offset: 482304, Size: 8},
	{Offset: 483328, Size: 8},
	{Offset: 484352, Size: 8},
	{Offset: 485376, Size: 8},
	{Offset: 486400, Size: 8},
	{Offset: 487424, Size: 8},
	{Offset: 488448, Size: 8},
	{Offset: 489472, Size: 8},
	{Offset: 490496, Size: 8},
	{Offset: 491520, Size: 8},
	{Offset: 492544, Size: 8},
	{Offset: 493568, Size: 8},
	{Offset: 494592, Size: 8},
	{Offset: 495616, Size: 8},
	{Offset: 496640, Size: 8},
	{Offset: 497664, Size: 8},
	{Offset: 498688, Size: 8},
	{Offset: 499712, Size: 8},
	{Offset: 500736, Size: 8},
	{Offset: 501760, Size: 8},
	{Offset: 502784, Size: 8},
	{Offset: 503808, Size: 8},
	{Offset: 504832, Size: 8},
	{Offset: 505856, Size: 8},
	{Offset: 506880, Size: 8},
	{Offset: 507904, Size: 8},
	{Offset: 508928, Size: 8},
	{Offset: 509952, Size: 8},
	{Offset: 510976, Size: 8},
	{Offset: 512000, Size: 8},
	{Offset: 513024, Size: 8},
	{Offset: 514048, Size: 8},
	{Offset: 515072, Size: 8},
	{Offset: 516096, Size: 8},
	{Offset: 517120, Size: 8},
	{Offset: 518144, Size: 8},
	{Offset: 519168, Size: 8},
	{Offset: 520192, Size: 8},
	{Offset: 521216, Size: 8},
	{Offset: 522240, Size: 8},
	{Offset: 523264, Size: 8},
	{Offset: 66715648, Size: 8},
	{Offset: 66716672, Size: 8},
	{Offset: 66717696, Size: 8},
	{Offset: 66718720, Size: 8},
	{Offset: 66719744, Size: 8},
	{Offset: 66720768, Size: 8},
	{Offset: 66721792, Size: 8},
	{Offset: 66722816, Size: 8},
	{Offset: 66723840, Size: 8},
	{Offset: 66724864, Size: 8},
	{Offset: 66725888, Size: 8},
	{Offset: 66726912, Size: 8},
	{Offset: 66727936, Size: 8},
	{Offset: 66728960, Size: 8},
	{Offset: 66729984, Size: 8},
	{Offset: 66731008, Size: 8},
	{Offset: 66732032, Size: 8},
	{Offset: 66733056, Size: 8},
	{Offset: 66734080, Size: 8},
	{Offset: 66735104, Size: 8},
	{Offset: 66736128, Size: 8},
	{Offset: 66737152, Size: 8},
	{Offset: 66738176, Size: 8},
	{Offset: 66739200, Size: 8},
	{Offset: 66740224, Size: 8},
	{Offset: 66741248, Size: 8},
	{Offset: 66742272, Size: 8},
	{Offset: 66743296, Size: 8},
	{Offset: 66744320, Size: 8},
	{Offset: 66745344, Size: 8},
	{Offset: 66746368, Size: 8},
	{Offset: 66747392, Size: 8},
	{Offset: 66748416, Size: 8},
	{Offset: 66749440, Size: 8},
	{Offset: 66750464, Size: 8},
	{Offset: 66751488, Size: 8},
	{Offset: 66752512, Size: 8},
	{Offset: 66753536, Size: 8},
	{Offset: 66754560, Size: 8},
	{Offset: 66755584, Size: 8},
	{Offset: 66756608, Size: 8},
	{Offset: 66757632, Size: 8},
	{Offset: 66758656, Size: 8},
	{Offset: 66759680, Size: 8},
	{Offset: 66760704, Size: 8},
	{Offset: 66761728, Size: 8},
	{Offset: 66762752, Size: 8},
	{Offset: 66763776, Size: 8},
	{Offset: 66764800, Size: 8},
	{Offset: 66765824, Size: 8},
	{Offset: 66766848, Size: 8},
	{Offset: 66767872, Size: 8},
	{Offset: 66768896, Size: 8},
	{Offset: 66769920, Size: 8},
	{Offset: 66770944, Size: 8},
	{Offset: 66771968, Size: 8},
	{Offset: 66772992, Size: 8},
	{Offset: 66774016, Size: 8},
	{Offset: 66775040, Size: 8},
	{Offset: 66776064, Size: 8},
	{Offset: 66777088, Size: 8},
	{Offset: 66778112, Size: 8},
	{Offset: 66779136, Size: 8},
	{Offset: 66780160, Size: 8},
	{Offset: 66781184, Size: 8},
	{Offset: 66782208, Size: 8},
	{Offset: 66783232, Size: 8},
	{Offset: 66784256, Size: 8},
	{Offset: 66785280, Size: 8},
	{Offset: 66786304, Size: 8},
	{Offset: 66787328, Size: 8},
	{Offset: 66788352, Size: 8},
	{Offset: 66789376, Size: 8},
	{Offset: 66790400, Size: 8},
	{Offset: 66791424, Size: 8},
	{Offset: 66792448, Size: 8},
	{Offset: 66793472, Size: 8},
	{Offset: 66794496, Size: 8},
	{Offset: 66795520, Size: 8},
	{Offset: 66796544, Size: 8},
	{Offset: 66797568, Size: 8},
	{Offset: 66798592, Size: 8},
	{Offset: 66799616, Size: 8},
	{Offset: 66800640, Size: 8},
	{Offset: 66801664, Size: 8},
	{Offset: 66802688, Size: 8},
	{Offset: 66803712, Size: 8},
	{Offset: 66804736, Size: 8},
	{Offset: 66805760, Size: 8},
	{Offset: 66806784, Size: 8},
	{Offset: 66807808, Size: 8},
	{Offset: 66808832, Size: 8},
	{Offset: 66809856, Size: 8},
	{Offset: 66810880, Size: 8},
	{Offset: 66811904, Size: 8},
	{Offset: 66812928, Size: 8},
	{Offset: 66813952, Size: 8},
	{Offset: 66814976, Size: 8},
	{Offset: 66816000, Size: 8},
	{Offset: 66817024, Size: 8},
	{Offset: 66818048, Size: 8},
	{Offset: 66819072, Size: 8},
	{Offset: 66820096, Size: 8},
	{Offset: 66821120, Size: 8},
	{Offset: 66822144, Size: 8},
	{Offset: 66823168, Size: 8},
	{Offset: 66824192, Size: 8},
	{Offset: 66825216, Size: 8},
	{Offset: 66826240, Size: 8},
	{Offset: 66827264, Size: 8},
	{Offset: 66828288, Size: 8},
	{Offset: 66829312, Size: 8},
	{Offset: 66830336, Size: 8},
	{Offset: 66831360, Size: 8},
	{Offset: 66832384, Size: 8},
	{Offset: 66833408, Size: 8},
	{Offset: 66834432, Size: 8},
	{Offset: 66835456, Size: 8},
	{Offset: 66836480, Size: 8},
	{Offset: 66837504, Size: 8},
	{Offset: 66838528, Size: 8},
	{Offset: 66839552, Size: 8},
	{Offset: 66840576, Size: 8},
	{Offset: 66841600, Size: 8},
	{Offset: 66842624, Size: 8},
	{Offset: 66843648, Size: 8},
	{Offset: 66844672, Size: 8},
	{Offset: 66845696, Size: 8},
	{Offset: 66977792, Size: 8},
	{Offset: 66978816, Size: 8},
	{Offset: 66979840, Size: 8},
	{Offset: 66980864, Size: 8},
	{Offset: 66981888, Size: 8},
	{Offset: 66982912, Size: 8},
	{Offset: 66983936, Size: 8},
	{Offset: 66984960, Size: 8},
	{Offset: 66985984, Size: 8},
	{Offset: 66987008, Size: 8},
	{Offset: 66988032, Size: 8},
	{Offset: 66989056, Size: 8},
	{Offset: 66990080, Size: 8},
	{Offset: 66991104, Size: 8},
	{Offset: 66992128, Size: 8},
	{Offset: 66993152, Size: 8},
	{Offset: 66994176, Size: 8},
	{Offset: 66995200, Size: 8},
	{Offset: 66996224, Size: 8},
	{Offset: 66997248, Size: 8},
	{Offset: 66998272, Size: 8},
	{Offset: 66999296, Size: 8},
	{Offset: 67000320, Size: 8},
	{Offset: 67001344, Size: 8},
	{Offset: 67002368, Size: 8},
	{Offset: 67003392, Size: 8},
	{Offset: 67004416, Size: 8},
	{Offset: 67005440, Size: 8},
	{Offset: 67006464, Size: 8},
	{Offset: 67007488, Size: 8},
	{Offset: 67008512, Size: 8},
	{Offset: 67009536, Size: 8},
	{Offset: 67010560, Size: 8},
	{Offset: 67011584, Size: 8},
	{Offset: 67012608, Size: 8},
	{Offset: 67013632, Size: 8},
	{Offset: 67014656, Size: 8},
	{Offset: 67015680, Size: 8},
	{Offset: 67016704, Size: 8},
	{Offset: 67017728, Size: 8},
	{Offset: 67018752, Size: 8},
	{Offset: 67019776, Size: 8},
	{Offset: 67020800, Size: 8},
	{Offset: 67021824, Size: 8},
	{Offset: 67022848, Size: 8},
	{Offset: 67023872, Size: 8},
	{Offset: 67024896, Size: 8},
	{Offset: 67025920, Size: 8},
	{Offset: 67026944, Size: 8},
	{Offset: 67027968, Size: 8},
	{Offset: 67028992, Size: 8},
	{Offset: 67030016, Size: 8},
	{Offset: 67031040, Size: 8},
	{Offset: 67032064, Size: 8},
	{Offset: 67033088, Size: 8},
	{Offset: 67034112, Size: 8},
	{Offset: 67035136, Size: 8},
	{Offset: 67036160, Size: 8},
	{Offset: 67037184, Size: 8},
	{Offset: 67038208, Size: 8},
	{Offset: 67039232, Size: 8},
	{Offset: 67040256, Size: 8},
	{Offset: 67041280, Size: 8},
	{Offset: 67042304, Size: 8},
	{Offset: 67043328, Size: 8},
	{Offset: 67044352, Size: 8},
	{Offset: 67045376, Size: 8},
	{Offset: 67046400, Size: 8},
	{Offset: 67047424, Size: 8},
	{Offset: 67048448, Size: 8},
	{Offset: 67049472, Size: 8},
	{Offset: 67050496, Size: 8},
	{Offset: 67051520, Size: 8},
	{Offset: 67052544, Size: 8},
	{Offset: 67053568, Size: 8},
	{Offset: 67054592, Size: 8},
	{Offset: 67055616, Size: 8},
	{Offset: 67056640, Size: 8},
	{Offset: 67057664, Size: 8},
	{Offset: 67058688, Size: 8},
	{Offset: 67059712, Size: 8},
	{Offset: 67060736, Size: 8},
	{Offset: 67061760, Size: 8},
	{Offset: 67062784, Size: 8},
	{Offset: 67063808, Size: 8},
	{Offset: 67064832, Size: 8},
	{Offset: 67065856, Size: 8},
	{Offset: 67066880, Size: 8},
	{Offset: 67067904, Size: 8},
	{Offset: 67068928, Size: 8},
	{Offset: 67069952, Size: 8},
	{Offset: 67070976, Size: 8},
	{Offset: 67072000, Size: 8},
	{Offset: 67073024, Size: 8},
	{Offset: 67074048, Size: 8},
	{Offset: 67075072, Size: 8},
	{Offset: 67076096, Size: 8},
	{Offset: 67077120, Size: 8},
	{Offset: 67078144, Size: 8},
	{Offset: 67079168, Size: 8},
	{Offset: 67080192, Size: 8},
	{Offset: 67081216, Size: 8},
	{Offset: 67082240, Size: 8},
	{Offset: 67083264, Size: 8},
	{Offset: 67084288, Size: 8},
	{Offset: 67085312, Size: 8},
	{Offset: 67086336, Size: 8},
	{Offset: 67087360, Size: 8},
	{Offset: 67088384, Size: 8},
	{Offset: 67089408, Size: 8},
	{Offset: 67090432, Size: 8},
	{Offset: 67091456, Size: 8},
	{Offset: 67092480, Size: 8},
	{Offset: 67093504, Size: 8},
	{Offset: 67094528, Size: 8},
	{Offset: 67095552, Size: 8},
	{Offset: 67096576, Size: 8},
	{Offset: 67097600, Size: 8},
	{Offset: 67098624, Size: 8},
	{Offset: 67099648, Size: 8},
	{Offset: 67100672, Size: 8},
	{Offset: 67101696, Size: 8},
	{Offset: 67102720, Size: 8},
	{Offset: 67103744, Size: 8},
	{Offset: 67104768, Size: 8},
	{Offset: 67105792, Size: 8},
	{Offset: 67106816, Size: 8},
	{Offset: 67107840, Size: 8},
}
