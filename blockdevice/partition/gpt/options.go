// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gpt

import "fmt"

// Options is the functional options struct.
type Options struct {
	PartitionEntriesStartLBA uint64
	MarkMBRBootable          bool
}

// Option is the functional option func.
type Option func(*Options) error

// WithPartitionEntriesStartLBA  sets the LBA to be used for specifying which LBA should be used for the start of the partition entries.
func WithPartitionEntriesStartLBA(o uint64) Option {
	return func(args *Options) error {
		if o < 2 {
			return fmt.Errorf("partition entries start LBA must be greater or equal than 2")
		}

		args.PartitionEntriesStartLBA = o

		return nil
	}
}

// WithMarkMBRBootable marks MBR partition as bootable.
func WithMarkMBRBootable(value bool) Option {
	return func(args *Options) error {
		args.MarkMBRBootable = value

		return nil
	}
}

// NewDefaultOptions initializes an `Options` struct with default values.
func NewDefaultOptions(setters ...Option) (*Options, error) {
	opts := &Options{
		PartitionEntriesStartLBA: 2,
	}

	for _, setter := range setters {
		if err := setter(opts); err != nil {
			return nil, err
		}
	}

	return opts, nil
}
