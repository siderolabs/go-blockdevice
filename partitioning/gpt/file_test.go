// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gpt_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/siderolabs/go-blockdevice/v2/partitioning/gpt"
)

// gptSignature is the on-disk "EFI PART" magic at the start of a GPT header.
var gptSignature = []byte("EFI PART")

func newImageFile(t *testing.T, size int64) *os.File {
	t.Helper()

	f, err := os.Create(filepath.Join(t.TempDir(), "image.raw"))
	require.NoError(t, err)
	require.NoError(t, f.Truncate(size))

	t.Cleanup(func() {
		assert.NoError(t, f.Close())
	})

	return f
}

func TestDeviceFromFileDefaultSectorSize(t *testing.T) {
	f := newImageFile(t, 16*1024*1024)

	dev, err := gpt.DeviceFromFile(f)
	require.NoError(t, err)

	assert.Equal(t, uint(512), dev.GetSectorSize())

	ioSize, err := dev.GetIOSize()
	require.NoError(t, err)
	assert.Equal(t, uint(512), ioSize)
}

func TestDeviceFromFileWithSectorSize(t *testing.T) {
	for _, sectorSize := range []uint{512, 4096} {
		t.Run("sector="+strconv.FormatUint(uint64(sectorSize), 10), func(t *testing.T) {
			f := newImageFile(t, 64*1024*1024)

			dev, err := gpt.DeviceFromFile(f, gpt.WithFileSectorSize(sectorSize))
			require.NoError(t, err)

			assert.Equal(t, sectorSize, dev.GetSectorSize())

			ioSize, err := dev.GetIOSize()
			require.NoError(t, err)
			assert.Equal(t, sectorSize, ioSize)
		})
	}
}

// TestDeviceFromFileSectorSizeRoundTrip ensures the configured sector size is
// honored end-to-end: a GPT table written through the file device with a 4K
// sector size must place the primary header at LBA 1 of a 4K sector (offset
// 4096), and a re-read must recover the same partition layout.
func TestDeviceFromFileSectorSizeRoundTrip(t *testing.T) {
	const (
		MiB        = 1024 * 1024
		imageSize  = 64 * MiB
		sectorSize = 4096
	)

	partType := uuid.MustParse("0FC63DAF-8483-4772-8E79-3D69D8477DE4")

	f := newImageFile(t, imageSize)

	dev, err := gpt.DeviceFromFile(f, gpt.WithFileSectorSize(sectorSize))
	require.NoError(t, err)

	table, err := gpt.New(dev)
	require.NoError(t, err)

	idx, _, err := table.AllocatePartition(8*MiB, "test", partType)
	require.NoError(t, err)
	require.Equal(t, 1, idx)

	require.NoError(t, table.Write())

	// Primary header sits at LBA 1, which with a 4K sector is offset 4096.
	hdr := make([]byte, len(gptSignature))
	_, err = f.ReadAt(hdr, sectorSize)
	require.NoError(t, err)
	assert.Equal(t, gptSignature, hdr, "expected GPT signature at offset %d (LBA 1 of %d-byte sectors)", sectorSize, sectorSize)

	// At offset 512 (LBA 1 if sector size were 512) there must NOT be a GPT
	// signature - that location is inside the protective MBR sector.
	at512 := make([]byte, len(gptSignature))
	_, err = f.ReadAt(at512, 512)
	require.NoError(t, err)
	assert.NotEqual(t, gptSignature, at512, "GPT signature unexpectedly found at offset 512; sector size override was not honored")

	// Re-read the table and confirm partitions round-trip.
	dev2, err := gpt.DeviceFromFile(f, gpt.WithFileSectorSize(sectorSize))
	require.NoError(t, err)

	table2, err := gpt.Read(dev2)
	require.NoError(t, err)

	assert.Equal(t, table.Partitions(), table2.Partitions())
}

// TestDeviceFromFileDefaultSectorPlacement is the 512-byte counterpart of the
// test above - it pins the documented default behavior, so any future change
// that silently shifts the default sector size will break here.
func TestDeviceFromFileDefaultSectorPlacement(t *testing.T) {
	const (
		MiB       = 1024 * 1024
		imageSize = 16 * MiB
	)

	partType := uuid.MustParse("0FC63DAF-8483-4772-8E79-3D69D8477DE4")

	f := newImageFile(t, imageSize)

	dev, err := gpt.DeviceFromFile(f)
	require.NoError(t, err)

	table, err := gpt.New(dev)
	require.NoError(t, err)

	_, _, err = table.AllocatePartition(2*MiB, "test", partType)
	require.NoError(t, err)

	require.NoError(t, table.Write())

	hdr := make([]byte, len(gptSignature))
	_, err = f.ReadAt(hdr, 512)
	require.NoError(t, err)
	assert.Equal(t, gptSignature, hdr, "expected GPT signature at offset 512 with default 512-byte sectors")
}
