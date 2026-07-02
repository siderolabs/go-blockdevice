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

func TestGetTransportDM(t *testing.T) {
	t.Parallel()

	d := &Device{}

	for _, tt := range []struct {
		name string
		uuid string
	}{
		{"mpath whole device", "mpath-36005076810800567c8000000000009f8"},
		{"mpath partition part-N- form", "part-1-mpath-36005076810800567c8000000000009f8"},
		{"mpath partition partN- form", "part1-mpath-36005076810800567c8000000000009f8"},
		{"LVM", "LVM-AbCdEf-0001-0002-0003-0004-0005-0006-lvname"},
		{"crypt", "CRYPT-LUKS2-abcdef1234567890abcdef1234567890-cryptname"},
		{"generic dm", "some-other-uuid"},
		{"no uuid file", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()

			if tt.uuid != "" {
				require.NoError(t, os.MkdirAll(filepath.Join(root, "dm"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(root, "dm", "uuid"), []byte(tt.uuid+"\n"), 0o644))
			}

			assert.Equal(t, "dm", d.getTransport(root, "dm-0"))
		})
	}
}

func TestDMKind(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		uuid     string
		expected string
	}{
		{"mpath whole device", "mpath-36005076810800567c8000000000009f8", "mpath"},
		{"mpath partition part-N- form", "part-1-mpath-36005076810800567c8000000000009f8", "mpath"},
		{"mpath partition partN- form", "part1-mpath-36005076810800567c8000000000009f8", "mpath"},
		{"LVM", "LVM-AbCdEf-0001-0002-0003-0004-0005-0006-lvname", "lvm"},
		{"crypt", "CRYPT-LUKS2-abcdef1234567890abcdef1234567890-cryptname", "crypt"},
		{"generic dm", "some-other-uuid", "dm"},
		{"empty", "", "dm"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, dmKind(tt.uuid))
		})
	}
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
