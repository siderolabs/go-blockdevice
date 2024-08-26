// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gpt

import "github.com/google/uuid"

// Options is a set of options for creating a new partition table.
type Options struct {
	SkipPMBR         bool
	MarkPMBRBootable bool

	// Number of LBAs to skip before the writing partition entries.
	SkipLBAs uint

	// DiskGUID is a GUID for the disk.
	//
	// If not set, on partition table creation, a new GUID is generated.
	DiskGUID uuid.UUID
}

// Option is a function that sets some option.
type Option func(*Options)

// WithSkipPMBR is an option to skip writing protective MBR.
func WithSkipPMBR() Option {
	return func(o *Options) {
		o.SkipPMBR = true
	}
}

// WithMarkPMBRBootable is an option to mark protective MBR bootable.
func WithMarkPMBRBootable() Option {
	return func(o *Options) {
		o.MarkPMBRBootable = true
	}
}

// WithSkipLBAs is an option to skip writing partition entries.
func WithSkipLBAs(n uint) Option {
	return func(o *Options) {
		o.SkipLBAs = n
	}
}

// WithDiskGUID is an option to set disk GUID.
func WithDiskGUID(guid uuid.UUID) Option {
	return func(o *Options) {
		o.DiskGUID = guid
	}
}

// PartitionOptions configure a partition.
type PartitionOptions struct {
	UniqueGUID uuid.UUID
	Flags      uint64
}

// PartitionOption is a function that sets some option.
type PartitionOption func(*PartitionOptions)

// WithUniqueGUID is an option to set a unique GUID for the partition.
func WithUniqueGUID(guid uuid.UUID) PartitionOption {
	return func(o *PartitionOptions) {
		o.UniqueGUID = guid
	}
}

// WithLegacyBIOSBootableAttribute marks the partition as bootable.
func WithLegacyBIOSBootableAttribute(val bool) PartitionOption {
	return func(args *PartitionOptions) {
		if val {
			args.Flags |= (1 << 2)
		}
	}
}
