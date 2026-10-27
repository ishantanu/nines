package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRetirementCLIRejectsOfflineAndMissingReview(t *testing.T) {
	cliEnv(t)
	var out bytes.Buffer
	for _, action := range []string{"plan", "apply", "status", "retry"} {
		if e := run([]string{"retire", action, "api", "--offline", "--agent"}, &out); e == nil || !strings.Contains(e.Error(), "requires a server") {
			t.Fatal(action, e)
		}
	}
	if e := run([]string{"retire", "apply", "api", "--server", "http://127.0.0.1:1", "--tenant", "default", "--agent"}, &out); e == nil {
		t.Fatal("missing review accepted")
	}
}
