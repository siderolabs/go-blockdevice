#include <stdint.h>

struct gpt_header {
	uint64_t	signature;		/* "EFI PART" */
	uint32_t	revision;
	uint32_t	header_size;		/* usually 92 bytes */
	uint32_t	header_crc32;		/* checksum of header with this
						 * field zeroed during calculation */
	uint32_t	reserved1;

	uint64_t	my_lba;			/* location of this header copy */
	uint64_t	alternate_lba;		/* location of the other header copy */
	uint64_t	first_usable_lba;	/* first usable LBA for partitions */
	uint64_t	last_usable_lba;	/* last usable LBA for partitions */

	uint8_t	disk_guid[16];		/* disk UUID */

	uint64_t	partition_entries_lba;	/* always 2 in primary header copy */
	uint32_t	num_partition_entries;
	uint32_t	sizeof_partition_entry;
	uint32_t	partition_entry_array_crc32;

	/*
	 * The rest of the block is reserved by UEFI and must be zero. EFI
	 * standard handles this by:
	 *
	 * uint8_t		reserved2[ BLKSSZGET - 92 ];
	 *
	 * This definition is useless in practice. It is necessary to read
	 * whole block from the device rather than sizeof(struct gpt_header)
	 * only.
	 */
} __attribute__ ((packed));
