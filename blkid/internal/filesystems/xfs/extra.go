// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package xfs

// XFS superblock structure constants.
//
//nolint:revive,stylecheck
const (
	XFS_MIN_BLOCKSIZE_LOG  = 9  /* i.e. 512 bytes */
	XFS_MAX_BLOCKSIZE_LOG  = 16 /* i.e. 65536 bytes */
	XFS_MIN_BLOCKSIZE      = (1 << XFS_MIN_BLOCKSIZE_LOG)
	XFS_MAX_BLOCKSIZE      = (1 << XFS_MAX_BLOCKSIZE_LOG)
	XFS_MIN_SECTORSIZE_LOG = 9  /* i.e. 512 bytes */
	XFS_MAX_SECTORSIZE_LOG = 15 /* i.e. 32768 bytes */
	XFS_MIN_SECTORSIZE     = (1 << XFS_MIN_SECTORSIZE_LOG)
	XFS_MAX_SECTORSIZE     = (1 << XFS_MAX_SECTORSIZE_LOG)

	XFS_DINODE_MIN_LOG  = 8
	XFS_DINODE_MAX_LOG  = 11
	XFS_DINODE_MIN_SIZE = (1 << XFS_DINODE_MIN_LOG)
	XFS_DINODE_MAX_SIZE = (1 << XFS_DINODE_MAX_LOG)

	XFS_MAX_RTEXTSIZE = (1024 * 1024 * 1024) /* 1GB */
	XFS_DFL_RTEXTSIZE = (64 * 1024)          /* 64kB */
	XFS_MIN_RTEXTSIZE = (4 * 1024)           /* 4kB */

	XFS_MIN_AG_BLOCKS = 64
)

// Valid returns true if the superblock is valid.
//
//nolint:gocyclo,cyclop
func (s SuperBlock) Valid() bool {
	if s.Get_sb_agcount() <= 0 ||
		s.Get_sb_sectsize() < XFS_MIN_SECTORSIZE ||
		s.Get_sb_sectsize() > XFS_MAX_SECTORSIZE ||
		s.Get_sb_sectlog() < XFS_MIN_SECTORSIZE_LOG ||
		s.Get_sb_sectlog() > XFS_MAX_SECTORSIZE_LOG ||
		s.Get_sb_sectsize() != (1<<s.Get_sb_sectlog()) ||
		s.Get_sb_blocksize() < XFS_MIN_BLOCKSIZE ||
		s.Get_sb_blocksize() > XFS_MAX_BLOCKSIZE ||
		s.Get_sb_blocklog() < XFS_MIN_BLOCKSIZE_LOG ||
		s.Get_sb_blocklog() > XFS_MAX_BLOCKSIZE_LOG ||
		s.Get_sb_blocksize() != (1<<s.Get_sb_blocklog()) ||
		s.Get_sb_inodesize() < XFS_DINODE_MIN_SIZE ||
		s.Get_sb_inodesize() > XFS_DINODE_MAX_SIZE ||
		s.Get_sb_inodelog() < XFS_DINODE_MIN_LOG ||
		s.Get_sb_inodelog() > XFS_DINODE_MAX_LOG ||
		s.Get_sb_inodesize() != (1<<s.Get_sb_inodelog()) ||
		(s.Get_sb_blocklog()-s.Get_sb_inodelog() != s.Get_sb_inopblog()) ||
		(s.Get_sb_rextsize()*s.Get_sb_blocksize() > XFS_MAX_RTEXTSIZE) ||
		(s.Get_sb_rextsize()*s.Get_sb_blocksize() < XFS_MIN_RTEXTSIZE) ||
		(s.Get_sb_imax_pct() > 100 /* zero sb_imax_pct is valid */) ||
		s.Get_sb_dblocks() == 0 {
		return false
	}

	return true
}

// FilesystemSize returns the size of the filesystem in bytes.
func (s SuperBlock) FilesystemSize() uint64 {
	logsBlocks := uint32(0)

	if s.Get_sb_logstart() != 0 {
		logsBlocks = s.Get_sb_logblocks()
	}

	availBlocks := s.Get_sb_dblocks() - uint64(logsBlocks)

	return availBlocks * uint64(s.Get_sb_blocksize())
}
