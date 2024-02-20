// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package ext

// BlockSize returns the block size of the filesystem.
func (s SuperBlock) BlockSize() uint32 {
	if s.Get_s_log_block_size() >= 32 {
		return 0
	}

	return 1024 << s.Get_s_log_block_size()
}

// FilesystemSize returns the size of the filesystem.
func (s SuperBlock) FilesystemSize() uint64 {
	return uint64(s.Get_s_blocks_count()) * uint64(s.BlockSize())
}
