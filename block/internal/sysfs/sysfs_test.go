// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package sysfs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/siderolabs/go-blockdevice/v2/block/internal/sysfs"
)

func TestReadFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(root, "uuid"), []byte("mpath-3600508\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "empty"), nil, 0o644))

	assert.Equal(t, "mpath-3600508", sysfs.ReadFile(filepath.Join(root, "uuid")))
	assert.Empty(t, sysfs.ReadFile(filepath.Join(root, "empty")))
	assert.Empty(t, sysfs.ReadFile(filepath.Join(root, "missing")))
	assert.Empty(t, sysfs.ReadFile(root))
}
