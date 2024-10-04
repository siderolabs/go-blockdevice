// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Code generated by "cstruct -pkg zfs -struct ZFSUB -input zfs.h -endianness LittleEndian"; DO NOT EDIT.

package zfs

import "encoding/binary"

var _ = binary.LittleEndian

// ZFSUB is a byte slice representing the zfs.h C header.
type ZFSUB []byte

// Get_ub_magic returns UBERBLOCK_MAGIC.
func (s ZFSUB) Get_ub_magic() uint64 {
	return binary.LittleEndian.Uint64(s[0:8])
}

// Put_ub_magic sets UBERBLOCK_MAGIC.
func (s ZFSUB) Put_ub_magic(v uint64) {
	binary.LittleEndian.PutUint64(s[0:8], v)
}

// Get_ub_version returns SPA_VERSION.
func (s ZFSUB) Get_ub_version() uint64 {
	return binary.LittleEndian.Uint64(s[8:16])
}

// Put_ub_version sets SPA_VERSION.
func (s ZFSUB) Put_ub_version(v uint64) {
	binary.LittleEndian.PutUint64(s[8:16], v)
}

// Get_ub_txg returns txg of last sync.
func (s ZFSUB) Get_ub_txg() uint64 {
	return binary.LittleEndian.Uint64(s[16:24])
}

// Put_ub_txg sets txg of last sync.
func (s ZFSUB) Put_ub_txg(v uint64) {
	binary.LittleEndian.PutUint64(s[16:24], v)
}

// Get_ub_guid_sum returns sum of all vdev guids.
func (s ZFSUB) Get_ub_guid_sum() uint64 {
	return binary.LittleEndian.Uint64(s[24:32])
}

// Put_ub_guid_sum sets sum of all vdev guids.
func (s ZFSUB) Put_ub_guid_sum(v uint64) {
	binary.LittleEndian.PutUint64(s[24:32], v)
}

// Get_ub_timestamp returns UTC time of last sync.
func (s ZFSUB) Get_ub_timestamp() uint64 {
	return binary.LittleEndian.Uint64(s[32:40])
}

// Put_ub_timestamp sets UTC time of last sync.
func (s ZFSUB) Put_ub_timestamp(v uint64) {
	binary.LittleEndian.PutUint64(s[32:40], v)
}

// Get_ub_rootbp returns MOS objset_phys_t.
func (s ZFSUB) Get_ub_rootbp() byte {
	return s[40]
}

// Put_ub_rootbp sets MOS objset_phys_t.
func (s ZFSUB) Put_ub_rootbp(v byte) {
	s[40] = v
}

// ZFSUB_SIZE is the size of the ZFSUB struct.
const ZFSUB_SIZE = 41