// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package gptutil implements helper functions for GPT tables.
package gptutil

// DiskSizer is an interface for block devices that can provide their sector size and total size.
type DiskSizer interface {
	GetSectorSize() uint
	GetSize() uint64
}

// LastLBA returns the last logical block address of the device.
func LastLBA(r DiskSizer) (uint64, bool) {
	sectorSize := r.GetSectorSize()
	size := r.GetSize()

	if uint64(sectorSize) > size {
		return 0, false
	}

	return (size / uint64(sectorSize)) - 1, true
}

// GUIDToUUID converts a GPT GUID to a UUID.
func GUIDToUUID(g []byte) []byte {
	return append(
		[]byte{
			g[3], g[2], g[1], g[0],
			g[5], g[4],
			g[7], g[6],
			g[8], g[9],
		},
		g[10:16]...,
	)
}

// UUIDToGUID converts a UUID to a GPT GUID.
func UUIDToGUID(u []byte) []byte {
	return append(
		[]byte{
			u[3], u[2], u[1], u[0],
			u[5], u[4],
			u[7], u[6],
			u[8], u[9],
		},
		u[10:16]...,
	)
}
