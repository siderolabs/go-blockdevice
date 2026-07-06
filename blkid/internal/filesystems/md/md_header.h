#include <stdint.h>

/* https://github.com/torvalds/linux/blob/master/drivers/md/md_p.h struct mdp_superblock_1 (constant part) */
struct mdp_superblock_1 {
	/* constant array information - 128 bytes */
	uint32_t	magic;			/* MD_SB_MAGIC: 0xa92b4efc - little endian */
	uint32_t	major_version;		/* 1 */
	uint32_t	feature_map;		/* bit 0 set if 'bitmap_offset' is meaningful */
	uint32_t	pad0;			/* always set to 0 when writing */

	uint8_t		set_uuid[16];		/* user-object uuid */
	char		set_name[32];		/* set and interpreted by user-space */

	uint64_t	ctime;			/* lo 40 bits are seconds, top 24 are microseconds or 0 */
	uint32_t	level;			/* -4 (multipath), -1 (linear), 0,1,4,5 */
	uint32_t	layout;			/* only for raid5 and raid10 currently */
	uint64_t	size;			/* used size of component devices, in 512byte sectors */

	uint32_t	chunksize;		/* in 512byte sectors */
	uint32_t	raid_disks;
	uint32_t	bitmap_offset;		/* sectors after start of superblock that bitmap starts */

	uint32_t	new_level;		/* only for feature bit '4' */
	uint64_t	reshape_position;	/* only for feature bit '4' */
	uint32_t	delta_disks;		/* only for feature bit '4' */
	uint32_t	new_layout;		/* only for feature bit '4' */
	uint32_t	new_chunk;		/* only for feature bit '4' */
	uint32_t	new_offset;		/* only for feature bit '4' */

	/* constant this-device information - 64 bytes */
	uint64_t	data_offset;		/* sector start of data, often 0 */
	uint64_t	data_size;		/* sectors in this device that can be used for data */
	uint64_t	super_offset;		/* sector start of this superblock */
} __attribute__ ((packed));
