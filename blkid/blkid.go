// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package blkid provides information about blockdevice filesystem types and IDs.
package blkid

import (
	"errors"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/siderolabs/go-blockdevice/v2/block"
)

// Common errors.
var (
	ErrFailedLock = errors.New("failed to acquire shared lock while probing blockdevice")
)

// Info represents the result of the probe.
type Info struct { //nolint:govet
	// Link to the block device, only if the probed file is a blockdevice.
	BlockDevice *block.Device

	// DevNo is the device number of the probed device.
	//
	// Only available if the probed file is a blockdevice.
	DevNo uint64

	// WholeDisk is true if the probed device is a whole disk.
	//
	// Only available if the probed file is a blockdevice.
	WholeDisk bool

	// Overall size of the probed device (in bytes).
	Size uint64

	// Sector size of the device (in bytes).
	SectorSize uint

	// Optimal I/O size for the device (in bytes).
	IOSize uint

	// ProbeResult is the result of probing the device.
	ProbeResult

	// Parts is the result of probing the nested filesystem/partitions.
	Parts []NestedProbeResult
}

// ProbeResult is a result of probing a single filesystem/partition.
type ProbeResult struct { //nolint:govet
	Name  string
	UUID  *uuid.UUID
	Label *string

	BlockSize           uint32
	FilesystemBlockSize uint32
	ProbedSize          uint64
}

// NestedResult is result of probing a nested filesystem/partition.
//
// It annotates the ProbeResult with the partition information.
type NestedResult struct {
	PartitionUUID  *uuid.UUID
	PartitionType  *uuid.UUID
	PartitionLabel *string
	PartitionIndex uint // 1-based index

	PartitionOffset, PartitionSize uint64
}

// NestedProbeResult is a result of probing a nested filesystem/partition.
type NestedProbeResult struct { //nolint:govet
	NestedResult
	ProbeResult

	Parts []NestedProbeResult
}

// ProbeOptions is the options for probing.
type ProbeOptions struct {
	// Logger to use for logging.
	Logger *zap.Logger
	// SkipLocking blockdevices in shared mode.
	SkipLocking bool
}

// ProbeOption is an option for probing.
type ProbeOption func(*ProbeOptions)

// WithProbeLogger sets the logger for the probe.
func WithProbeLogger(logger *zap.Logger) ProbeOption {
	return func(o *ProbeOptions) {
		o.Logger = logger
	}
}

// WithSkipLocking skips locking blockdevices in shared mode.
func WithSkipLocking(skip bool) ProbeOption {
	return func(o *ProbeOptions) {
		o.SkipLocking = skip
	}
}

func applyProbeOptions(opts ...ProbeOption) ProbeOptions {
	o := ProbeOptions{
		Logger: zap.NewNop(),
	}

	for _, opt := range opts {
		opt(&o)
	}

	return o
}
