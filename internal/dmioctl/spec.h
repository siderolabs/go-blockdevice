#include <stdint.h>

/*
 * struct dm_target_spec from <linux/dm-ioctl.h>.
 *
 * 'int32_t status' is spelled unsigned here (it is written by the kernel only) so that the
 * generator emits an accessor for it and the offsets which follow stay correct.
 */
struct dm_target_spec {
	uint64_t	sector_start;
	uint64_t	length;
	uint32_t	status;		/* used when reading from kernel only */
	uint32_t	next;		/* offset in bytes to the next target_spec */
	char	target_type[16];
};
