// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package iso9660_test

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/filesystems/iso9660"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
)

type imageReader struct {
	*bytes.Reader

	sectorSize uint
}

func (r imageReader) GetSectorSize() uint {
	return r.sectorSize
}

func (r imageReader) GetSize() uint64 {
	return uint64(r.Size())
}

func TestProbeHybridGPT(t *testing.T) {
	t.Parallel()

	f, err := os.Open("../../../testdata/hybrid.iso.zst")
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, f.Close()) })

	zr, err := zstd.NewReader(f)
	require.NoError(t, err)

	t.Cleanup(zr.Close)

	image, err := io.ReadAll(zr)
	require.NoError(t, err)

	// the GPT in hybrid ISO images always uses 512-byte sectors, while CD-ROM devices have 2048-byte sectors
	for _, sectorSize := range []uint{512, 2048} {
		res, err := (&iso9660.Probe{}).Probe(imageReader{Reader: bytes.NewReader(image), sectorSize: sectorSize}, magic.Magic{})
		require.NoError(t, err)
		require.NotNil(t, res)

		require.Len(t, res.Parts, 3)

		esp := res.Parts[1]
		assert.Equal(t, uuid.MustParse("36323032-3930-4232-b132-303635323135"), *esp.UUID)
		assert.Equal(t, uuid.MustParse("c12a7328-f81f-11d2-ba4b-00a0c93ec93b"), *esp.TypeUUID)
		assert.EqualValues(t, 2, esp.Index)
		assert.EqualValues(t, 2048*512, esp.Offset)
		assert.EqualValues(t, 8192*512, esp.Size)
	}
}
