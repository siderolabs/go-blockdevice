// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package luks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
)

// ErrMetadataRejected is returned when detached-header validation rejects the LUKS metadata.
var ErrMetadataRejected = errors.New("luks metadata rejected")

// ValidationPolicy controls which LUKS2 metadata settings are considered acceptable.
type ValidationPolicy struct {
	AllowedSegmentEncryptions []string
	AllowSegmentFlags bool
}

type luks2JSONMetadata struct {
	Keyslots map[string]luks2JSONKeyslot `json:"keyslots"`
	Segments map[string]luks2JSONSegment `json:"segments"`
	Tokens   map[string]json.RawMessage  `json:"tokens"`
}

type luks2JSONKeyslot struct {
	Type string `json:"type"`
	Area struct {
		Encryption string `json:"encryption"`
	} `json:"area"`
}

type luks2JSONSegment struct {
	Type       string   `json:"type"`
	Encryption string   `json:"encryption"`
	Flags      []string `json:"flags"`
}

func isNullCipher(enc string) bool {
	enc = strings.ToLower(enc)
	return strings.Contains(enc, "cipher_null")
}

func validateLUKS2JSONMetadata(metadata *luks2JSONMetadata, policy ValidationPolicy) error {
	if metadata == nil {
		return fmt.Errorf("%w: nil metadata", ErrMetadataRejected)
	}

	if len(metadata.Segments) == 0 {
		return fmt.Errorf("%w: missing segments", ErrMetadataRejected)
	}

	seenCrypt := false

	for id, segment := range metadata.Segments {
		if segment.Type != "crypt" {
			return fmt.Errorf("%w: unsupported segment type %q (segment %q)", ErrMetadataRejected, segment.Type, id)
		}

		seenCrypt = true

		if segment.Encryption == "" {
			return fmt.Errorf("%w: missing segment encryption (segment %q)", ErrMetadataRejected, id)
		}

		if isNullCipher(segment.Encryption) {
			return fmt.Errorf("%w: null cipher segment encryption %q (segment %q)", ErrMetadataRejected, segment.Encryption, id)
		}

		if policy.AllowedSegmentEncryptions != nil && !slices.Contains(policy.AllowedSegmentEncryptions, segment.Encryption) {
			return fmt.Errorf("%w: segment encryption %q is not allowed", ErrMetadataRejected, segment.Encryption)
		}

		if !policy.AllowSegmentFlags && len(segment.Flags) > 0 {
			return fmt.Errorf("%w: segment contains flags %v (segment %q)", ErrMetadataRejected, segment.Flags, id)
		}
	}

	if !seenCrypt {
		return fmt.Errorf("%w: no crypt segments present", ErrMetadataRejected)
	}

	for id, slot := range metadata.Keyslots {
		if slot.Type != "luks2" {
			return fmt.Errorf("%w: unsupported keyslot type %q (keyslot %q)", ErrMetadataRejected, slot.Type, id)
		}

		if slot.Area.Encryption != "" && isNullCipher(slot.Area.Encryption) {
			return fmt.Errorf("%w: null cipher keyslot encryption %q (keyslot %q)", ErrMetadataRejected, slot.Area.Encryption, id)
		}
	}

	return nil
}

func createDetachedHeaderFile(dir string) (*os.File, error) {
	if dir == "" {
		return nil, fmt.Errorf("detached header dir is empty")
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create detached header dir %q: %w", dir, err)
	}

	fi, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to stat detached header dir %q: %w", dir, err)
	}

	if !fi.IsDir() {
		return nil, fmt.Errorf("detached header dir %q is not a directory", dir)
	}

	if fi.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("detached header dir %q is writable by non-owner (mode %o)", dir, fi.Mode().Perm())
	}

	f, err := os.CreateTemp(dir, "luks-header-")
	if err != nil {
		return nil, fmt.Errorf("failed to create detached header file in %q: %w", dir, err)
	}

	if err = f.Chmod(0o600); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return nil, fmt.Errorf("failed to chmod detached header file %q: %w", f.Name(), err)
	}

	return f, nil
}

func (l *LUKS) dumpJSONMetadata(ctx context.Context, headerPath string) (*luks2JSONMetadata, error) {
	stdout, err := l.runCommand(ctx, []string{"luksDump", "--type", "luks2", "--dump-json-metadata", headerPath}, nil)
	if err != nil {
		return nil, err
	}

	var metadata luks2JSONMetadata

	if err = json.Unmarshal([]byte(stdout), &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse luks json metadata: %w", err)
	}

	return &metadata, nil
}
