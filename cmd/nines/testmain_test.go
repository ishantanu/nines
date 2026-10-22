package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// CLI tests must never consume a developer's saved context or API credentials.
func TestMain(m *testing.M) {
	dir, e := os.MkdirTemp("", "nines-cli-tests-*")
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	for _, name := range []string{"NINES_API_KEY", "API_TOKEN", "NINES_URL", "NINES_TENANT", "NINES_CONTEXT"} {
		if err := os.Unsetenv(name); err != nil {
			panic(err)
		}
	}
	if err := os.Setenv("NINES_CONFIG", filepath.Join(dir, "config.yaml")); err != nil {
		panic(err)
	}
	code := m.Run()
	if err := os.RemoveAll(dir); err != nil {
		panic(err)
	}
	os.Exit(code)
}
