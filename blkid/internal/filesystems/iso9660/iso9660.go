// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package iso9660 probes ISO9660 filesystems.
package iso9660

//go:generate go run ../../../../internal/cstruct/cstruct.go -pkg iso9660 -struct VolumeDescriptor -input volume.h -endianness NativeEndian

import (
	"strings"

	"github.com/siderolabs/go-pointer"
	"golang.org/x/text/encoding/unicode"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/partitions/gpt"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
)

const (
	superblockOffset = 0x8000
)

var isoMagic = magic.Magic{
	Offset: superblockOffset + 1,
	Value:  []byte("CD001"),
}

// Probe for the filesystem.
type Probe struct{}

// Magic returns the magic value for the filesystem.
func (p *Probe) Magic() []*magic.Magic {
	return []*magic.Magic{&isoMagic}
}

// Name returns the name of the filesystem.
func (p *Probe) Name() string {
	return "iso9660"
}

const (
	vdMax           = 16
	vdEnd           = 0xff
	vdBootRecord    = 0
	vdPrimary       = 1
	vdSupplementary = 2

	sectorSize = 2048
)

func isonum16(b []byte) uint16 {
	return uint16(b[0]) | uint16(b[1])<<8
}

// Probe runs the further inspection and returns the result if successful.
func (p *Probe) Probe(r probe.Reader, _ magic.Magic) (*probe.Result, error) {
	var pvd, joilet VolumeDescriptor

vdLoop:
	for i := range vdMax {
		buf := make([]byte, VOLUMEDESCRIPTOR_SIZE)

		if _, err := r.ReadAt(buf, superblockOffset+sectorSize*int64(i)); err != nil {
			break
		}

		vd := VolumeDescriptor(buf)

		switch vd.Get_vd_type() {
		case vdEnd:
			break vdLoop
		case vdBootRecord:
			// skip
		case vdPrimary:
			pvd = vd
		case vdSupplementary:
			joilet = vd
		}

		if pvd != nil && joilet != nil {
			break
		}
	}

	if pvd == nil {
		return nil, nil //nolint:nilnil
	}

	logicalBlockSize := isonum16(pvd.Get_logical_block_size())
	spaceSize := isonum16(pvd.Get_space_size())

	res := &probe.Result{
		BlockSize:           uint32(logicalBlockSize),
		FilesystemBlockSize: uint32(logicalBlockSize),
		ProbedSize:          uint64(spaceSize) * uint64(logicalBlockSize),
	}

	if joilet != nil {
		lblBytes := joilet.Get_volume_id()

		if label, err := unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder().Bytes(lblBytes); err == nil {
			res.Label = pointer.To(strings.TrimRight(string(label), " \000"))
		}
	}

	if res.Label == nil {
		lblBytes := pvd.Get_volume_id()
		res.Label = pointer.To(strings.TrimRight(string(lblBytes), " \000"))
	}

	probeHybridGPT(r, res)

	return res, nil
}

// hybridGPTSectorSize is the sector size of the GPT in hybrid ISO images (e.g. built with xorriso),
// which doesn't depend on the block size of the device the image is on (e.g. 2048 for a CD-ROM).
const hybridGPTSectorSize = 512

// sectorSizeReader overrides the sector size of the underlying reader.
type sectorSizeReader struct {
	probe.Reader

	sectorSize uint
}

func (r sectorSizeReader) GetSectorSize() uint {
	return r.sectorSize
}

// probeHybridGPT probes the GPT embedded into a hybrid ISO image, and adds its partitions to the result.
//
// The ISO9660 filesystem doesn't use the first 32KiB of the image (the system area),
// which is where hybrid images put a (protective) MBR and a GPT, e.g. to expose the EFI System Partition.
// The Linux kernel doesn't scan partitions on CD-ROM devices, so this is the only way to discover them.
//
// Errors are ignored, as a broken GPT shouldn't prevent the ISO9660 filesystem from being detected.
func probeHybridGPT(r probe.Reader, res *probe.Result) {
	sectorSizes := []uint{r.GetSectorSize()}

	if sectorSizes[0] != hybridGPTSectorSize {
		sectorSizes = append(sectorSizes, hybridGPTSectorSize)
	}

	for _, sectorSize := range sectorSizes {
		gptRes, err := (&gpt.Probe{}).Probe(sectorSizeReader{Reader: r, sectorSize: sectorSize}, magic.Magic{})
		if err != nil || gptRes == nil {
			continue
		}

		res.Parts = gptRes.Parts
		res.ExtraSignatures = append(res.ExtraSignatures, gptRes.ExtraSignatures...)

		return
	}
}
