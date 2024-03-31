#include <stdint.h>

/* https://github.com/util-linux/util-linux/blob/c0207d354ee47fb56acfa64b03b5b559bb301280/libblkid/src/superblocks/lvm.c#L23-L32 */
struct lvm2_pv_header {
	/* label_header */
	uint8_t		id[8];		/* LABELONE */
	uint64_t	sector_xl;	/* Sector number of this label */
	uint32_t	crc_xl;		/* From next field to end of sector */
	uint32_t	offset_xl;	/* Offset from start of struct to contents */
	uint8_t		type[8];	/* LVM2 001 */
	/* pv_header */
	uint8_t		pv_uuid[32];
} __attribute__ ((packed));
