#include <stdint.h>

/* https://github.com/util-linux/util-linux/blob/8179f69eec2f04be47607d89349505ab7d6ec6be/libblkid/src/superblocks/vmfs.c#L18-L23 */
struct vmfs_volume_info {
	uint32_t	magic;
	uint32_t	ver;
	uint8_t		irrelevant[122];
	uint8_t		uuid[16];
} __attribute__ ((packed));
