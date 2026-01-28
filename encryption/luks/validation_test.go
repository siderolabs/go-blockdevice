// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package luks

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateLUKS2JSONMetadata_AllowsConfiguredCipher(t *testing.T) {
	metadata := &luks2JSONMetadata{
		Keyslots: map[string]luks2JSONKeyslot{
			"0": {
				Type: "luks2",
				Area: struct {
					Encryption string `json:"encryption"`
				}{Encryption: "aes-xts-plain64"},
			},
		},
		Segments: map[string]luks2JSONSegment{
			"0": {
				Type:       "crypt",
				Encryption: "aes-xts-plain64",
			},
		},
	}

	policy := ValidationPolicy{AllowedSegmentEncryptions: []string{"aes-xts-plain64"}}

	if err := validateLUKS2JSONMetadata(metadata, policy); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateLUKS2JSONMetadata_RejectsNullCipherSegment(t *testing.T) {
	metadata := &luks2JSONMetadata{
		Keyslots: map[string]luks2JSONKeyslot{
			"0": {Type: "luks2"},
		},
		Segments: map[string]luks2JSONSegment{
			"0": {
				Type:       "crypt",
				Encryption: "cipher_null-ecb",
			},
		},
	}

	err := validateLUKS2JSONMetadata(metadata, ValidationPolicy{AllowedSegmentEncryptions: []string{"cipher_null-ecb"}})
	if err == nil {
		t.Fatalf("expected error")
	}

	if !errors.Is(err, ErrMetadataRejected) {
		t.Fatalf("expected ErrMetadataRejected, got %v", err)
	}
}

func TestValidateLUKS2JSONMetadata_RejectsUnexpectedCipher(t *testing.T) {
	metadata := &luks2JSONMetadata{
		Keyslots: map[string]luks2JSONKeyslot{
			"0": {Type: "luks2"},
		},
		Segments: map[string]luks2JSONSegment{
			"0": {
				Type:       "crypt",
				Encryption: "xchacha20,aes-adiantum-plain64",
			},
		},
	}

	err := validateLUKS2JSONMetadata(metadata, ValidationPolicy{AllowedSegmentEncryptions: []string{"aes-xts-plain64"}})
	if err == nil {
		t.Fatalf("expected error")
	}

	if !errors.Is(err, ErrMetadataRejected) {
		t.Fatalf("expected ErrMetadataRejected, got %v", err)
	}
}

func TestCreateDetachedHeaderFile_RejectsGroupWritableDir(t *testing.T) {
	dir := t.TempDir()

	if err := os.Chmod(dir, 0o770); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	_, err := createDetachedHeaderFile(dir)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestCreateDetachedHeaderFile_RejectsSymlink(t *testing.T) {
	target := t.TempDir()
	linkParent := t.TempDir()
	linkPath := filepath.Join(linkParent, "hdr")

	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, err := createDetachedHeaderFile(linkPath)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestCreateDetachedHeaderFile_Creates0600File(t *testing.T) {
	dir := t.TempDir()

	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	f, err := createDetachedHeaderFile(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(f.Name())
		_ = f.Close()
	})

	st, err := os.Stat(f.Name())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	if st.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600, got %o", st.Mode().Perm())
	}
}
