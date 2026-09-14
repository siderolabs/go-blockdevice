// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package dmioctl

import (
	"bytes"
	"fmt"
	"strings"
)

// alignment of the target records which follow the header in a payload.
const alignment = 8

// Target is a single target of a device-mapper table.
//
// The sector counts are in 512-byte sectors, the unit device-mapper tables always use.
type Target struct {
	// Type of the target, e.g. "linear".
	Type string
	// Params of the target, in the format the target type defines, e.g. "8:0 2048" for "linear".
	Params string
	// SectorStart is the offset of the target within the device being mapped.
	SectorStart uint64
	// Length of the target in sectors.
	Length uint64
}

// Payload describes the argument of a device-mapper ioctl.
type Payload struct {
	// Name of the device to operate on; may be empty if UUID is set.
	Name string
	// UUID of the device to operate on; may be empty if Name is set.
	//
	// The kernel looks a device up by UUID in preference to the name.
	UUID string
	// Targets of the table, for TableLoadRequest; empty for every other command.
	Targets []Target
	// Flags of the command.
	Flags uint32
	// MinSize is the minimum size of the buffer to allocate.
	//
	// The kernel writes its reply into the same buffer, so a command which returns data needs
	// room for it beyond the header.
	MinSize int
}

// Build assembles the payload into a buffer to hand to the ioctl.
func (p Payload) Build() ([]byte, error) {
	if len(p.Name) >= NameLen {
		return nil, fmt.Errorf("device name %q is too long: %d bytes, limit is %d", p.Name, len(p.Name), NameLen-1)
	}

	if len(p.UUID) >= UUIDLen {
		return nil, fmt.Errorf("device UUID %q is too long: %d bytes, limit is %d", p.UUID, len(p.UUID), UUIDLen-1)
	}

	if strings.ContainsRune(p.Name, 0) || strings.ContainsRune(p.UUID, 0) {
		return nil, fmt.Errorf("device name %q and UUID %q may not contain a NUL byte", p.Name, p.UUID)
	}

	size := HEADER_SIZE

	for _, target := range p.Targets {
		recordSize, err := target.recordSize()
		if err != nil {
			return nil, err
		}

		size += recordSize
	}

	buf := make([]byte, max(size, p.MinSize))

	header := Header(buf)
	header.Put_version_major(VersionMajor)
	header.Put_version_minor(VersionMinor)
	header.Put_version_patch(VersionPatch)
	header.Put_data_size(uint32(len(buf)))
	header.Put_data_start(HEADER_SIZE)
	header.Put_target_count(uint32(len(p.Targets)))
	header.Put_flags(p.Flags)
	header.Put_name([]byte(p.Name))
	header.Put_uuid([]byte(p.UUID))

	offset := HEADER_SIZE

	for _, target := range p.Targets {
		recordSize, err := target.recordSize()
		if err != nil {
			return nil, err
		}

		spec := Spec(buf[offset:])
		spec.Put_sector_start(target.SectorStart)
		spec.Put_length(target.Length)
		// 'next' is the offset from the start of this record to the start of the next one,
		// which is to say the length of this record
		spec.Put_next(uint32(recordSize))
		spec.Put_target_type([]byte(target.Type))

		copy(buf[offset+SPEC_SIZE:], target.Params)

		offset += recordSize
	}

	return buf, nil
}

// recordSize returns the size of the target record: the target spec, the NUL-terminated
// parameters, and padding to the alignment the kernel expects.
func (t Target) recordSize() (int, error) {
	if len(t.Type) >= TypeLen {
		return 0, fmt.Errorf("target type %q is too long: %d bytes, limit is %d", t.Type, len(t.Type), TypeLen-1)
	}

	if strings.ContainsRune(t.Type, 0) || strings.ContainsRune(t.Params, 0) {
		return 0, fmt.Errorf("target type %q and params %q may not contain a NUL byte", t.Type, t.Params)
	}

	size := SPEC_SIZE + len(t.Params) + 1

	return (size + alignment - 1) & ^(alignment - 1), nil
}

// NameListEntry is a single device of the reply to ListDevicesRequest.
type NameListEntry struct {
	// Name of the device.
	Name string
	// Dev is the device number, as major:minor packed by the kernel.
	Dev uint64
}

// ParseNameList parses the reply to ListDevicesRequest.
//
// The reply is a chain of 'struct dm_name_list' records: the device number, the offset to the next
// record from the start of this one (zero on the last record), and the NUL-terminated name.
func ParseNameList(payload []byte) ([]NameListEntry, error) {
	if len(payload) < HEADER_SIZE {
		return nil, fmt.Errorf("payload is too short: %d bytes", len(payload))
	}

	header := Header(payload)

	dataStart, dataSize := int(header.Get_data_start()), int(header.Get_data_size())

	if dataStart < HEADER_SIZE || dataSize > len(payload) || dataStart > dataSize {
		return nil, fmt.Errorf("invalid reply bounds: data_start %d, data_size %d, payload %d bytes", dataStart, dataSize, len(payload))
	}

	var result []NameListEntry

	for offset := dataStart; offset+NAMELIST_SIZE <= dataSize; {
		record := NameList(payload[offset:dataSize])

		if record.Get_dev() == 0 {
			// the kernel zeroes the device number of the first record to flag an empty list
			break
		}

		name, _, ok := bytes.Cut(payload[offset+NAMELIST_SIZE:dataSize], []byte{0})
		if !ok {
			return nil, fmt.Errorf("unterminated device name at offset %d", offset)
		}

		result = append(result, NameListEntry{
			Dev:  record.Get_dev(),
			Name: string(name),
		})

		next := int(record.Get_next())
		if next == 0 {
			break
		}

		// the record the offset points at has to fit into the reply: stopping on one which
		// doesn't would return an incomplete device list as if it were the whole of it
		if next < NAMELIST_SIZE || next > dataSize-offset-NAMELIST_SIZE {
			return nil, fmt.Errorf("invalid record length %d at offset %d", next, offset)
		}

		offset += next
	}

	return result, nil
}

// CString returns the NUL-terminated string at the start of the buffer.
func CString(buf []byte) string {
	name, _, _ := bytes.Cut(buf, []byte{0})

	return string(name)
}

// DecodeDev converts a device number as the ioctl interface reports it into major and minor numbers.
//
// The kernel packs it with new_encode_dev(): the low 8 bits of the minor, then the major, then the
// rest of the minor, which is not the layout of a dev_t.
func DecodeDev(dev uint64) (major, minor uint32) {
	major = uint32((dev >> 8) & 0xfff)
	minor = uint32((dev & 0xff) | ((dev >> 12) & 0xfffff00))

	return major, minor
}
