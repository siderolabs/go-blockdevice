#include <stdint.h>

struct xfs_super_block {
	uint32_t	sb_magicnum;	/* magic number == XFS_SB_MAGIC */
	uint32_t	sb_blocksize;	/* logical block size, bytes */
	uint64_t	sb_dblocks;	/* number of data blocks */
	uint64_t	sb_rblocks;	/* number of realtime blocks */
	uint64_t	sb_rextents;	/* number of realtime extents */
	unsigned char	sb_uuid[16];	/* file system unique id */
	uint64_t	sb_logstart;	/* starting block of log if internal */
	uint64_t	sb_rootino;	/* root inode number */
	uint64_t	sb_rbmino;	/* bitmap inode for realtime extents */
	uint64_t	sb_rsumino;	/* summary inode for rt bitmap */
	uint32_t	sb_rextsize;	/* realtime extent size, blocks */
	uint32_t	sb_agblocks;	/* size of an allocation group */
	uint32_t	sb_agcount;	/* number of allocation groups */
	uint32_t	sb_rbmblocks;	/* number of rt bitmap blocks */
	uint32_t	sb_logblocks;	/* number of log blocks */

	uint16_t	sb_versionnum;	/* header version == XFS_SB_VERSION */
	uint16_t	sb_sectsize;	/* volume sector size, bytes */
	uint16_t	sb_inodesize;	/* inode size, bytes */
	uint16_t	sb_inopblock;	/* inodes per block */
	char		sb_fname[12];	/* file system name */
	uint8_t		sb_blocklog;	/* log2 of sb_blocksize */
	uint8_t		sb_sectlog;	/* log2 of sb_sectsize */
	uint8_t		sb_inodelog;	/* log2 of sb_inodesize */
	uint8_t		sb_inopblog;	/* log2 of sb_inopblock */
	uint8_t		sb_agblklog;	/* log2 of sb_agblocks (rounded up) */
	uint8_t		sb_rextslog;	/* log2 of sb_rextents */
	uint8_t		sb_inprogress;	/* mkfs is in progress, don't mount */
	uint8_t		sb_imax_pct;	/* max % of fs for inode space */
					/* statistics */
	uint64_t	sb_icount;	/* allocated inodes */
	uint64_t	sb_ifree;	/* free inodes */
	uint64_t	sb_fdblocks;	/* free data blocks */
	uint64_t	sb_frextents;	/* free realtime extents */
	uint64_t	sb_uquotino;	/* inode for user quotas */
	uint64_t	sb_gquotino;	/* inode for group or project quotas */
	uint16_t	sb_qflags;	/* quota flags */
	uint8_t		sb_flags;	/* misc flags */
	uint8_t		sb_shared_vn;	/* reserved, zeroed */
	uint32_t	sb_inoalignmt;	/* inode alignment */
	uint32_t	sb_unit;	/* stripe or raid unit */
	uint32_t	sb_width;	/* stripe or raid width */
	uint8_t		sb_dirblklog;	/* directory block allocation granularity */
	uint8_t		sb_logsectlog;	/* log sector sector size */
	uint16_t	sb_logsectsize;	/* log sector size */
	uint32_t	sb_logsunit;	/* log device stripe or raid unit */
	uint32_t	sb_features2;	/* additional version flags */
	uint32_t	sb_bad_features2;	/* mirror of sb_features2 */
} __attribute__((packed));
