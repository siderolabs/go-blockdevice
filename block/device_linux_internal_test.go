// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package block

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadNVMeFirmwareRevision(t *testing.T) {
	t.Parallel()

	t.Run("present", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "device"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "device", "firmware_rev"), []byte("1B2QEXM7\n"), 0o644))

		assert.Equal(t, "1B2QEXM7", readNVMeFirmwareRevision(root))
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, readNVMeFirmwareRevision(t.TempDir()))
	})
}

func TestReadBlockDeviceModalias(t *testing.T) {
	t.Parallel()

	t.Run("scsi-style present at device/modalias", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "device"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "device", "modalias"), []byte("scsi:t-0x00\n"), 0o644))

		assert.Equal(t, "scsi:t-0x00", readBlockDeviceModalias(root))
	})

	t.Run("nvme-style present only at device/device/modalias", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "device", "device"), 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(root, "device", "device", "modalias"),
			[]byte("pci:v000015B7d00005030sv000015B7sd00005030bc01sc08i02\n"),
			0o644,
		))

		assert.Equal(t, "pci:v000015B7d00005030sv000015B7sd00005030bc01sc08i02", readBlockDeviceModalias(root))
	})

	t.Run("device/modalias takes precedence", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "device", "device"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "device", "modalias"), []byte("primary\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, "device", "device", "modalias"), []byte("fallback\n"), 0o644))

		assert.Equal(t, "primary", readBlockDeviceModalias(root))
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, readBlockDeviceModalias(t.TempDir()))
	})
}
