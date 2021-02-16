// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package luks provides a way to call LUKS2 cryptsetup.
package luks

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/talos-systems/go-cmd/pkg/cmd"
	"golang.org/x/sys/unix"

	"github.com/talos-systems/go-blockdevice/blockdevice/encryption"
	"github.com/talos-systems/go-blockdevice/blockdevice/filesystem/luks"
	"github.com/talos-systems/go-blockdevice/blockdevice/util"
)

// Cipher LUKS2 cipher type.
type Cipher int

// String converts to command line string parameter value.
func (c Cipher) String() (string, error) {
	switch c {
	case AESXTSPlain64Cipher:
		return AESXTSPlain64CipherString, nil
	default:
		return "", fmt.Errorf("unknown cipher kind %d", c)
	}
}

// ParseCipherKind converts cipher string into cipher type.
func ParseCipherKind(s string) (Cipher, error) {
	switch s {
	case "": // default
		fallthrough
	case AESXTSPlain64CipherString:
		return AESXTSPlain64Cipher, nil
	default:
		return 0, fmt.Errorf("unknown cipher kind %s", s)
	}
}

const (
	// AESXTSPlain64CipherString string representation of aes-xts-plain64 cipher.
	AESXTSPlain64CipherString = "aes-xts-plain64"
	// AESXTSPlain64Cipher represents aes-xts-plain64 encryption cipher.
	AESXTSPlain64Cipher Cipher = iota
)

// LUKS implements LUKS2 encryption provider.
type LUKS struct {
	cipher               Cipher
	iterTime             time.Duration
	pbkdfForceIterations uint
	pbkdfMemory          uint64
}

// New creates new LUKS2 encryption provider.
func New(cipher Cipher, options ...Option) *LUKS {
	l := &LUKS{
		cipher: cipher,
	}

	for _, option := range options {
		option(l)
	}

	return l
}

// Open runs luksOpen on a device and returns mapped device path.
func (l *LUKS) Open(deviceName string, key *encryption.Key) (string, error) {
	parts := strings.Split(deviceName, "/")
	mappedPath := util.PartPathEncrypted(parts[len(parts)-1])
	parts = strings.Split(mappedPath, "/")
	mappedName := parts[len(parts)-1]

	args := []string{"luksOpen", deviceName, mappedName, "--key-file=-"}
	args = append(args, keyslotArgs(key)...)

	err := l.runCommand(args, key.Value)
	if err != nil {
		return "", err
	}

	return mappedPath, nil
}

// Encrypt implements encryption.Provider.
func (l *LUKS) Encrypt(deviceName string, key *encryption.Key) error {
	cipher, err := l.cipher.String()
	if err != nil {
		return err
	}

	args := []string{"luksFormat", "--type", "luks2", "--key-file=-", "-c", cipher, deviceName}
	args = append(args, l.argonArgs()...)
	args = append(args, keyslotArgs(key)...)

	err = l.runCommand(args, key.Value)
	if err != nil {
		return err
	}

	return err
}

// Close implements encryption.Provider.
func (l *LUKS) Close(devname string) error {
	return l.runCommand([]string{"luksClose", devname}, nil)
}

// AddKey adds a new key at the LUKS encryption slot.
func (l *LUKS) AddKey(devname string, key, newKey *encryption.Key) error {
	var buffer bytes.Buffer

	keyfileLen, _ := buffer.Write(key.Value) //nolint:errcheck
	buffer.Write(newKey.Value)               //nolint:errcheck

	args := []string{
		"luksAddKey",
		devname,
		"--key-file=-",
		fmt.Sprintf("--keyfile-size=%d", keyfileLen),
	}

	args = append(args, l.argonArgs()...)
	args = append(args, keyslotArgs(newKey)...)

	return l.runCommand(args, buffer.Bytes())
}

// SetKey sets new key value at the LUKS encryption slot.
func (l *LUKS) SetKey(devname string, oldKey, newKey *encryption.Key) error {
	if oldKey.Slot != newKey.Slot {
		return fmt.Errorf("old and new key slots must match")
	}

	var buffer bytes.Buffer

	keyfileLen, _ := buffer.Write(oldKey.Value) //nolint:errcheck
	buffer.Write(newKey.Value)                  //nolint:errcheck

	args := []string{
		"luksChangeKey",
		devname,
		"--key-file=-",
		fmt.Sprintf("--key-slot=%d", newKey.Slot),
		fmt.Sprintf("--keyfile-size=%d", keyfileLen),
	}

	args = append(args, l.argonArgs()...)

	return l.runCommand(args, buffer.Bytes())
}

// CheckKey checks if the key is valid.
func (l *LUKS) CheckKey(devname string, key *encryption.Key) (bool, error) {
	args := []string{"luksOpen", "--test-passphrase", devname, "--key-file=-"}

	args = append(args, keyslotArgs(key)...)

	err := l.runCommand(args, key.Value)
	if err != nil {
		if err == encryption.ErrEncryptionKeyRejected { //nolint:errorlint
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// RemoveKey adds a new key at the LUKS encryption slot.
func (l *LUKS) RemoveKey(devname string, slot int, key *encryption.Key) error {
	return l.runCommand([]string{"luksKillSlot", devname, fmt.Sprintf("%d", slot), "--key-file=-", fmt.Sprintf("--key-slot=%d", key.Slot)}, key.Value)
}

// ReadKeyslots returns deserialized LUKS2 keyslots JSON.
func (l *LUKS) ReadKeyslots(deviceName string) (*encryption.Keyslots, error) {
	f, err := os.OpenFile(deviceName, os.O_RDONLY|unix.O_CLOEXEC, os.ModeDevice)
	if err != nil {
		return nil, err
	}

	defer f.Close() //nolint:errcheck

	sb := &luks.SuperBlock{}

	if err = binary.Read(f, binary.BigEndian, sb); err != nil {
		return nil, err
	}

	size := binary.Size(sb)
	if _, err = f.Seek(int64(size), 0); err != nil {
		return nil, err
	}

	jsonArea := make([]byte, int(sb.HeaderSize)-size)

	if _, err = f.Read(jsonArea); err != nil {
		return nil, err
	}

	jsonArea = bytes.Trim(bytes.TrimSpace(jsonArea), "\x00")

	var keyslots *encryption.Keyslots

	if err = json.Unmarshal(jsonArea, &keyslots); err != nil {
		return nil, err
	}

	return keyslots, nil
}

// CheckKey try using the key

// runCommand executes cryptsetup with arguments.
func (l *LUKS) runCommand(args []string, stdin []byte) error {
	_, err := cmd.RunContext(cmd.WithStdin(
		context.Background(),
		bytes.NewBuffer(stdin)), "cryptsetup", args...)
	if err != nil {
		var exitError *cmd.ExitError

		if errors.As(err, &exitError) {
			switch exitError.ExitCode {
			case 1:
				if strings.Contains(string(exitError.Output), "Keyslot open failed.\nNo usable keyslot is available.") {
					return encryption.ErrEncryptionKeyRejected
				}
			case 2:
				return encryption.ErrEncryptionKeyRejected
			case 5:
				return encryption.ErrDeviceBusy
			}
		}

		return fmt.Errorf("failed to call cryptsetup: %w", err)
	}

	return nil
}

func (l *LUKS) argonArgs() []string {
	args := []string{}

	if l.iterTime != 0 {
		args = append(args, fmt.Sprintf("--iter-time=%d", l.iterTime.Milliseconds()))
	}

	if l.pbkdfMemory != 0 {
		args = append(args, fmt.Sprintf("--pbkdf-memory=%d", l.pbkdfMemory))
	}

	if l.pbkdfForceIterations != 0 {
		args = append(args, fmt.Sprintf("--pbkdf-force-iterations=%d", l.pbkdfForceIterations))
	}

	return args
}

func keyslotArgs(key *encryption.Key) []string {
	if key.Slot != encryption.AnyKeyslot {
		return []string{fmt.Sprintf("--key-slot=%d", key.Slot)}
	}

	return []string{}
}
