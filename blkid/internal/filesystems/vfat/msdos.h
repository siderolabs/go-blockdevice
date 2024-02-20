#include <stdint.h>

struct msdos_super_block {
/* DOS 2.0 BPB */
/* 00*/	unsigned char	ms_ignored[3];
/* 03*/	unsigned char	ms_sysid[8];
/* 0b*/	uint16_t	ms_sector_size;
/* 0d*/	uint8_t		ms_cluster_size;
/* 0e*/	uint16_t	ms_reserved;
/* 10*/	uint8_t		ms_fats;
/* 11*/	uint16_t	ms_dir_entries;
/* 13*/	uint16_t	ms_sectors; /* =0 iff V3 or later */
/* 15*/	unsigned char	ms_media;
/* 16*/	uint16_t	ms_fat_length; /* Sectors per FAT */
/* DOS 3.0 BPB */
/* 18*/	uint16_t	ms_secs_track;
/* 1a*/	uint16_t	ms_heads;
/* 1c*/	uint32_t	ms_hidden;
/* DOS 3.31 BPB */
/* 20*/	uint32_t	ms_total_sect; /* iff ms_sectors == 0 */
/* DOS 3.4 EBPB */
/* 24*/	unsigned char	ms_drive_number;
/* 25*/	unsigned char	ms_boot_flags;
/* 26*/	unsigned char	ms_ext_boot_sign; /* 0x28 - DOS 3.4 EBPB; 0x29 - DOS 4.0 EBPB */
/* 27*/	unsigned char	ms_serno[4];
/* DOS 4.0 EBPB */
/* 2b*/	unsigned char	ms_label[11];
/* 36*/	unsigned char   ms_magic[8];
/* padding */
/* 3e*/	unsigned char	ms_dummy2[448];
/*1fe*/	unsigned char	ms_pmagic[2];
} __attribute__((packed));
