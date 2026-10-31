package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlaygroundLauncherRemembersSelectionsWithoutCredentials(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	trace := filepath.Join(dir, "trace")
	state := filepath.Join(dir, "settings")
	provider := filepath.Join(dir, "private providers.yaml")
	if err = os.WriteFile(provider, []byte("tenants: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fake := `#!/bin/sh
{
 printf 'CALL\n'
 printf '<%s>\n' "$@"
 printf 'TENANTS=%s\nAUTH=%s\nPROMETHEUS=%s\nAI=%s\nTOPOLOGY=%s\n' "$NINES_PLAYGROUND_TENANTS_FILE" "$NINES_PLAYGROUND_AUTH_FILE" "$NINES_PLAYGROUND_PROMETHEUS_CONFIG" "$NINES_PLAYGROUND_AI_TARGETS" "$NINES_PLAYGROUND_TOPOLOGY_TARGETS"
} >> "$PLAYGROUND_TEST_TRACE"
if [ "${PLAYGROUND_FAIL_WAIT:-}" = 1 ]; then
 for arg in "$@"; do [ "$arg" != wait ] || exit 7; done
fi
`
	if err = os.WriteFile(filepath.Join(dir, "docker"), []byte(fake), 0700); err != nil {
		t.Fatal(err)
	}
	run := func(fail bool, args ...string) string {
		t.Helper()
		cmd := exec.Command(filepath.Join(root, "playground"), args...)
		cmd.Dir = dir
		for _, entry := range os.Environ() {
			key, _, _ := strings.Cut(entry, "=")
			if key != "PATH" && key != "POSTGRES_PASSWORD" && !strings.HasPrefix(key, "NINES_PLAYGROUND_") {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		cmd.Env = append(cmd.Env, "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "POSTGRES_PASSWORD=synthetic-private-password", "JEV_API_KEY=synthetic-private-key", "NINES_PLAYGROUND_STATE_FILE="+state, "PLAYGROUND_TEST_TRACE="+trace)
		if fail {
			cmd.Env = append(cmd.Env, "PLAYGROUND_FAIL_WAIT=1")
		}
		out, e := cmd.CombinedOutput()
		if (e != nil) != fail {
			t.Fatalf("launcher error=%v; output=%s", e, out)
		}
		return string(out)
	}
	run(false, "up", "--mimir", "--jev", "--ai", "--topology", "--providers", provider, "--no-build")
	data, err := os.ReadFile(state)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "synthetic-private") {
		t.Fatal("saved credentials")
	}
	stat, err := os.Stat(state)
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatal("settings must be private", err)
	}
	if err = os.WriteFile(trace, nil, 0600); err != nil {
		t.Fatal(err)
	}
	run(false, "verify")
	run(false, "status")
	output, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<compose.playground.yaml>", "<mimir>", "<ai>", "<topology>", "TENANTS=" + provider, "AUTH=./deploy/playground/auth-mimir.yaml", "PROMETHEUS=./deploy/playground/prometheus-mimir.yaml", "AI=./deploy/playground/ai/targets.json"} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("saved selection missing: %s", want)
		}
	}
	if strings.Contains(string(output), "compose.playground.mimir.yaml") {
		t.Fatal("launcher used an overlay")
	}
	// The script must not report readiness if sample setup fails.
	out := run(true, "up", "--no-build")
	if strings.Contains(out, "Open http") {
		t.Fatal("reported readiness after setup failure")
	}
	// Losing a temporary provider file must not prevent safe logs or shutdown.
	if err = os.Remove(provider); err != nil {
		t.Fatal(err)
	}
	run(false, "logs", "--no-follow")
	run(false, "down")
	run(false, "up", "--prometheus", "--no-jev", "--no-ai", "--no-topology", "--no-providers", "--no-build")
}
