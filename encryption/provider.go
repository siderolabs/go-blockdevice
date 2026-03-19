// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package encryption

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/siderolabs/go-blockdevice/v2/encryption/token"
)

const (
	// LUKS2 encryption.
	LUKS2 = "luks2"
	// Unknown unecrypted or unsupported encryption.
	Unknown = "unknown"
)

// Provider represents encryption utility methods.
type Provider interface {
	TokenProvider
	Encrypt(ctx context.Context, devname string, key *Key) error
	IsOpen(ctx context.Context, devname, mappedName string) (bool, string, error)
	Open(ctx context.Context, devname, mappedName string, key *Key) (string, error)
	Close(ctx context.Context, devname string) error
	AddKey(ctx context.Context, devname string, key, newKey *Key) error
	CheckKey(ctx context.Context, devname string, key *Key) (bool, error)
	RemoveKey(ctx context.Context, devname string, slot int, key *Key) error
	ReadKeyslots(deviceName string) (*Keyslots, error)
}

// TokenProvider represents token management methods.
type TokenProvider interface {
	SetToken(ctx context.Context, devname string, slot int, token token.Token) error
	ReadToken(ctx context.Context, devname string, slot int, token token.Token) error
	RemoveToken(ctx context.Context, devname string, slot int) error
}

var (
	// ErrEncryptionKeyRejected triggered when encryption key does not match.
	ErrEncryptionKeyRejected = fmt.Errorf("encryption key rejected")

	// ErrDeviceBusy returned when mapped device is still in use.
	ErrDeviceBusy = fmt.Errorf("mapped device is still in use")

	// ErrTokenNotFound returned when trying to get/delete not existing token.
	ErrTokenNotFound = fmt.Errorf("no token with supplied id exists")

	// ErrDeviceNotReady returned when device is not ready.
	ErrDeviceNotReady = fmt.Errorf("device is not ready")
)

// Keyslots represents LUKS2 keyslots metadata.
type Keyslots struct {
	Keyslots map[string]*Keyslot `json:"keyslots"`
}

// JSONMetadata represents LUKS2 JSON metadata.
type JSONMetadata struct {
	Keyslots map[string]*Keyslot `json:"keyslots"`
	Segments map[string]*Segment `json:"segments"`
}

// Segment represents a single LUKS2 segment.
type Segment struct {
	Type       string     `json:"type"`
	Size       string     `json:"size"`
	IVTweak    string     `json:"iv_tweak"`
	Encryption string     `json:"encryption"`
	Flags      []string   `json:"flags,omitempty"`
	Offset     StringUint `json:"offset"`
	SectorSize int64      `json:"sector_size"`
}

// StringUint is a uint64 that unmarshals from a JSON quoted string (e.g. "16777216").
type StringUint uint64

// UnmarshalJSON implements json.Unmarshaler.
func (s *StringUint) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), "\"")

	v, err := strconv.ParseUint(str, 10, 64)
	if err != nil {
		return err
	}

	*s = StringUint(v)

	return nil
}

// KeyslotArea represents the area parameters of a LUKS2 keyslot.
type KeyslotArea struct {
	Encryption string `json:"encryption"`
}

// KeyslotKDF represents the KDF parameters of a LUKS2 keyslot.
type KeyslotKDF struct {
	Type string `json:"type"`
}

// Keyslot represents a single LUKS2 keyslot.
type Keyslot struct {
	Type    string      `json:"type"`
	Area    KeyslotArea `json:"area"`
	KDF     KeyslotKDF  `json:"kdf"`
	KeySize int64       `json:"key_size"`
}

// NewKey create a new key.
func NewKey(slot int, value []byte) *Key {
	return &Key{
		Value: value,
		Slot:  slot,
	}
}

// AnyKeyslot tells providers to pick any keyslot.
const AnyKeyslot = -1

// Key represents a single key.
type Key struct {
	Value []byte
	Slot  int
}
