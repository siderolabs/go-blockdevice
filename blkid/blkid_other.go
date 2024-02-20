// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build !linux

package blkid

import (
	"fmt"
	"os"
)

// ProbePath returns the probe information for the specified path.
func ProbePath(devpath string) (*Info, error) {
	return nil, fmt.Errorf("not implemented")
}

// Probe returns the probe information for the specified file.
func Probe(f *os.File) (*Info, error) {
	return nil, fmt.Errorf("not implemented")
}
