// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package luks provides a way to call LUKS2 cryptsetup.
package luks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/siderolabs/go-cmd/pkg/cmd"

	"github.com/siderolabs/go-blockdevice/v2/block"
	"github.com/siderolabs/go-blockdevice/v2/encryption"
	"github.com/siderolabs/go-blockdevice/v2/encryption/token"
	"github.com/siderolabs/go-blockdevice/v2/internal/luks2"
)

// Cipher LUKS2 cipher type.
type Cipher int

var keySizeDefaults = map[Cipher]uint{
	AESXTSPlain64Cipher: 512,
	XChaCha12Cipher:     256,
	XChaCha20Cipher:     256,
}

// String converts to command line string parameter value.
func (c Cipher) String() (string, error) {
	switch c {
	case AESXTSPlain64Cipher:
		return AESXTSPlain64CipherString, nil
	case XChaCha12Cipher:
		return XChaCha12String, nil
	case XChaCha20Cipher:
		return XChaCha20String, nil
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
	case XChaCha12String:
		return XChaCha12Cipher, nil
	case XChaCha20String:
		return XChaCha20Cipher, nil
	default:
		return 0, fmt.Errorf("unknown cipher kind %s", s)
	}
}

const (
	// AESXTSPlain64CipherString string representation of aes-xts-plain64 cipher.
	AESXTSPlain64CipherString = "aes-xts-plain64"
	// XChaCha12String string representation of xchacha12 cipher.
	XChaCha12String = "xchacha12,aes-adiantum-plain64"
	// XChaCha20String string representation of xchacha20 cipher.
	XChaCha20String = "xchacha20,aes-adiantum-plain64"
	// AESXTSPlain64Cipher represents aes-xts-plain64 encryption cipher.
	AESXTSPlain64Cipher Cipher = iota
	// XChaCha12Cipher represents xchacha12 encryption cipher.
	XChaCha12Cipher
	// XChaCha20Cipher represents xchacha20 encryption cipher.
	XChaCha20Cipher
)

const (
	// PerfNoReadWorkqueue sets --perf-no_read_workqueue.
	PerfNoReadWorkqueue = "no_read_workqueue"
	// PerfNoWriteWorkqueue sets --perf-no_write_workqueue.
	PerfNoWriteWorkqueue = "no_write_workqueue"
	// PerfSameCPUCrypt sets --perf-same_cpu_crypt.
	PerfSameCPUCrypt = "same_cpu_crypt"
)

// ValidatePerfOption checks that specified string is a valid perf option.
func ValidatePerfOption(value string) error {
	switch value {
	case PerfNoReadWorkqueue:
		fallthrough
	case PerfNoWriteWorkqueue:
		fallthrough
	case PerfSameCPUCrypt:
		return nil
	}

	return fmt.Errorf("invalid perf option %v", value)
}

// LUKS implements LUKS2 encryption provider.
type LUKS struct {
	perfOptions          []string
	cipher               Cipher
	iterTime             time.Duration
	pbkdfForceIterations uint
	pbkdfMemory          uint64
	blockSize            uint64
	keySize              uint
}

// New creates new LUKS2 encryption provider.
func New(cipher Cipher, options ...Option) *LUKS {
	l := &LUKS{
		cipher: cipher,
	}

	for _, option := range options {
		option(l)
	}

	if l.keySize == 0 {
		l.keySize = keySizeDefaults[cipher]
	}

	return l
}

// Open runs luksOpen on a device and returns mapped device path.
func (l *LUKS) Open(ctx context.Context, deviceName, mappedName string, key *encryption.Key) (string, error) {
	args := slices.Concat(
		[]string{"luksOpen", deviceName, mappedName, "--key-file=-"},
		keyslotArgs(key),
		l.perfArgs(),
	)

	_, err := l.runCommand(ctx, args, key.Value)
	if err != nil {
		return "", err
	}

	return filepath.Join("/dev/mapper", mappedName), nil
}

// Encrypt implements encryption.Provider.
func (l *LUKS) Encrypt(ctx context.Context, deviceName string, key *encryption.Key) error {
	cipher, err := l.cipher.String()
	if err != nil {
		return err
	}

	args := slices.Concat(
		[]string{"luksFormat", "--type", "luks2", "--key-file=-", "-c", cipher, deviceName},
		l.argonArgs(),
		keyslotArgs(key),
		l.encryptionArgs(),
	)

	if l.blockSize != 0 {
		args = append(args, fmt.Sprintf("--sector-size=%d", l.blockSize))
	}

	_, err = l.runCommand(ctx, args, key.Value)

	return err
}

// Resize implements encryption.Provider.
func (l *LUKS) Resize(ctx context.Context, devname string, key *encryption.Key) error {
	args := []string{"resize", devname, "--key-file=-"}

	_, err := l.runCommand(ctx, args, key.Value)

	return err
}

// Close implements encryption.Provider.
func (l *LUKS) Close(ctx context.Context, devname string) error {
	_, err := l.runCommand(ctx, []string{"luksClose", devname}, nil)

	return err
}

// AddKey adds a new key at the LUKS encryption slot.
func (l *LUKS) AddKey(ctx context.Context, devname string, key, newKey *encryption.Key) error {
	var buffer bytes.Buffer

	keyfileLen, _ := buffer.Write(key.Value)
	buffer.Write(newKey.Value)

	args := slices.Concat(
		[]string{
			"luksAddKey",
			devname,
			"--key-file=-",
			fmt.Sprintf("--keyfile-size=%d", keyfileLen),
		},
		l.argonArgs(),
		l.encryptionArgs(),
		keyslotArgs(newKey),
	)

	_, err := l.runCommand(ctx, args, buffer.Bytes())

	return err
}

// SetKey sets new key value at the LUKS encryption slot.
func (l *LUKS) SetKey(ctx context.Context, devname string, oldKey, newKey *encryption.Key) error {
	if oldKey.Slot != newKey.Slot {
		return fmt.Errorf("old and new key slots must match")
	}

	var buffer bytes.Buffer

	keyfileLen, _ := buffer.Write(oldKey.Value)
	buffer.Write(newKey.Value)

	args := slices.Concat(
		[]string{
			"luksChangeKey",
			devname,
			"--key-file=-",
			fmt.Sprintf("--key-slot=%d", newKey.Slot),
			fmt.Sprintf("--keyfile-size=%d", keyfileLen),
		},
		l.argonArgs(),
		l.perfArgs(),
	)

	_, err := l.runCommand(ctx, args, buffer.Bytes())

	return err
}

// CheckKey checks if the key is valid.
func (l *LUKS) CheckKey(ctx context.Context, devname string, key *encryption.Key) (bool, error) {
	args := slices.Concat(
		[]string{"luksOpen", "--test-passphrase", devname, "--key-file=-"},
		keyslotArgs(key),
	)

	_, err := l.runCommand(ctx, args, key.Value)
	if err != nil {
		if err == encryption.ErrEncryptionKeyRejected { //nolint:errorlint
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// RemoveKey removes a key at the specified LUKS encryption slot.
func (l *LUKS) RemoveKey(ctx context.Context, devname string, slot int, key *encryption.Key) error {
	_, err := l.runCommand(ctx, []string{"luksKillSlot", devname, strconv.Itoa(slot), "--key-file=-"}, key.Value)
	if err != nil {
		return err
	}

	if err = l.RemoveToken(ctx, devname, slot); err != nil && !errors.Is(err, encryption.ErrTokenNotFound) {
		return err
	}

	return nil
}

// ReadKeyslots returns deserialized LUKS2 keyslots JSON.
func (l *LUKS) ReadKeyslots(deviceName string) (*encryption.Keyslots, error) {
	bd, err := block.NewFromPath(deviceName)
	if err != nil {
		return nil, err
	}

	defer bd.Close() //nolint:errcheck

	sb := make(luks2.Luks2Header, 4096)

	if _, err = io.ReadFull(bd.File(), sb[:]); err != nil {
		return nil, err
	}

	jsonArea := make([]byte, int(sb.Get_hdr_size())-len(sb))

	if _, err = io.ReadFull(bd.File(), jsonArea); err != nil {
		return nil, err
	}

	jsonArea = bytes.Trim(bytes.TrimSpace(jsonArea), "\x00")

	var keyslots *encryption.Keyslots

	if err = json.Unmarshal(jsonArea, &keyslots); err != nil {
		return nil, err
	}

	return keyslots, nil
}

// SetToken adds arbitrary token to the key slot.
// Token id == slot id: only one token per key slot is supported.
func (l *LUKS) SetToken(ctx context.Context, devname string, slot int, token token.Token) error {
	data, err := token.Bytes()
	if err != nil {
		return err
	}

	id := strconv.Itoa(slot)

	_, err = l.runCommand(ctx, []string{"token", "import", "-q", devname, "--token-id", id, "--json-file=-", "--token-replace"}, data)

	return err
}

// ReadToken reads arbitrary token from the luks metadata.
func (l *LUKS) ReadToken(ctx context.Context, devname string, slot int, token token.Token) error {
	stdout, err := l.runCommand(ctx, []string{"token", "export", "-q", devname, "--token-id", strconv.Itoa(slot), "--json-file=-"}, nil)
	if err != nil {
		return err
	}

	return token.Decode([]byte(stdout))
}

// RemoveToken removes token from the luks metadata.
func (l *LUKS) RemoveToken(ctx context.Context, devname string, slot int) error {
	_, err := l.runCommand(ctx, []string{"token", "remove", "--token-id", strconv.Itoa(slot), devname}, nil)

	return err
}

var notFoundMatcher = regexp.MustCompile("(is not in use|Failed to get token)")

// runCommand executes cryptsetup with arguments.
func (l *LUKS) runCommand(ctx context.Context, args []string, stdin []byte) (string, error) {
	stdout, err := cmd.RunContext(cmd.WithStdin(
		ctx,
		bytes.NewReader(stdin)), "cryptsetup", args...)
	if err != nil {
		var exitError *cmd.ExitError

		if errors.As(err, &exitError) {
			switch exitError.ExitCode {
			case 1:
				if strings.Contains(string(exitError.Output), "No usable keyslot is available.") {
					return "", encryption.ErrEncryptionKeyRejected
				}

				if notFoundMatcher.Match(exitError.Output) {
					return "", encryption.ErrTokenNotFound
				}
			case 2:
				return "", encryption.ErrEncryptionKeyRejected
			case 5:
				return "", encryption.ErrDeviceBusy
			}
		}

		return "", fmt.Errorf("failed to call cryptsetup: %w", err)
	}

	return stdout, nil
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

func (l *LUKS) perfArgs() []string {
	res := []string{}

	for _, o := range l.perfOptions {
		res = append(res, fmt.Sprintf("--perf-%s", o))
	}

	return res
}

func (l *LUKS) encryptionArgs() []string {
	res := []string{}

	if l.keySize != 0 {
		res = append(res, fmt.Sprintf("--key-size=%d", l.keySize))
	}

	return append(res, l.perfArgs()...)
}

func keyslotArgs(key *encryption.Key) []string {
	if key.Slot != encryption.AnyKeyslot {
		return []string{fmt.Sprintf("--key-slot=%d", key.Slot)}
	}

	return []string{}
}
