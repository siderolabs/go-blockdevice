// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package encryption

import (
	"fmt"

	"github.com/siderolabs/go-blockdevice/blockdevice/encryption/token"
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
	Encrypt(devname string, key *Key) error
	Open(devname string, key *Key) (string, error)
	Close(devname string) error
	AddKey(devname string, key, newKey *Key) error
	SetKey(devname string, key, newKey *Key) error
	CheckKey(devname string, key *Key) (bool, error)
	RemoveKey(devname string, slot int, key *Key) error
	ReadKeyslots(deviceName string) (*Keyslots, error)
}

// TokenProvider represents token management methods.
type TokenProvider interface {
	SetToken(devname string, slot int, token token.Token) error
	ReadToken(devname string, slot int, token token.Token) error
	RemoveToken(devname string, slot int) error
}

var (
	// ErrEncryptionKeyRejected triggered when encryption key does not match.
	ErrEncryptionKeyRejected = fmt.Errorf("encryption key rejected")

	// ErrDeviceBusy returned when mapped device is still in use.
	ErrDeviceBusy = fmt.Errorf("mapped device is still in use")

	// ErrTokenNotFound returned when trying to get/delete not existing token.
	ErrTokenNotFound = fmt.Errorf("no token with supplied id exists")
)

// Keyslots represents LUKS2 keyslots metadata.
type Keyslots struct {
	Keyslots map[string]*Keyslot `json:"keyslots"`
}

// Keyslot represents a single LUKS2 keyslot.
type Keyslot struct {
	Type    string `json:"type"`
	KeySize int64  `json:"key_size"`
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
