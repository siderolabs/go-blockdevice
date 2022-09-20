// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package disk provides utility method for disk listing and searching using /sys/block data.
package disk

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	glob "github.com/ryanuber/go-glob"
)

// Type is the disk type: HDD, SSD, SD card, NVMe drive.
type Type int

const (
	// TypeUnknown is set when couldn't detect the disk type.
	TypeUnknown Type = iota
	// TypeSSD SATA SSD disk.
	TypeSSD
	// TypeHDD HDD disk.
	TypeHDD
	// TypeNVMe NVMe disk.
	TypeNVMe
	// TypeSD SD card.
	TypeSD
)

func (t Type) String() string {
	//nolint:exhaustive
	switch t {
	case TypeSSD:
		return "ssd"
	case TypeHDD:
		return "hdd"
	case TypeNVMe:
		return "nvme"
	case TypeSD:
		return "sd"
	default:
		return "unknown"
	}
}

// ParseType converts string id to the disk type id.
func ParseType(id string) (Type, error) {
	id = strings.ToLower(id)

	switch id {
	case "ssd":
		return TypeSSD, nil
	case "hdd":
		return TypeHDD, nil
	case "nvme":
		return TypeNVMe, nil
	case "sd":
		return TypeSD, nil
	}

	return 0, fmt.Errorf("unknown disk type %v", id)
}

// Disk reresents disk information obtained by reading /sys/block.
//
//nolint:govet
type Disk struct {
	// Size disk size in bytes.
	Size uint64
	// Model from /sys/block/*/device/model.
	Model string
	// DeviceName device name (e.g. /dev/sda).
	DeviceName string
	// Name /sys/block/<dev>/device/name.
	Name string
	// Serial /sys/block/<dev>/device/serial.
	Serial string
	// Modalias /sys/block/<dev>/device/modalias.
	Modalias string
	// WWID /sys/block/<dev>/wwid.
	WWID string
	// UUID /sys/block/<dev>/uuid.
	UUID string
	// Type is the disk type: HDD, SSD, SD card, NVMe drive.
	Type Type
	// BusPath PCI bus path.
	BusPath string
	// ReadOnly indicates that the kernel has marked this disk as read-only.
	ReadOnly bool
}

// List returns list of disks by reading /sys/block.
func List() ([]*Disk, error) {
	disks := []*Disk{}

	sysblock := "/sys/block"

	devices, err := os.ReadDir(sysblock)
	if err != nil {
		return nil, fmt.Errorf("failed to read /sys/block directory %w", err)
	}

	for _, dev := range devices {
		skip := false
		deviceName := filepath.Base(dev.Name())

		for _, prefix := range []string{"sg", "sr", "loop", "md", "dm-", "ram"} {
			if strings.HasPrefix(deviceName, prefix) {
				skip = true

				break
			}
		}

		if skip {
			continue
		}

		disk := Get(deviceName)
		if disk.Size == 0 {
			continue
		}

		disks = append(disks, disk)
	}

	return disks, nil
}

// Get gathers disk information from sys block.
func Get(dev string) *Disk {
	sysblock := "/sys/block"

	dev = filepath.Base(dev)

	fullPath, _ := os.Readlink(filepath.Join(sysblock, dev)) //nolint:errcheck

	busPath := strings.TrimPrefix(fullPath, "../devices")
	busPath = strings.TrimSuffix(busPath, filepath.Join("block", dev))

	readFile := func(parts ...string) string {
		path := filepath.Join(parts...)

		f, e := os.Open(path)
		if e != nil {
			return ""
		}

		data, e := io.ReadAll(f)
		if e != nil {
			return ""
		}

		return strings.TrimSpace(string(data))
	}

	blockSizeString := readFile(
		fmt.Sprintf("/sys/class/block/%s/queue/logical_block_size", dev),
	)
	if blockSizeString == "" {
		blockSizeString = "512"
	}

	var size uint64

	s := readFile(sysblock, dev, "size")
	if s != "" {
		var err error

		size, err = strconv.ParseUint(strings.TrimSpace(s), 10, 64)
		if err != nil {
			size = 0
		}

		blockSize, _ := strconv.ParseUint(strings.TrimSpace(blockSizeString), 10, 64) //nolint:errcheck

		size *= blockSize
	}

	diskType := TypeUnknown
	rotational := readFile(sysblock, dev, "queue/rotational")

	switch {
	case strings.Contains(dev, "nvme"):
		diskType = TypeNVMe
	case strings.Contains(dev, "mmc"):
		diskType = TypeSD
	case rotational == "1":
		diskType = TypeHDD
	case rotational == "0":
		diskType = TypeSSD
	}

	uuid := readFile(sysblock, dev, "uuid")
	if uuid == "" {
		uuid = readFile(sysblock, dev, "device/uuid")
	}

	wwid := readFile(sysblock, dev, "wwid")
	if wwid == "" {
		wwid = readFile(sysblock, dev, "device/wwid")
	}

	serial := readFile(sysblock, dev, "serial")
	if serial == "" {
		serial = readFile(sysblock, dev, "device/serial")
	}

	var readOnlyBool bool

	readOnly := readFile(sysblock, dev, "ro")
	if readOnly == "1" {
		readOnlyBool = true
	}

	return &Disk{
		DeviceName: fmt.Sprintf("/dev/%s", dev),
		Size:       size,
		Model:      readFile(sysblock, dev, "device/model"),
		Name:       readFile(sysblock, dev, "device/name"),
		Serial:     serial,
		Modalias:   readFile(sysblock, dev, "device/modalias"),
		WWID:       wwid,
		UUID:       uuid,
		Type:       diskType,
		BusPath:    busPath,
		ReadOnly:   readOnlyBool,
	}
}

// Find disk matching provided spec.
// string parameters may include wildcards.
func Find(matchers ...Matcher) (*Disk, error) {
	disks, err := List()
	if err != nil {
		return nil, err
	}

	for _, disk := range disks {
		if Match(disk, matchers...) {
			return disk, nil
		}
	}

	return nil, nil //nolint:nilnil
}

// Matcher is a function that can handle some custom disk matching logic.
type Matcher func(disk *Disk) bool

// WithType select disk with type.
func WithType(t Type) Matcher {
	return func(d *Disk) bool {
		return d.Type == t
	}
}

// WithModel select disk with model.
func WithModel(model string) Matcher {
	return func(d *Disk) bool {
		return glob.Glob(model, d.Model)
	}
}

// WithName select disk with name.
func WithName(name string) Matcher {
	return func(d *Disk) bool {
		return glob.Glob(name, d.Name)
	}
}

// WithSerial select disk with serial.
func WithSerial(serial string) Matcher {
	return func(d *Disk) bool {
		return glob.Glob(serial, d.Serial)
	}
}

// WithModalias select disk with modalias.
func WithModalias(modalias string) Matcher {
	return func(d *Disk) bool {
		return glob.Glob(modalias, d.Modalias)
	}
}

// WithWWID select disk with WWID.
func WithWWID(wwid string) Matcher {
	return func(d *Disk) bool {
		return glob.Glob(wwid, d.WWID)
	}
}

// WithUUID select disk with UUID.
func WithUUID(uuid string) Matcher {
	return func(d *Disk) bool {
		return glob.Glob(uuid, d.UUID)
	}
}

// WithBusPath select disk by it's full path.
func WithBusPath(path string) Matcher {
	return func(d *Disk) bool {
		return glob.Glob(path, d.BusPath)
	}
}

// Match checks if the disk matches the spec.
// Spec can contain part of the field and strings can contain wildcards.
// "and" condition is used when this spec is processed.
func Match(disk *Disk, matchers ...Matcher) bool {
	for _, match := range matchers {
		if !match(disk) {
			return false
		}
	}

	return true
}
