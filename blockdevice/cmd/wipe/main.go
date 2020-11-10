// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"flag"
	"log"

	"github.com/talos-systems/go-blockdevice/blockdevice"
)

func main() {
	fastWipe := flag.Bool("fast", true, "Use fast wipe instead of full wipe")
	flag.Parse()

	for _, dev := range flag.Args() {
		dev := dev

		log.Printf("Processing device %q", dev)

		var method string

		if err := func() error {
			bd, err := blockdevice.Open(dev)
			if err != nil {
				return err
			}

			defer bd.Close() //nolint: errcheck

			if *fastWipe {
				err = bd.FastWipe()
				method = "fast"
			} else {
				method, err = bd.Wipe()
			}

			return err
		}(); err != nil {
			log.Fatalf("Failed wiping %q: %s", dev, err)
		}

		log.Printf("Successfully wiped %q via %q", dev, method)
	}
}
