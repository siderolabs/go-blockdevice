// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package blkid provides information about blockdevice filesystem types and IDs.
package blkid

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/siderolabs/go-blockdevice/v2/blkid/internal/probe"
	"github.com/siderolabs/go-blockdevice/v2/block"
)

// Common errors.
var (
	ErrFailedLock = errors.New("failed to acquire shared lock while probing blockdevice")
)

// DefaultLockTimeout is the default time to wait for the shared lock on the blockdevice.
//
// The lock is most often contended by udev, which takes an exclusive lock on the whole disk while it
// processes a uevent, so a device which was just written to is briefly unprobeable.
const DefaultLockTimeout = 5 * time.Second

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
type ProbeResult struct {
	Name  string
	UUID  *uuid.UUID
	Label *string

	SignatureRanges []SignatureRange

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

// SignatureRange is a range of bytes for signature detection.
type SignatureRange = probe.SignatureRange

// ProbeOptions is the options for probing.
type ProbeOptions struct {
	// Logger to use for logging.
	Logger *zap.Logger
	// LockTimeout is the time to wait for the shared lock on the blockdevice.
	//
	// Zero means a single attempt with no waiting.
	LockTimeout time.Duration
	// SkipLocking blockdevices in shared mode.
	SkipLocking bool
	// SectorSize is the sector size to use for probing.
	SectorSize uint
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

// WithLockTimeout sets the time to wait for the shared lock on the blockdevice.
//
// The lock is retried until the timeout expires, and ErrFailedLock is returned if it can't be
// acquired in time. Zero means a single attempt, failing immediately if the device is locked.
func WithLockTimeout(timeout time.Duration) ProbeOption {
	return func(o *ProbeOptions) {
		o.LockTimeout = timeout
	}
}

// WithSectorSize sets the sector size to use for probing.
//
// This is useful when probing a file that is not a blockdevice.
func WithSectorSize(sectorSize uint) ProbeOption {
	return func(o *ProbeOptions) {
		o.SectorSize = sectorSize
	}
}

func applyProbeOptions(opts ...ProbeOption) ProbeOptions {
	o := ProbeOptions{
		Logger:      zap.NewNop(),
		SectorSize:  block.DefaultBlockSize,
		LockTimeout: DefaultLockTimeout,
	}

	for _, opt := range opts {
		opt(&o)
	}

	return o
}
