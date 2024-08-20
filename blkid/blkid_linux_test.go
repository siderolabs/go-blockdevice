// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build linux

package blkid_test

import (
	"bytes"
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

	cmd := exec.Command("mkfs.xfs", "--unsupported", "-L", "somelabel", path)
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

//go:embed testdata/zfs.img.zst
var zfsImage []byte

func zfsSetup(t *testing.T, path string) {
	t.Helper()

	out, err := os.OpenFile(path, os.O_RDWR, 0)
	require.NoError(t, err)

	zr, err := zstd.NewReader(bytes.NewReader(zfsImage))
	require.NoError(t, err)

	_, err = io.Copy(out, zr)
	require.NoError(t, err)

	require.NoError(t, out.Close())
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

func swapSetup(t *testing.T, path string) {
	t.Helper()

	cmd := exec.Command("mkswap", "--label", "swaplabel", "-p", "8192", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())
}

func swapSetup2(t *testing.T, path string) {
	t.Helper()

	cmd := exec.Command("mkswap", "--label", "swapswap", "-p", "4096", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())
}

func lvm2Setup(t *testing.T, path string) {
	t.Helper()

	cmd := exec.Command("pvcreate", "-v", path)
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

	cmd := exec.Command("mksquashfs", contents, path, "-all-root", "-noappend", "-no-progress", "-no-compression")
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
		},
		{
			name:     "lvm2-pv",
			loopOnly: true,

			size:  500 * MiB,
			setup: lvm2Setup,

			expectedName:       "lvm2-pv",
			expectedLabelRegex: regexp.MustCompile(`(?m)^[0-9a-zA-Z]{6}-[0-9a-zA-Z]{4}-[0-9a-zA-Z]{4}-[0-9a-zA-Z]{4}-[0-9a-zA-Z]{4}-[0-9a-zA-Z]{4}-[0-9a-zA-Z]{6}$`),
		},
		{
			name:   "zfs",
			noLoop: true,

			size:  0,
			setup: zfsSetup,

			expectedName:       "zfs",
			expectedLabelRegex: regexp.MustCompile(`^[0-9a-f]{16}$`),
		},
		{
			name:   "squashfs",
			noLoop: true,

			size:  0,
			setup: squashfsSetup,

			expectedName: "squashfs",

			expectedBlockSize:   []uint32{0x20000},
			expectedFSBlockSize: []uint32{0x20000},
			expectedFSSize:      0x100554,
		},
		{
			name: "talosmeta",

			size:  2 * 256 * 1024,
			setup: talosmetaSetup,

			expectedName: "talosmeta",

			expectedFSSize: 2 * 256 * 1024,
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

	cmd := exec.Command("sfdisk", path)
	cmd.Stdin = strings.NewReader(script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run())
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

func TestProbePathGPT(t *testing.T) {
	for _, test := range []struct { //nolint:govet
		name string

		size  uint64
		setup func(*testing.T, string)

		expectedUUID  uuid.UUID
		expectedParts []blkid.NestedProbeResult
	}{
		{
			name: "good GPT",

			size:  2 * GiB,
			setup: setupGPT,

			expectedUUID:  uuid.MustParse("DDDA0816-8B53-47BF-A813-9EBB1F73AAA2"),
			expectedParts: expectedParts,
		},
		{
			name: "corrupted GPT",

			size:  2 * GiB,
			setup: wipe1MB(setupGPT),

			expectedUUID:  uuid.MustParse("DDDA0816-8B53-47BF-A813-9EBB1F73AAA2"),
			expectedParts: expectedParts,
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
					assert.Equal(t, test.size-1*MiB-33*block.DefaultBlockSize, info.ProbedSize)

					require.NotNil(t, info.UUID)
					assert.Equal(t, test.expectedUUID, *info.UUID)

					assert.Equal(t, test.expectedParts, info.Parts)
				})
			})
		}
	}
}

func setupNestedGPT(t *testing.T, path string) {
	t.Helper()

	setupGPT(t, path)

	require.NoError(t, exec.Command("partprobe", path).Run())

	vfatSetup(t, path+"p1")
	extfsSetup(t, path+"p3")
	xfsSetup(t, path+"p6")
}

func TestProbePathNested(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test requires root privileges")
	}

	if hostname, _ := os.Hostname(); hostname == "buildkitsandbox" { //nolint: errcheck
		t.Skip("test not supported under buildkit as partition devices are not propagated from /dev")
	}

	for _, test := range []struct { //nolint:govet
		name string

		size  uint64
		setup func(*testing.T, string)

		expectedUUID  uuid.UUID
		expectedParts []blkid.NestedProbeResult
	}{
		{
			name: "good GPT, ext4fs, xfs, vfat, none",

			size:  2 * GiB,
			setup: setupNestedGPT,

			expectedUUID:  uuid.MustParse("DDDA0816-8B53-47BF-A813-9EBB1F73AAA2"),
			expectedParts: expectedParts,
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
			assert.Equal(t, "extfs", info.Parts[2].Name)
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

func setupOurGPT(t *testing.T, path string) {
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

	vfatSetup(t, path+"p1")
	xfsSetup(t, path+"p3")
	xfsSetup(t, path+"p5")
	xfsSetup(t, path+"p6")

	require.NoError(t, blk.Unlock())
	require.NoError(t, blk.Close())
}

func TestProbePathOurGPT(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test requires root privileges")
	}

	if hostname, _ := os.Hostname(); hostname == "buildkitsandbox" { //nolint: errcheck
		t.Skip("test not supported under buildkit as partition devices are not propagated from /dev")
	}

	for _, test := range []struct { //nolint:govet
		name string

		size  uint64
		setup func(*testing.T, string)

		expectedUUID  uuid.UUID
		expectedParts []blkid.NestedProbeResult
	}{
		{
			name: "good GPT, ext4fs, xfs, vfat, none",

			size:  2 * GiB,
			setup: setupOurGPT,

			expectedUUID:  uuid.MustParse("DDDA0816-8B53-47BF-A813-9EBB1F73AAA2"),
			expectedParts: expectedParts,
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
