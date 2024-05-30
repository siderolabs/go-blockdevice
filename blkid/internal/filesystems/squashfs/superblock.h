#include <stdint.h>

struct sqsh_super_block {
	uint32_t	magic;
	uint32_t	inode_count;
	uint32_t	mod_time;
	uint32_t	block_size;
	uint32_t	frag_count;
	uint16_t	compressor;
	uint16_t	block_log;
	uint16_t	flags;
	uint16_t	id_count;
	uint16_t	version_major;
	uint16_t	version_minor;
	uint64_t	root_inode;
	uint64_t	bytes_used;
	uint64_t	id_table;
	uint64_t	xattr_table;
	uint64_t	inode_table;
	uint64_t	dir_table;
	uint64_t	frag_table;
	uint64_t	export_table;
} __attribute__((packed));
