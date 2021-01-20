// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package blockdevice provides a library for working with block devices.
package blockdevice

import "errors"

// ErrMissingPartitionTable indicates that the the block device does not have a
// partition table.
var ErrMissingPartitionTable = errors.New("missing partition table")

// OutOfSpaceError is implemented by out of space errors.
type OutOfSpaceError interface {
	OutOfSpaceError()
}

// IsOutOfSpaceError checks if provided error is 'out of space'.
func IsOutOfSpaceError(err error) bool {
	_, ok := err.(OutOfSpaceError) //nolint:errorlint

	return ok
}
