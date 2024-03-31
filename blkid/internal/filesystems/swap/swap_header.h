#include <stdint.h>

/* linux/include/linux/swap.h */
struct swap_header {
	uint32_t	version;
	uint32_t	lastpage;
	uint32_t	nr_badpages;
	unsigned char	uuid[16];
	unsigned char	volume[16];
	uint32_t	padding[117];
	uint32_t	badpages[1];
} __attribute__((packed));
