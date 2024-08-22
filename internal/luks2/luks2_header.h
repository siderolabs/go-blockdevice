#include <stdint.h>

struct luks2_phdr {
	char		magic[6];
	uint16_t	version;
	uint64_t	hdr_size;	/* in bytes, including JSON area */
	uint64_t	seqid;		/* increased on every update */
	char		label[48];
	char		checksum_alg[32];
	uint8_t		salt[64]; /* unique for every header/offset */
	char		uuid[40];
	char		subsystem[48]; /* owner subsystem label */
	uint64_t	hdr_offset;	/* offset from device start in bytes */
	char		_padding[184];
	uint8_t		csum[64];
	/* Padding to 4k, then JSON area */
} __attribute__ ((packed));
