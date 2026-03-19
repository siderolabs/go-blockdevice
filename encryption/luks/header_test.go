// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//nolint:testpackage
package luks

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/siderolabs/go-blockdevice/v2/encryption"
	"github.com/siderolabs/go-blockdevice/v2/internal/luks2"
)

func newTestHeader(seqid uint64, version uint16) luks2.Luks2Header {
	h := make(luks2.Luks2Header, 4096)
	h.Put_version(version)
	h.Put_seqid(seqid)

	return h
}

func defaultKeyslot() *encryption.Keyslot {
	return &encryption.Keyslot{
		Type: "luks2",
		Area: encryption.KeyslotArea{Encryption: AESXTSPlain64CipherString},
		KDF:  encryption.KeyslotKDF{Type: "argon2id"},
	}
}

func defaultMetadata() *encryption.JSONMetadata {
	return &encryption.JSONMetadata{
		Keyslots: map[string]*encryption.Keyslot{"0": defaultKeyslot()},
		Segments: map[string]*encryption.Segment{
			"0": {Type: "crypt", Encryption: AESXTSPlain64CipherString},
		},
	}
}

func TestValidateHeader(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		metadata    *encryption.JSONMetadata
		name        string
		wantContain string
		version     uint16
	}{
		{
			name:     "valid",
			metadata: defaultMetadata(),
		},
		{
			name: "valid with xchacha12",
			metadata: &encryption.JSONMetadata{
				Keyslots: map[string]*encryption.Keyslot{
					"0": {
						Type: "luks2",
						Area: encryption.KeyslotArea{Encryption: XChaCha12String},
						KDF:  encryption.KeyslotKDF{Type: "argon2id"},
					},
				},
				Segments: map[string]*encryption.Segment{
					"0": {Type: "crypt", Encryption: XChaCha12String},
				},
			},
		},
		{
			name:        "invalid header version",
			version:     1,
			metadata:    defaultMetadata(),
			wantContain: "header version",
		},
		{
			name: "invalid keyslot type",
			metadata: func() *encryption.JSONMetadata {
				m := defaultMetadata()
				ks := defaultKeyslot()
				ks.Type = "luks1"
				m.Keyslots["0"] = ks

				return m
			}(),
			wantContain: "unexpected type",
		},
		{
			name: "invalid keyslot area encryption",
			metadata: func() *encryption.JSONMetadata {
				m := defaultMetadata()
				ks := defaultKeyslot()
				ks.Area.Encryption = "serpent-xts-plain64"
				m.Keyslots["0"] = ks

				return m
			}(),
			wantContain: "unexpected area encryption",
		},
		{
			name: "invalid KDF type",
			metadata: func() *encryption.JSONMetadata {
				m := defaultMetadata()
				ks := defaultKeyslot()
				ks.KDF.Type = "pbkdf2"
				m.Keyslots["0"] = ks

				return m
			}(),
			wantContain: "unexpected KDF type",
		},
		{
			name: "no keyslots",
			metadata: func() *encryption.JSONMetadata {
				m := defaultMetadata()
				m.Keyslots = map[string]*encryption.Keyslot{}

				return m
			}(),
			wantContain: "no keyslots found",
		},
		{
			name: "multiple segments",
			metadata: func() *encryption.JSONMetadata {
				m := defaultMetadata()
				m.Segments["1"] = &encryption.Segment{Type: "crypt", Encryption: AESXTSPlain64CipherString}

				return m
			}(),
			wantContain: "expected 1 segment",
		},
		{
			name: "invalid segment type",
			metadata: func() *encryption.JSONMetadata {
				m := defaultMetadata()
				m.Segments["0"] = &encryption.Segment{Type: "linear", Encryption: AESXTSPlain64CipherString}

				return m
			}(),
			wantContain: "unexpected type",
		},
		{
			name: "invalid segment encryption",
			metadata: func() *encryption.JSONMetadata {
				m := defaultMetadata()
				m.Segments["0"] = &encryption.Segment{Type: "crypt", Encryption: "null"}

				return m
			}(),
			wantContain: "unexpected encryption",
		},
		{
			name: "segment with flags",
			metadata: func() *encryption.JSONMetadata {
				m := defaultMetadata()
				m.Segments["0"] = &encryption.Segment{Type: "crypt", Encryption: AESXTSPlain64CipherString, Flags: []string{"allow-discards"}}

				return m
			}(),
			wantContain: "unexpected flags",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			version := tt.version
			if version == 0 {
				version = 2
			}

			err := validateHeader(
				newTestHeader(1, version),
				tt.metadata,
			)

			if tt.wantContain == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantContain)
			}
		})
	}
}
