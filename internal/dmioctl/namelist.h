#include <stdint.h>

/*
 * struct dm_name_list from <linux/dm-ioctl.h>, the record of a ListDevicesRequest reply.
 *
 * The kernel declares a trailing 'char name[0]'; the NUL-terminated name follows this structure,
 * so it is left out here and read at NAMELIST_SIZE.
 */
struct dm_name_list {
	uint64_t	dev;
	uint32_t	next;		/* offset to the next record from the start of this one */
};
