package main

import (
	"io/fs"
	"testing"
)

func TestTooOpenIgnoresUnixBitsOnlyOnWindows(t *testing.T) {
	defer func(old string) { goos = old }(goos)
	for _, tc := range []struct {
		os   string
		mode fs.FileMode
		want bool
	}{
		{"linux", 0o600, false},
		{"linux", 0o644, true},
		{"darwin", 0o700 | fs.ModeDir, false},
		{"darwin", 0o755 | fs.ModeDir, true},
		// Go reports these for every Windows file and directory; ACLs govern access.
		{"windows", 0o666, false},
		{"windows", 0o777 | fs.ModeDir, false},
	} {
		goos = tc.os
		if got := tooOpen(tc.mode); got != tc.want {
			t.Errorf("%s %v: got %v, want %v", tc.os, tc.mode, got, tc.want)
		}
	}
}
