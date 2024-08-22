// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package luks provides a way to call LUKS2 cryptsetup.
package luks

import "time"

// Option represents luks configuration callback.
type Option func(l *LUKS)

// WithIterTime sets iter-time parameter.
func WithIterTime(value time.Duration) Option {
	return func(l *LUKS) {
		l.iterTime = value
	}
}

// WithPBKDFForceIterations sets pbkdf-force-iterations parameter.
func WithPBKDFForceIterations(value uint) Option {
	return func(l *LUKS) {
		l.pbkdfForceIterations = value
	}
}

// WithPBKDFMemory sets pbkdf-memory parameter.
func WithPBKDFMemory(value uint64) Option {
	return func(l *LUKS) {
		l.pbkdfMemory = value
	}
}

// WithKeySize sets generated key size.
func WithKeySize(value uint) Option {
	return func(l *LUKS) {
		l.keySize = value
	}
}

// WithBlockSize sets block size.
func WithBlockSize(value uint64) Option {
	return func(l *LUKS) {
		l.blockSize = value
	}
}

// WithPerfOptions enables encryption perf options.
func WithPerfOptions(options ...string) Option {
	return func(l *LUKS) {
		l.perfOptions = options
	}
}
