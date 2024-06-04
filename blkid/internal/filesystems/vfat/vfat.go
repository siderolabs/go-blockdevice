// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package vfat probes FAT12/FAT16/FAT32 filesystems.
package vfat

//go:generate go run ../../cstruct/cstruct.go -pkg vfat -struct MSDOSSB -input msdos.h -endianness LittleEndian

//go:generate go run ../../cstruct/cstruct.go -pkg vfat -struct VFATSB -input vfat.h -endianness LittleEndian

import (
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/utils"
)

var (
	fatMagic1 = magic.Magic{
		Offset: 0x52,
		Value:  []byte("MSWIN"),
	}

	fatMagic2 = magic.Magic{
		Offset: 0x52,
		Value:  []byte("FAT32   "),
	}

	fatMagic3 = magic.Magic{
		Offset: 0x36,
		Value:  []byte("MSDOS"),
	}

	fatMagic4 = magic.Magic{
		Offset: 0x36,
		Value:  []byte("FAT16   "),
	}

	fatMagic5 = magic.Magic{
		Offset: 0x36,
		Value:  []byte("FAT12   "),
	}

	fatMagic6 = magic.Magic{
		Offset: 0x36,
		Value:  []byte("FAT     "),
	}
)

// Probe for the filesystem.
type Probe struct{}

// Magic returns the magic value for the filesystem.
func (p *Probe) Magic() []*magic.Magic {
	return []*magic.Magic{
		&fatMagic1,
		&fatMagic2,
		&fatMagic3,
		&fatMagic4,
		&fatMagic5,
		&fatMagic6,
	}
}

// Name returns the name of the filesystem.
func (p *Probe) Name() string {
	return "vfat"
}

// Probe runs the further inspection and returns the result if successful.
func (p *Probe) Probe(r probe.Reader, _ magic.Magic) (*probe.Result, error) {
	vfatBuf := make([]byte, VFATSB_SIZE)
	msdosBuf := make([]byte, MSDOSSB_SIZE)

	if err := utils.ReadFullAt(r, vfatBuf, 0); err != nil {
		return nil, err
	}

	if err := utils.ReadFullAt(r, msdosBuf, 0); err != nil {
		return nil, err
	}

	vfatSB := VFATSB(vfatBuf)
	msdosSB := MSDOSSB(msdosBuf)

	if !isValid(msdosSB) {
		return nil, nil //nolint:nilnil
	}

	sectorCount := uint32(msdosSB.Get_ms_sectors())
	if sectorCount == 0 {
		sectorCount = msdosSB.Get_ms_total_sect()
	}

	sectorSize := uint32(msdosSB.Get_ms_sector_size())

	res := &probe.Result{
		BlockSize:           sectorSize,
		FilesystemBlockSize: uint32(vfatSB.Get_vs_cluster_size()) * sectorSize,
		ProbedSize:          uint64(sectorCount) * uint64(sectorSize),
	}

	return res, nil
}

func isValid(msdosSB MSDOSSB) bool {
	if msdosSB.Get_ms_fats() == 0 {
		return false
	}

	if msdosSB.Get_ms_reserved() == 0 {
		return false
	}

	if !(0xf8 <= msdosSB.Get_ms_media() || msdosSB.Get_ms_media() == 0xf0) {
		return false
	}

	if !utils.IsPowerOf2(msdosSB.Get_ms_cluster_size()) {
		return false
	}

	if !utils.IsPowerOf2(msdosSB.Get_ms_sector_size()) {
		return false
	}

	if msdosSB.Get_ms_sector_size() < 512 || msdosSB.Get_ms_sector_size() > 4096 {
		return false
	}

	return true
}
