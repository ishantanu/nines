package model

import (
	"regexp"
	"testing"
)

func TestNewIDIsUUIDv4(t *testing.T) {
	format := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seen := map[string]bool{}
	for range 128 {
		id := NewID()
		if !format.MatchString(id) || seen[id] {
			t.Fatalf("invalid or duplicate UUIDv4: %q", id)
		}
		seen[id] = true
	}
}
