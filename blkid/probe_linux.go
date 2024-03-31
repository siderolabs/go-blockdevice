// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build linux

package blkid

import (
	"fmt"
	"io"
	"os"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/chain"
	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
)

type probeReader struct {
	io.ReaderAt

	sectorSize uint
	size       uint64
}

func (r *probeReader) GetSectorSize() uint {
	return r.sectorSize
}

func (r *probeReader) GetSize() uint64 {
	return r.size
}

func (i *Info) fillProbeResult(f *os.File) error {
	chain := chain.Default()

	res, matched, err := i.probe(f, chain, 0, i.Size)
	if err != nil {
		return fmt.Errorf("error probing: %w", err)
	}

	if res == nil {
		return nil
	}

	i.Name = matched.Name()
	i.UUID = res.UUID
	i.BlockSize = res.BlockSize
	i.FilesystemBlockSize = res.FilesystemBlockSize
	i.ProbedSize = res.ProbedSize
	i.Label = res.Label

	if err = i.fillNested(f, chain, 0, &i.Parts, res.Parts); err != nil {
		return fmt.Errorf("error probing nested: %w", err)
	}

	return nil
}

func (i *Info) fillNested(f *os.File, chain chain.Chain, offset uint64, out *[]NestedProbeResult, parts []probe.Partition) error {
	if len(parts) == 0 {
		return nil
	}

	*out = make([]NestedProbeResult, len(parts))

	for idx, part := range parts {
		(*out)[idx].PartitionIndex = part.Index
		(*out)[idx].PartitionUUID = part.UUID
		(*out)[idx].PartitionType = part.TypeUUID
		(*out)[idx].PartitionLabel = part.Label

		(*out)[idx].PartitionOffset = part.Offset
		(*out)[idx].PartitionSize = part.Size

		res, matched, err := i.probe(f, chain, offset+part.Offset, part.Size)
		if err != nil {
			return fmt.Errorf("error probing nested: %w", err)
		}

		if res == nil {
			continue
		}

		(*out)[idx].Name = matched.Name()
		(*out)[idx].UUID = res.UUID
		(*out)[idx].BlockSize = res.BlockSize
		(*out)[idx].FilesystemBlockSize = res.FilesystemBlockSize
		(*out)[idx].ProbedSize = res.ProbedSize
		(*out)[idx].Label = res.Label

		if err = i.fillNested(f, chain, offset+part.Offset, &(*out)[idx].Parts, res.Parts); err != nil {
			return fmt.Errorf("error probing nested: %w", err)
		}
	}

	return nil
}

func (i *Info) probe(f *os.File, chain chain.Chain, offset, length uint64) (*probe.Result, probe.Prober, error) {
	if offset+length > i.Size {
		return nil, nil, fmt.Errorf("probing range is out of bounds: offset %d + len %d > size %d", offset, length, i.Size)
	}

	if length < uint64(chain.MaxMagicSize()) {
		return nil, nil, fmt.Errorf("probing range is too small: len %d < max magic size %d", length, chain.MaxMagicSize())
	}

	magicReadSize := max(uint(chain.MaxMagicSize()), i.IOSize)

	if uint64(magicReadSize) > length {
		magicReadSize = uint(length)
	}

	buf := make([]byte, magicReadSize)

	_, err := f.ReadAt(buf, int64(offset))
	if err != nil {
		return nil, nil, fmt.Errorf("error reading magic buffer: %w", err)
	}

	pR := &probeReader{
		ReaderAt: io.NewSectionReader(f, int64(offset), int64(length)),

		sectorSize: i.SectorSize,
		size:       length,
	}

	for _, matched := range chain.MagicMatches(buf) {
		res, err := matched.Prober.Probe(pR, matched.Magic)
		if err != nil || res == nil {
			// skip failed probes
			continue
		}

		return res, matched.Prober, nil
	}

	return nil, nil, nil
}
