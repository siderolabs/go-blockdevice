// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package dmioctl_test

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/siderolabs/go-blockdevice/v2/internal/dmioctl"
)

func TestPayloadBuildEmpty(t *testing.T) {
	t.Parallel()

	payload, err := dmioctl.Payload{}.Build()
	require.NoError(t, err)

	assert.Len(t, payload, dmioctl.HEADER_SIZE)

	header := dmioctl.Header(payload)

	assert.EqualValues(t, dmioctl.VersionMajor, header.Get_version_major())
	assert.EqualValues(t, dmioctl.VersionMinor, header.Get_version_minor())
	assert.EqualValues(t, dmioctl.VersionPatch, header.Get_version_patch())
	assert.EqualValues(t, dmioctl.HEADER_SIZE, header.Get_data_size())
	assert.EqualValues(t, dmioctl.HEADER_SIZE, header.Get_data_start())
	assert.EqualValues(t, 0, header.Get_target_count())
	assert.EqualValues(t, 0, header.Get_flags())
	assert.Empty(t, dmioctl.CString(header.Get_name()))
	assert.Empty(t, dmioctl.CString(header.Get_uuid()))
}

func TestPayloadBuildDeviceID(t *testing.T) {
	t.Parallel()

	payload, err := dmioctl.Payload{
		Name:  "mpatha-part1",
		UUID:  "part1-mpath-36005076810800567c8000000000009fb",
		Flags: dmioctl.SuspendFlag,
	}.Build()
	require.NoError(t, err)

	header := dmioctl.Header(payload)

	assert.Equal(t, "mpatha-part1", dmioctl.CString(header.Get_name()))
	assert.Equal(t, "part1-mpath-36005076810800567c8000000000009fb", dmioctl.CString(header.Get_uuid()))
	assert.EqualValues(t, dmioctl.SuspendFlag, header.Get_flags())

	// the fields are fixed-size and NUL-padded, so nothing may be left beyond the terminator
	assert.Equal(t, make([]byte, dmioctl.NameLen-len("mpatha-part1")-1), header.Get_name()[len("mpatha-part1")+1:])
}

func TestPayloadBuildMinSize(t *testing.T) {
	t.Parallel()

	payload, err := dmioctl.Payload{MinSize: 16 * 1024}.Build()
	require.NoError(t, err)

	assert.Len(t, payload, 16*1024)

	// the kernel is told how much room it has by data_size
	assert.EqualValues(t, 16*1024, dmioctl.Header(payload).Get_data_size())

	// a MinSize below the header size doesn't shrink the payload
	payload, err = dmioctl.Payload{MinSize: 8}.Build()
	require.NoError(t, err)

	assert.Len(t, payload, dmioctl.HEADER_SIZE)
}

func TestPayloadBuildTargets(t *testing.T) {
	t.Parallel()

	const (
		params1 = "7:0 2048"   // 8 bytes, so the record is 40 + 8 + 1 = 49, padded to 56
		params2 = "7:0 123456" // 10 bytes, so the record is 40 + 10 + 1 = 51, padded to 56
	)

	payload, err := dmioctl.Payload{
		Name: "mpatha-part1",
		Targets: []dmioctl.Target{
			{Type: "linear", Params: params1, SectorStart: 0, Length: 2048},
			{Type: "linear", Params: params2, SectorStart: 2048, Length: 4096},
		},
	}.Build()
	require.NoError(t, err)

	assert.Len(t, payload, dmioctl.HEADER_SIZE+56+56)
	assert.EqualValues(t, 2, dmioctl.Header(payload).Get_target_count())
	assert.EqualValues(t, len(payload), dmioctl.Header(payload).Get_data_size())

	for _, test := range []struct {
		params string
		typ    string

		offset      int
		sectorStart uint64
		length      uint64
		next        uint32
	}{
		{offset: dmioctl.HEADER_SIZE, sectorStart: 0, length: 2048, typ: "linear", params: params1, next: 56},
		{offset: dmioctl.HEADER_SIZE + 56, sectorStart: 2048, length: 4096, typ: "linear", params: params2, next: 56},
	} {
		spec := dmioctl.Spec(payload[test.offset:])

		assert.Equal(t, test.sectorStart, spec.Get_sector_start())
		assert.Equal(t, test.length, spec.Get_length())
		assert.Equal(t, test.next, spec.Get_next())
		assert.Equal(t, test.typ, dmioctl.CString(spec.Get_target_type()))

		// the parameters follow the spec as a NUL-terminated string
		assert.Equal(t, test.params, dmioctl.CString(payload[test.offset+dmioctl.SPEC_SIZE:]))
	}
}

func TestPayloadBuildTargetPadding(t *testing.T) {
	t.Parallel()

	// the kernel walks the records by the 'next' offset of each, which has to stay 8-byte aligned
	for _, test := range []struct {
		params string

		expectedNext uint32
	}{
		{params: "", expectedNext: 48},                      // 40 + 0 + 1 = 41 -> 48
		{params: strings.Repeat("x", 7), expectedNext: 48},  // 40 + 7 + 1 = 48 -> 48
		{params: strings.Repeat("x", 8), expectedNext: 56},  // 40 + 8 + 1 = 49 -> 56
		{params: strings.Repeat("x", 15), expectedNext: 56}, // 40 + 15 + 1 = 56 -> 56
		{params: strings.Repeat("x", 16), expectedNext: 64}, // 40 + 16 + 1 = 57 -> 64
	} {
		t.Run(test.params, func(t *testing.T) {
			t.Parallel()

			payload, err := dmioctl.Payload{
				Targets: []dmioctl.Target{{Type: "linear", Params: test.params}},
			}.Build()
			require.NoError(t, err)

			assert.Len(t, payload, dmioctl.HEADER_SIZE+int(test.expectedNext))
			assert.Equal(t, test.expectedNext, dmioctl.Spec(payload[dmioctl.HEADER_SIZE:]).Get_next())
		})
	}
}

func TestPayloadBuildErrors(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name          string
		expectedError string

		payload dmioctl.Payload
	}{
		{
			name:          "name too long",
			payload:       dmioctl.Payload{Name: strings.Repeat("n", dmioctl.NameLen)},
			expectedError: "device name .* is too long: 128 bytes, limit is 127",
		},
		{
			name:          "uuid too long",
			payload:       dmioctl.Payload{UUID: strings.Repeat("u", dmioctl.UUIDLen)},
			expectedError: "device UUID .* is too long: 129 bytes, limit is 128",
		},
		{
			name:          "NUL in name",
			payload:       dmioctl.Payload{Name: "mpatha\x00-part1"},
			expectedError: "may not contain a NUL byte",
		},
		{
			name:          "target type too long",
			payload:       dmioctl.Payload{Targets: []dmioctl.Target{{Type: strings.Repeat("t", dmioctl.TypeLen)}}},
			expectedError: "target type .* is too long: 16 bytes, limit is 15",
		},
		{
			name:          "NUL in target params",
			payload:       dmioctl.Payload{Targets: []dmioctl.Target{{Type: "linear", Params: "7:0\x002048"}}},
			expectedError: "may not contain a NUL byte",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := test.payload.Build()
			assert.ErrorContains(t, err, "")
			assert.Regexp(t, test.expectedError, err.Error())
		})
	}

	// the longest name and UUID which still fit are accepted
	_, err := dmioctl.Payload{
		Name: strings.Repeat("n", dmioctl.NameLen-1),
		UUID: strings.Repeat("u", dmioctl.UUIDLen-1),
	}.Build()
	assert.NoError(t, err)
}

// nameListReply builds a reply to ListDevicesRequest the way the kernel lays it out.
func nameListReply(t *testing.T, entries []dmioctl.NameListEntry) []byte {
	t.Helper()

	records := make([]byte, 0, 256)

	for i, entry := range entries {
		size := dmioctl.NAMELIST_SIZE + len(entry.Name) + 1
		last := i == len(entries)-1

		// every record but the last is padded out and points at the next one
		if !last {
			size = (size + 7) &^ 7
		}

		record := make([]byte, size)

		dmioctl.NameList(record).Put_dev(entry.Dev)
		copy(record[dmioctl.NAMELIST_SIZE:], entry.Name)

		if !last {
			dmioctl.NameList(record).Put_next(uint32(size))
		}

		records = append(records, record...)
	}

	// an empty list is a single record with a zeroed device number
	if len(entries) == 0 {
		records = make([]byte, dmioctl.NAMELIST_SIZE)
	}

	payload := make([]byte, dmioctl.HEADER_SIZE+len(records))
	copy(payload[dmioctl.HEADER_SIZE:], records)

	header := dmioctl.Header(payload)
	header.Put_data_start(dmioctl.HEADER_SIZE)
	header.Put_data_size(uint32(len(payload)))

	return payload
}

func TestParseNameList(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		entries []dmioctl.NameListEntry
	}{
		{
			name: "empty",
		},
		{
			name:    "one device",
			entries: []dmioctl.NameListEntry{{Name: "mpatha", Dev: 0xfd00}},
		},
		{
			name: "several devices with names of differing length",
			entries: []dmioctl.NameListEntry{
				{Name: "mpatha", Dev: 0xfd00},
				{Name: "mpatha-part1", Dev: 0xfd01},
				{Name: "x", Dev: 0xfd02},
				{Name: "vg0-lv_root", Dev: 0xfd03},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := dmioctl.ParseNameList(nameListReply(t, test.entries))
			require.NoError(t, err)

			assert.Equal(t, test.entries, parsed)
		})
	}
}

func TestParseNameListErrors(t *testing.T) {
	t.Parallel()

	t.Run("payload too short", func(t *testing.T) {
		t.Parallel()

		_, err := dmioctl.ParseNameList(make([]byte, 16))
		assert.ErrorContains(t, err, "payload is too short")
	})

	t.Run("bounds outside the payload", func(t *testing.T) {
		t.Parallel()

		payload := make([]byte, dmioctl.HEADER_SIZE)
		dmioctl.Header(payload).Put_data_start(dmioctl.HEADER_SIZE)
		dmioctl.Header(payload).Put_data_size(dmioctl.HEADER_SIZE * 2)

		_, err := dmioctl.ParseNameList(payload)
		assert.ErrorContains(t, err, "invalid reply bounds")
	})

	t.Run("unterminated name", func(t *testing.T) {
		t.Parallel()

		payload := make([]byte, dmioctl.HEADER_SIZE+dmioctl.NAMELIST_SIZE+4)
		header := dmioctl.Header(payload)
		header.Put_data_start(dmioctl.HEADER_SIZE)
		header.Put_data_size(uint32(len(payload)))

		record := dmioctl.NameList(payload[dmioctl.HEADER_SIZE:])
		record.Put_dev(0xfd00)
		copy(payload[dmioctl.HEADER_SIZE+dmioctl.NAMELIST_SIZE:], "name")

		_, err := dmioctl.ParseNameList(payload)
		assert.ErrorContains(t, err, "unterminated device name")
	})

	t.Run("record length below the record header", func(t *testing.T) {
		t.Parallel()

		payload := nameListReply(t, []dmioctl.NameListEntry{{Name: "a", Dev: 1}, {Name: "b", Dev: 2}})
		dmioctl.NameList(payload[dmioctl.HEADER_SIZE:]).Put_next(4)

		_, err := dmioctl.ParseNameList(payload)
		assert.ErrorContains(t, err, "invalid record length")
	})

	t.Run("record length past the end of the reply", func(t *testing.T) {
		t.Parallel()

		for _, next := range []uint32{uint32(dmioctl.HEADER_SIZE) * 2, 1 << 30, 1<<32 - 1} {
			payload := nameListReply(t, []dmioctl.NameListEntry{{Name: "a", Dev: 1}, {Name: "b", Dev: 2}})
			dmioctl.NameList(payload[dmioctl.HEADER_SIZE:]).Put_next(next)

			// the list is truncated rather than short one device: an offset which leaves no room
			// for the record it points at is a malformed reply
			_, err := dmioctl.ParseNameList(payload)
			assert.ErrorContains(t, err, "invalid record length")
		}
	})
}

func TestDecodeDev(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		major, minor uint32
	}{
		{major: 253, minor: 0},
		{major: 253, minor: 7},
		{major: 253, minor: 255},
		{major: 253, minor: 256},
		{major: 7, minor: 0},
		{major: 8, minor: 16},
		{major: 259, minor: 1048575},
		{major: 4095, minor: 0},
	} {
		t.Run("", func(t *testing.T) {
			t.Parallel()

			// new_encode_dev(), the encoding the ioctl interface reports device numbers in
			encoded := uint64((test.minor & 0xff) | (test.major << 8) | ((test.minor &^ 0xff) << 12))

			major, minor := dmioctl.DecodeDev(encoded)

			assert.Equal(t, test.major, major)
			assert.Equal(t, test.minor, minor)
		})
	}
}

// TestHeaderLayout pins the offsets of the fields the kernel reads, which have to match
// 'struct dm_ioctl' exactly.
func TestHeaderLayout(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 312, dmioctl.HEADER_SIZE)
	assert.Equal(t, 40, dmioctl.SPEC_SIZE)
	assert.Equal(t, 12, dmioctl.NAMELIST_SIZE)

	header := make(dmioctl.Header, dmioctl.HEADER_SIZE)

	header.Put_version_major(1)
	header.Put_version_minor(2)
	header.Put_version_patch(3)
	header.Put_data_size(4)
	header.Put_data_start(5)
	header.Put_target_count(6)
	header.Put_open_count(7)
	header.Put_flags(8)
	header.Put_event_nr(9)
	header.Put_dev(10)

	for offset, expected := range map[int]uint32{0: 1, 4: 2, 8: 3, 12: 4, 16: 5, 20: 6, 24: 7, 28: 8, 32: 9} {
		assert.Equal(t, expected, binary.NativeEndian.Uint32(header[offset:offset+4]), "offset %d", offset)
	}

	assert.EqualValues(t, 10, binary.NativeEndian.Uint64(header[40:48]))
	assert.Equal(t, 128, len(header.Get_name()))
	assert.Equal(t, 129, len(header.Get_uuid()))
}
