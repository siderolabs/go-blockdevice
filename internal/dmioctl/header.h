#include <stdint.h>

/*
 * struct dm_ioctl from <linux/dm-ioctl.h>.
 *
 * Two spellings differ from the kernel header, so that the generator emits usable accessors:
 * 'uint32_t version[3]' is spelled out as three fields, and 'int32_t open_count' is unsigned
 * (it is written by the kernel only, and is never negative).
 */
struct dm_ioctl {
	uint32_t	version_major;
	uint32_t	version_minor;
	uint32_t	version_patch;
	uint32_t	data_size;	/* total size of data passed in including this struct */
	uint32_t	data_start;	/* offset to start of data relative to start of this struct */
	uint32_t	target_count;	/* in/out */
	uint32_t	open_count;	/* out */
	uint32_t	flags;		/* in/out */
	uint32_t	event_nr;	/* in/out */
	uint32_t	padding;
	uint64_t	dev;		/* in/out */
	char	name[128];	/* device name */
	char	uuid[129];	/* unique identifier for the block device */
	char	data[7];	/* padding or data */
};
