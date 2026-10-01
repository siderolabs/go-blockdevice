#include <stdint.h>

/* https://github.com/util-linux/util-linux/blob/8179f69eec2f04be47607d89349505ab7d6ec6be/libblkid/src/superblocks/vmfs.c#L9-L16 */
struct vmfs_fs_info {
	uint32_t	magic;
	uint32_t	volume_version;
	uint8_t		version;
	uint8_t		uuid[16];
	uint32_t	mode;
	char		label[128];
} __attribute__ ((packed));
