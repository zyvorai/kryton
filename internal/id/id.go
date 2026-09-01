package id

import (
	"crypto/rand"
	"fmt"
)

// New returns a RFC 4122 version 4 UUID using crypto/rand.
func New() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("secure random source unavailable: %w", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
