// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package luks2 contains LUKS header.
package luks2

//go:generate go run ../cstruct/cstruct.go -pkg luks2 -struct Luks2Header -input luks2_header.h -endianness BigEndian
