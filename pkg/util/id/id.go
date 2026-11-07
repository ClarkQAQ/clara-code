// Package id creates opaque identifiers for Clara Core domain objects.
package id

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// New returns an opaque, time-prefixed identifier.
func New(prefix string) string {
	var random [10]byte
	if _, err := rand.Read(random[:]); err != nil {
		panic(fmt.Sprintf("clara-core: read random identifier bytes: %v", err))
	}
	stamp := time.Now().UTC().Format("20060102T150405.000000000")
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return stamp + "_" + hex.EncodeToString(random[:])
	}
	return prefix + "_" + stamp + "_" + hex.EncodeToString(random[:])
}
