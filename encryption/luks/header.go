// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package luks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/siderolabs/go-blockdevice/v2/block"
	"github.com/siderolabs/go-blockdevice/v2/encryption"
	"github.com/siderolabs/go-blockdevice/v2/internal/luks2"
)

type luksHeader struct {
	jsonMetadata *encryption.JSONMetadata
	header       luks2.Luks2Header
	raw          []byte
}

// Read both LUKS headers.
func (l *LUKS) readHeader(deviceName string) (*luksHeader, error) {
	bd, err := block.NewFromPath(deviceName)
	if err != nil {
		return nil, err
	}

	defer bd.Close() //nolint:errcheck

	sb1 := make(luks2.Luks2Header, 4096)

	if _, err = io.ReadFull(bd.File(), sb1[:]); err != nil {
		return nil, err
	}

	if string(sb1.Get_magic()) != luks2.MagicValue {
		return nil, fmt.Errorf("invalid LUKS2 header magic (is %q a LUKS device?): %s", deviceName, string(sb1.Get_magic()))
	}

	jsonAreaRaw1 := make([]byte, sb1.Get_hdr_size()-uint64(len(sb1)))
	if _, err = io.ReadFull(bd.File(), jsonAreaRaw1); err != nil {
		return nil, err
	}

	jsonArea1 := bytes.Trim(bytes.TrimSpace(jsonAreaRaw1), "\x00")

	sb2 := make(luks2.Luks2Header, 4096)

	if _, err = io.ReadFull(bd.File(), sb2[:]); err != nil {
		return nil, err
	}

	jsonAreaRaw2 := make([]byte, int(sb1.Get_hdr_size())-len(sb1))
	if _, err = io.ReadFull(bd.File(), jsonAreaRaw2); err != nil {
		return nil, err
	}

	jsonArea2 := bytes.Trim(bytes.TrimSpace(jsonAreaRaw2), "\x00")

	// a tmp struct to unmarshal only the offset in case rest of the json is corrupted
	type segment struct {
		Offset string `json:"offset"`
	}

	type jsonOffsetMeta struct {
		Segments map[string]segment `json:"segments"`
	}

	var offsetMeta jsonOffsetMeta

	if err = json.Unmarshal(jsonArea1, &offsetMeta); err != nil {
		if err = json.Unmarshal(jsonArea2, &offsetMeta); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON area of both headers: %w", err)
		}
	}

	dataOffset, err := strconv.Atoi(offsetMeta.Segments["0"].Offset)
	if err != nil {
		return nil, fmt.Errorf("invalid data offset: %w", err)
	}

	if dataOffset == 0 {
		return nil, fmt.Errorf("unexpected zero data offset")
	}

	headerData := make([]byte, dataOffset)

	if _, err = bd.File().Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	if _, err = io.ReadFull(bd.File(), headerData); err != nil {
		return nil, err
	}

	// validate that the headers are identical if they have the same seqid
	if sb1.Get_seqid() == sb2.Get_seqid() {
		if !bytes.Equal(jsonArea1, jsonArea2) {
			return nil, fmt.Errorf("LUKS2 headers have the same seqid but different JSON areas")
		}
	}

	// pick the header with the higher seqid
	selectedHeader := sb1
	selectedJSON := jsonArea1

	if sb1.Get_seqid() < sb2.Get_seqid() {
		selectedHeader = sb2
		selectedJSON = jsonArea2
	}

	var metadata *encryption.JSONMetadata

	if err = json.Unmarshal(selectedJSON, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON area: %w", err)
	}

	if err := validateHeader(selectedHeader, metadata); err != nil {
		return nil, fmt.Errorf("LUKS2 header validation error: %w", err)
	}

	return &luksHeader{
		jsonMetadata: metadata,
		header:       selectedHeader,
		raw:          headerData,
	}, nil
}

var validCiphers = map[string]struct{}{
	AESXTSPlain64CipherString: {},
	XChaCha12String:           {},
	XChaCha20String:           {},
}

func validateHeader(sb luks2.Luks2Header, metadata *encryption.JSONMetadata) error {
	if sb.Get_version() != 2 {
		return fmt.Errorf("unexpected LUKS2 primary header version: %d", sb.Get_version())
	}

	if len(metadata.Keyslots) == 0 {
		return fmt.Errorf("no keyslots found")
	}

	for id, keyslot := range metadata.Keyslots {
		if keyslot.Type != "luks2" {
			return fmt.Errorf("keyslot %s has unexpected type %q", id, keyslot.Type)
		}

		if _, ok := validCiphers[keyslot.Area.Encryption]; !ok {
			return fmt.Errorf("keyslot %s has unexpected area encryption %q", id, keyslot.Area.Encryption)
		}

		// cryptsetup < 2.4.0 (e.g. 2.3.0) defaulted to argon2i, while newer versions default to argon2id.
		switch keyslot.KDF.Type {
		case "argon2id", "argon2i":
		default:
			return fmt.Errorf("keyslot %s has unexpected KDF type %q", id, keyslot.KDF.Type)
		}
	}

	if len(metadata.Segments) != 1 {
		return fmt.Errorf("expected 1 segment, got %d", len(metadata.Segments))
	}

	segment, ok := metadata.Segments["0"]
	if !ok {
		return fmt.Errorf("segment 0 not found")
	}

	if segment.Type != "crypt" {
		return fmt.Errorf("segment 0 has unexpected type %q", segment.Type)
	}

	if _, ok := validCiphers[segment.Encryption]; !ok {
		return fmt.Errorf("segment 0 has unexpected encryption %q", segment.Encryption)
	}

	if len(segment.Flags) > 0 {
		return fmt.Errorf("segment 0 has unexpected flags: %v", segment.Flags)
	}

	return nil
}
