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
