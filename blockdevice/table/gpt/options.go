// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gpt

// Options is the functional options struct.
type Options struct {
	PrimaryGPT               bool
	PartitionEntriesStartLBA uint64
}

// Option is the functional option func.
type Option func(*Options)

// WithPrimaryGPT sets the contents of offset 24 in the GPT header to the location of the primary header.
func WithPrimaryGPT(o bool) Option {
	return func(args *Options) {
		args.PrimaryGPT = o
	}
}

// WithPartitionEntriesStartLBA  sets the LBA to be used for specifying which LBA should be used for the start of the partition entries.
func WithPartitionEntriesStartLBA(o uint64) Option {
	return func(args *Options) {
		args.PartitionEntriesStartLBA = o
	}
}

// NewDefaultOptions initializes a Options struct with default values.
func NewDefaultOptions(setters ...interface{}) *Options {
	opts := &Options{
		PrimaryGPT:               true,
		PartitionEntriesStartLBA: 2048,
	}

	for _, setter := range setters {
		if s, ok := setter.(Option); ok {
			s(opts)
		}
	}

	return opts
}
