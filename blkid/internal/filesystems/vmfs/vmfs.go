// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package vmfs probes VMware VMFS volumes and filesystems.
//
// VMFS is not usable on Linux, but the signatures are left behind on disks
// previously used as ESXi datastores, and other tools (e.g. mkfs.xfs) refuse
// to overwrite them, so they should be detected (and wiped).
package vmfs

//go:generate go run ../../../../internal/cstruct/cstruct.go -pkg vmfs -struct VolumeInfo -input vmfs_volume.h -endianness LittleEndian
//go:generate go run ../../../../internal/cstruct/cstruct.go -pkg vmfs -struct FSInfo -input vmfs_fs.h -endianness LittleEndian

import (
	"bytes"

	"github.com/siderolabs/go-pointer"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/magic"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
)

// https://github.com/util-linux/util-linux/blob/8179f69eec2f04be47607d89349505ab7d6ec6be/libblkid/src/superblocks/vmfs.c#L79-L101
const (
	volumeInfoOffset = 1024 * 1024
	fsInfoOffset     = 2048 * 1024

	volumeMagic = 0xc001d00d
	fsMagic     = 0x2fabf15e

	magicSize = 4
)

// nullMagic matches always: the magic values are located at 1 MiB/2 MiB offsets,
// and expressing them as fixed magics would make every probe read 2 MiB.
var nullMagic = magic.Magic{}

// VolumeProbe for the VMFS volume (LVM) member.
type VolumeProbe struct{}

// Magic returns the magic value for the VMFS volume.
func (p *VolumeProbe) Magic() []*magic.Magic {
	return []*magic.Magic{&nullMagic}
}

// Name returns the name of the VMFS volume.
func (p *VolumeProbe) Name() string {
	return "vmfs_volume_member"
}

// Probe runs the further inspection and returns the result if successful.
func (p *VolumeProbe) Probe(r probe.Reader, _ magic.Magic) (*probe.Result, error) {
	if r.GetSize() < volumeInfoOffset+VOLUMEINFO_SIZE {
		return nil, nil //nolint:nilnil
	}

	buf := make([]byte, VOLUMEINFO_SIZE)

	if _, err := r.ReadAt(buf, volumeInfoOffset); err != nil {
		return nil, err
	}

	hdr := VolumeInfo(buf)

	if hdr.Get_magic() != volumeMagic {
		return nil, nil //nolint:nilnil
	}

	res := &probe.Result{
		ExtraSignatures: []probe.SignatureRange{
			{Offset: volumeInfoOffset, Size: magicSize},
		},
	}

	// VMFS volume carries the filesystem descriptor, so report its signature as well,
	// otherwise wiping the volume signature reveals the filesystem one.
	fsRes, err := (&FSProbe{}).Probe(r, nullMagic)
	if err != nil {
		return nil, err
	}

	if fsRes != nil {
		res.ExtraSignatures = append(res.ExtraSignatures, fsRes.ExtraSignatures...)
	}

	return res, nil
}

// FSProbe for the VMFS filesystem.
type FSProbe struct{}

// Magic returns the magic value for the VMFS filesystem.
func (p *FSProbe) Magic() []*magic.Magic {
	return []*magic.Magic{&nullMagic}
}

// Name returns the name of the VMFS filesystem.
func (p *FSProbe) Name() string {
	return "vmfs"
}

// Probe runs the further inspection and returns the result if successful.
func (p *FSProbe) Probe(r probe.Reader, _ magic.Magic) (*probe.Result, error) {
	if r.GetSize() < fsInfoOffset+FSINFO_SIZE {
		return nil, nil //nolint:nilnil
	}

	buf := make([]byte, FSINFO_SIZE)

	if _, err := r.ReadAt(buf, fsInfoOffset); err != nil {
		return nil, err
	}

	hdr := FSInfo(buf)

	if hdr.Get_magic() != fsMagic {
		return nil, nil //nolint:nilnil
	}

	res := &probe.Result{
		ExtraSignatures: []probe.SignatureRange{
			{Offset: fsInfoOffset, Size: magicSize},
		},
	}

	// VMFS UUID is not reported, as libblkid formats it in a non-RFC 4122 way (8-8-4-12).

	if lbl := hdr.Get_label(); lbl[0] != 0 {
		idx := bytes.IndexByte(lbl, 0)
		if idx == -1 {
			idx = len(lbl)
		}

		res.Label = pointer.To(string(bytes.TrimRight(lbl[:idx], " ")))
	}

	return res, nil
}
