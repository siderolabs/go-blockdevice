// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package gptstructs provides encoded definitions for GPT on-disk structures.
package gptstructs

//go:generate go run ../cstruct/cstruct.go -pkg gptstructs -struct Header -input header.h -endianness LittleEndian

//go:generate go run ../cstruct/cstruct.go -pkg gptstructs -struct Entry -input entry.h -endianness LittleEndian

// NumEntries is the number of entries in the GPT.
const NumEntries = 128
