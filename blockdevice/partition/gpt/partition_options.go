// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gpt

import (
	"github.com/google/uuid"
)

// PartitionOptions represent the options available for partitions.
//
//nolint:govet
type PartitionOptions struct {
	Type        uuid.UUID
	Name        string
	Offset      uint64
	MaximumSize bool
	Attibutes   uint64
}

// PartitionOption is the functional option func.
type PartitionOption func(*PartitionOptions)

// WithPartitionType sets the partition type.
func WithPartitionType(id string) PartitionOption {
	return func(args *PartitionOptions) {
		// TODO: An Option should return an error.
		//nolint: errcheck
		guuid, _ := uuid.Parse(id)
		args.Type = guuid
	}
}

// WithPartitionName sets the partition name.
func WithPartitionName(o string) PartitionOption {
	return func(args *PartitionOptions) {
		args.Name = o
	}
}

// WithOffset sets partition start offset in bytes.
func WithOffset(o uint64) PartitionOption {
	return func(args *PartitionOptions) {
		args.Offset = o
	}
}

// WithMaximumSize indicates if the partition should be created with the maximum size possible.
func WithMaximumSize(o bool) PartitionOption {
	return func(args *PartitionOptions) {
		args.MaximumSize = o
	}
}

// WithLegacyBIOSBootableAttribute marks the partition as bootable.
func WithLegacyBIOSBootableAttribute(o bool) PartitionOption {
	return func(args *PartitionOptions) {
		if o {
			args.Attibutes |= (1 << 2)
		}
	}
}

// NewDefaultPartitionOptions initializes a Options struct with default values.
func NewDefaultPartitionOptions(setters ...PartitionOption) *PartitionOptions {
	// TODO: An Option should return an error.
	//nolint: errcheck
	guuid, _ := uuid.Parse("0FC63DAF-8483-4772-8E79-3D69D8477DE4")

	opts := &PartitionOptions{
		Type:   guuid,
		Name:   "",
		Offset: 0,
	}

	for _, setter := range setters {
		setter(opts)
	}

	return opts
}
