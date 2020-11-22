// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//nolint: scopelint
package util_test

import (
	"testing"

	"github.com/talos-systems/go-blockdevice/blockdevice/util"
)

func Test_PartNo(t *testing.T) {
	type args struct {
		devname string
	}

	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "hda1",
			args: args{
				devname: "hda1",
			},
			want: "1",
		},
		{
			name: "hda10",
			args: args{
				devname: "hda10",
			},
			want: "10",
		},
		{
			name: "sda1",
			args: args{
				devname: "sda1",
			},
			want: "1",
		},
		{
			name: "sda10",
			args: args{
				devname: "sda10",
			},
			want: "10",
		},
		{
			name: "nvme1n2p2",
			args: args{
				devname: "nvme1n2p2",
			},
			want: "2",
		},
		{
			name: "nvme1n2p11",
			args: args{
				devname: "nvme1n2p11",
			},
			want: "11",
		},
		{
			name: "vda1",
			args: args{
				devname: "vda1",
			},
			want: "1",
		},
		{
			name: "vda10",
			args: args{
				devname: "vda10",
			},
			want: "10",
		},
		{
			name: "xvda1",
			args: args{
				devname: "xvda1",
			},
			want: "1",
		},
		{
			name: "xvda10",
			args: args{
				devname: "xvda10",
			},
			want: "10",
		},
		{
			name: "loop0p1",
			args: args{
				devname: "loop0p1",
			},
			want: "1",
		},
		{
			name: "loop7p11",
			args: args{
				devname: "loop7p11",
			},
			want: "11",
		},
		{
			name: "loop4p4",
			args: args{
				devname: "loop4p4",
			},
			want: "4",
		},
		{
			name: "mmcblk0p3",
			args: args{
				devname: "mmcblk0p3",
			},
			want: "3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//nolint: errcheck
			if got, _ := util.PartNo(tt.args.devname); got != tt.want {
				t.Errorf("PartNo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_DevnameFromPartname(t *testing.T) {
	type args struct {
		devname string
		partno  string
	}

	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "hda1",
			args: args{
				devname: "hda1",
				partno:  "1",
			},
			want: "hda",
		},
		{
			name: "sda1",
			args: args{
				devname: "sda1",
				partno:  "1",
			},
			want: "sda",
		},
		{
			name: "vda1",
			args: args{
				devname: "vda1",
				partno:  "1",
			},
			want: "vda",
		},
		{
			name: "nvme1n2p11",
			args: args{
				devname: "nvme1n2p11",
				partno:  "11",
			},
			want: "nvme1n2",
		},
		{
			name: "loop0p1",
			args: args{
				devname: "loop0p1",
				partno:  "1",
			},
			want: "loop0",
		},
		{
			name: "loop7p11",
			args: args{
				devname: "loop7p11",
				partno:  "11",
			},
			want: "loop7",
		},
		{
			name: "loop4p1",
			args: args{
				devname: "loop4p1",
				partno:  "4",
			},
			want: "loop4",
		},
		{
			name: "mmcblk0p3",
			args: args{
				devname: "mmcblk0p3",
				partno:  "3",
			},
			want: "mmcblk0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//nolint: errcheck
			if got, _ := util.DevnameFromPartname(tt.args.devname); got != tt.want {
				t.Errorf("DevnameFromPartname() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPartName(t *testing.T) {
	type args struct {
		d string
		n int
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "loop",
			args: args{
				d: "loop0",
				n: 5,
			},
			want: "loop0p5",
		},
		{
			name: "mmc",
			args: args{
				d: "mmcblk0",
				n: 9,
			},
			want: "mmcblk0p9",
		},
		{
			name: "nvme1n2",
			args: args{
				d: "nvme1n2",
				n: 1,
			},
			want: "nvme1n2p1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := util.PartName(tt.args.d, tt.args.n); got != tt.want {
				t.Errorf("PartName() = %v, want %v", got, tt.want)
			}
		})
	}
}
