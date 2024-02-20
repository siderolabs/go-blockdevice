/* PVD - Primary volume descriptor */
struct iso_volume_descriptor {
	/* High Sierra has 8 bytes before descriptor with Volume Descriptor LBN value, those are skipped by blkid_probe_get_buffer() */
	unsigned char	vd_type;
	unsigned char	vd_id[5];
	unsigned char	vd_version;
	unsigned char	flags;
	unsigned char	system_id[32];
	unsigned char	volume_id[32];
	unsigned char	unused[8];
	unsigned char	space_size[8];
	unsigned char	escape_sequences[32];
	unsigned char  set_size[4];
	unsigned char  vol_seq_num[4];
	unsigned char  logical_block_size[4];
	unsigned char  path_table_size[8];
	union {
		struct {
			unsigned char type_l_path_table[4];
			unsigned char opt_type_l_path_table[4];
			unsigned char type_m_path_table[4];
			unsigned char opt_type_m_path_table[4];
			unsigned char root_dir_record[34];
			unsigned char volume_set_id[128];
			unsigned char publisher_id[128];
			unsigned char data_preparer_id[128];
			unsigned char application_id[128];
			unsigned char copyright_file_id[37];
			unsigned char abstract_file_id[37];
			unsigned char bibliographic_file_id[37];
			unsigned char created[17];
			unsigned char modified[17];
			unsigned char expiration[17];
			unsigned char effective[17];
			unsigned char std_version;
		} iso; /* ISO9660 */
	};
} __attribute__((packed));
