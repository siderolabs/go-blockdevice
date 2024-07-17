#include <stdint.h>

struct gpt_entry {
	uint8_t	partition_type_guid[16];	/* type UUID */
	uint8_t	unique_partition_guid[16];	/* partition UUID */
	uint64_t	starting_lba;
	uint64_t	ending_lba;

	/*struct gpt_entry_attributes	attributes;*/

	uint64_t	attributes;

	uint8_t	partition_name[72]; /* UTF-16LE string*/
} __attribute__ ((packed));
