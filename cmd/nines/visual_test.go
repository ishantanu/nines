package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVisualCLIRequiresServerAndExplicitSharing(t *testing.T) {
	cliEnv(t)
	var out bytes.Buffer
	for _, args := range [][]string{{"visual", "list", "checkout", "--offline"}, {"visual", "assess", "checkout", "--offline"}, {"visual", "review", "checkout", "receipt", "--offline"}} {
		if e := run(args, &out); e == nil || !strings.Contains(e.Error(), "requires a server") {
			t.Fatal(args, e)
		}
	}
	if e := run([]string{"visual", "assess", "checkout", "--server", "http://127.0.0.1:1", "--tenant", "default", "--agent"}, &out); e == nil || !strings.Contains(e.Error(), "explicit --consent") {
		t.Fatal("missing consent attempted network", e)
	}
	if e := run([]string{"replays", "delete", "capture", "--server", "http://127.0.0.1:1", "--tenant", "default"}, &out); e == nil || !strings.Contains(e.Error(), "confirmation") {
		t.Fatal("missing delete confirmation attempted network", e)
	}
}
