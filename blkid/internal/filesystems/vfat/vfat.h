#include <stdint.h>

struct vfat_super_block {
/* 00*/	unsigned char	vs_ignored[3];
/* 03*/	unsigned char	vs_sysid[8];
/* 0b*/	unsigned char	vs_sector_size[2];
/* 0d*/	uint8_t		vs_cluster_size;
/* 0e*/	uint16_t	vs_reserved;
/* 10*/	uint8_t		vs_fats;
/* 11*/	unsigned char	vs_dir_entries[2];
/* 13*/	unsigned char	vs_sectors[2];
/* 15*/	unsigned char	vs_media;
/* 16*/	uint16_t	vs_fat_length;
/* 18*/	uint16_t	vs_secs_track;
/* 1a*/	uint16_t	vs_heads;
/* 1c*/	uint32_t	vs_hidden;
/* 20*/	uint32_t	vs_total_sect;
/* 24*/	uint32_t	vs_fat32_length;
/* 28*/	uint16_t	vs_flags;
/* 2a*/	uint8_t		vs_version[2];
/* 2c*/	uint32_t	vs_root_cluster;
/* 30*/	uint16_t	vs_fsinfo_sector;
/* 32*/	uint16_t	vs_backup_boot;
/* 34*/	uint16_t	vs_reserved2[6];
/* 40*/	unsigned char	vs_drive_number;
/* 41*/	unsigned char	vs_boot_flags;
/* 42*/	unsigned char	vs_ext_boot_sign; /* 0x28 - without vs_label/vs_magic; 0x29 - with */
/* 43*/	unsigned char	vs_serno[4];
/* 47*/	unsigned char	vs_label[11];
/* 52*/	unsigned char   vs_magic[8];
/* 5a*/	unsigned char	vs_dummy2[420];
/*1fe*/	unsigned char	vs_pmagic[2];
} __attribute__((packed));
