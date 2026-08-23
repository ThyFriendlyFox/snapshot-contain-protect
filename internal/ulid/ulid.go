// Package ulid generates lexicographically sortable identifiers.
package ulid

import (
	"crypto/rand"
	"encoding/binary"
	"sync"
	"time"
)

// crockford is Crockford base32. It omits I, L, O and U.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var (
	mu       sync.Mutex
	lastMs   uint64
	lastRand [10]byte
)

// New returns a 26-character ULID: 48 bits of millisecond time, 80 bits of
// randomness. Two calls in the same millisecond stay ordered because the
// random part increments instead of being redrawn.
func New() string {
	return newAt(time.Now())
}

func newAt(t time.Time) string {
	ms := uint64(t.UnixMilli())

	mu.Lock()
	if ms == lastMs {
		increment(&lastRand)
	} else {
		lastMs = ms
		if _, err := rand.Read(lastRand[:]); err != nil {
			// crypto/rand does not fail on any supported platform. Fall back to
			// the clock so an identifier is still produced.
			binary.BigEndian.PutUint64(lastRand[:8], uint64(t.UnixNano()))
		}
	}
	r := lastRand
	mu.Unlock()

	var raw [16]byte
	raw[0] = byte(ms >> 40)
	raw[1] = byte(ms >> 32)
	raw[2] = byte(ms >> 24)
	raw[3] = byte(ms >> 16)
	raw[4] = byte(ms >> 8)
	raw[5] = byte(ms)
	copy(raw[6:], r[:])

	return encode(raw)
}

func increment(b *[10]byte) {
	for i := len(b) - 1; i >= 0; i-- {
		b[i]++
		if b[i] != 0 {
			return
		}
	}
}

// encode writes 128 bits as 26 base32 characters, high bits first.
func encode(raw [16]byte) string {
	out := make([]byte, 26)
	var bits, acc uint32
	pos := 25
	for i := 15; i >= 0; i-- {
		acc |= uint32(raw[i]) << bits
		bits += 8
		for bits >= 5 {
			out[pos] = crockford[acc&31]
			pos--
			acc >>= 5
			bits -= 5
		}
	}
	if pos >= 0 {
		out[pos] = crockford[acc&31]
	}
	return string(out)
}
