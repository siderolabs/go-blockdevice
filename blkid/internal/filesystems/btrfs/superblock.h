#include <stdint.h>

struct btrfs_super_block {
	uint8_t		csum[32];			/* checksum */
	uint8_t		fsid[16];			/* filesystem UUID */
	uint64_t	bytenr;				/* this block number */
	uint64_t	flags;				/* flags */
	uint64_t	magic;				/* BTRFS_MAGIC */
	uint64_t	generation;			/* generation */
	uint64_t	root;				/* tree root */
	uint64_t	chunk_root;			/* chunk tree root */
	uint64_t	log_root;			/* log tree root */
	uint64_t	log_root_transid;		/* log root transid */
	uint64_t	total_bytes;			/* total filesystem size */
	uint64_t	bytes_used;			/* bytes used */
	uint64_t	root_dir_objectid;		/* root dir objectid */
	uint64_t	num_devices;			/* number of devices */
	uint32_t	sectorsize;			/* sector size */
	uint32_t	nodesize;			/* node size */
	uint32_t	leafsize;			/* leaf size (deprecated) */
	uint32_t	stripesize;			/* stripe size */
	uint32_t	sys_chunk_array_size;		/* size of sys_chunk_array */
	uint64_t	chunk_root_generation;		/* chunk root generation */
	uint64_t	compat_flags;			/* compat flags */
	uint64_t	compat_ro_flags;		/* compat read-only flags */
	uint64_t	incompat_flags;			/* incompat flags */
	uint16_t	csum_type;			/* checksum type */
	uint8_t		root_level;			/* root level */
	uint8_t		chunk_root_level;		/* chunk root level */
	uint8_t		log_root_level;			/* log root level */
	uint8_t		dev_item[98];			/* embedded device */
	char		label[256];			/* filesystem label */
} __attribute__((packed));
