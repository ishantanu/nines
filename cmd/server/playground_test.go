package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/tenancy"
)

func TestPlaygroundJevTenantVariants(t *testing.T) {
	clearIntentProviderEnvironment(t)
	t.Setenv("API_TOKEN", "synthetic-playground-owner-token")
	t.Setenv("NINES_SOURCE_HOSTS", "")
	t.Setenv("JEV_API_KEY", "")
	t.Setenv("TYPESAFE_API_KEY", "")
	for _, pair := range [][2]string{{"tenants.yaml", "tenants-jev.yaml"}, {"tenants-mimir.yaml", "tenants-mimir-jev.yaml"}} {
		t.Run(pair[0], func(t *testing.T) {
			base := filepath.Join("..", "..", "deploy", "playground", pair[0])
			opt := filepath.Join("..", "..", "deploy", "playground", pair[1])
			ordinary, err := tenancy.Load(base, os.Getenv)
			if err != nil {
				t.Fatal(err)
			}
			optional, err := tenancy.Load(opt, os.Getenv)
			if err != nil || len(optional) != len(ordinary) {
				t.Fatal("invalid optional tenant configuration", err)
			}
			for i := range ordinary {
				if ordinary[i].Config.SystemOne != nil || optional[i].Config.SystemOne == nil {
					t.Fatal("Jev must be explicit in only the optional variant")
				}
				route := optional[i].Config.SystemOne
				if route.Provider != "typesafe" || route.Model != "jev-1.14.0" || route.APIKeyEnv != "JEV_API_KEY" {
					t.Fatal("unexpected Jev route", route)
				}
				optional[i].Config.SystemOne = nil
				if !reflect.DeepEqual(ordinary[i], optional[i]) {
					t.Fatal("Jev variant changed tenant access, measurement source or rule destination")
				}
			}
			start := func(file string, enabled bool) {
				t.Helper()
				runtimes, err := initTenants(context.Background(), file, false, true, "")
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					for _, runtime := range runtimes {
						runtime.Close()
					}
				}()
				for _, runtime := range runtimes {
					if (runtime.Workspace.IntentConversations.SystemOne != nil) != enabled || runtime.Workspace.IntentConversations.Model != nil {
						t.Fatal("unexpected external-model enablement")
					}
				}
			}
			// No network work is scheduled: these exercise startup, not providers.
			start(base, false)
			if _, err := initTenants(context.Background(), opt, false, true, ""); err == nil {
				t.Fatal("optional Jev accepted a missing key")
			}
			t.Setenv("JEV_API_KEY", "synthetic-jev-configuration-key")
			start(base, false)
			start(opt, true)
		})
	}
}

func TestPlaygroundComposeSelection(t *testing.T) {
	if os.Getenv("NINES_TEST_COMPOSE_CONFIG") != "1" {
		t.Skip("set NINES_TEST_COMPOSE_CONFIG=1 for Docker Compose configuration acceptance")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(t.TempDir(), "empty.env")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, mimir := range []bool{false, true} {
		for _, jev := range []bool{false, true} {
			name := "prometheus"
			if mimir {
				name = "mimir"
			}
			if jev {
				name += "+jev"
			}
			t.Run(name, func(t *testing.T) {
				args := []string{"compose", "--project-directory", root, "--env-file", empty, "-p", "nines-config-check", "-f", "compose.playground.yaml"}
				if mimir {
					args = append(args, "-f", "compose.playground.mimir.yaml", "--profile", "mimir")
				}
				if jev {
					args = append(args, "-f", "compose.playground.jev.yaml")
				}
				args = append(args, "config", "--format", "json")
				cmd := exec.Command("docker", args...)
				cmd.Dir = root
				// Never interpolate a developer's .env or real provider credentials.
				for _, entry := range os.Environ() {
					if !strings.HasPrefix(entry, "JEV_API_KEY=") && !strings.HasPrefix(entry, "POSTGRES_PASSWORD=") {
						cmd.Env = append(cmd.Env, entry)
					}
				}
				cmd.Env = append(cmd.Env, "POSTGRES_PASSWORD=synthetic-config-check")
				withoutKey := append([]string(nil), cmd.Env...)
				if jev {
					cmd.Env = append(cmd.Env, "JEV_API_KEY=synthetic-config-check")
				}
				output, err := cmd.Output()
				if err != nil {
					t.Fatal("Compose configuration rejected", err)
				}
				var config struct {
					Services map[string]struct {
						Environment map[string]string                 `json:"environment"`
						Volumes     []struct{ Source, Target string } `json:"volumes"`
					} `json:"services"`
				}
				if err := json.Unmarshal(output, &config); err != nil {
					t.Fatal(err)
				}
				server := config.Services["nines"]
				target, filename := "/etc/nines/tenants.yaml", "tenants.yaml"
				if mimir {
					filename = "tenants-mimir.yaml"
				}
				if jev {
					target = "/etc/nines/tenants-jev.yaml"
					filename = strings.TrimSuffix(filename, ".yaml") + "-jev.yaml"
				}
				if server.Environment["TENANTS_FILE"] != target || (server.Environment["JEV_API_KEY"] != "") != jev {
					t.Fatal("Compose selected incorrect Jev enablement")
				}
				matched := false
				for _, volume := range server.Volumes {
					if volume.Target == target {
						matched = filepath.Base(volume.Source) == filename
					}
				}
				if !matched {
					t.Fatal("Compose selected incorrect tenant file")
				}
				if jev {
					missingKey := exec.Command("docker", args...)
					missingKey.Dir = root
					missingKey.Env = withoutKey
					failure, err := missingKey.CombinedOutput()
					if err == nil || !strings.Contains(string(failure), "JEV_API_KEY") {
						t.Fatal("optional Jev override did not reject its missing key")
					}
				}
			})
		}
	}
}

func TestTopologyPlaygroundComposeMountsInstrumentedTargets(t *testing.T) {
	if os.Getenv("NINES_TEST_COMPOSE_CONFIG") != "1" {
		t.Skip("set NINES_TEST_COMPOSE_CONFIG=1 for Compose acceptance")
	}
	root, e := filepath.Abs(filepath.Join("..", ".."))
	if e != nil {
		t.Fatal(e)
	}
	empty := filepath.Join(t.TempDir(), "empty.env")
	if e = os.WriteFile(empty, nil, 0600); e != nil {
		t.Fatal(e)
	}
	for _, mimir := range []bool{false, true} {
		t.Run(fmt.Sprintf("mimir-%t", mimir), func(t *testing.T) {
			args := []string{"compose", "--project-directory", root, "--env-file", empty, "-p", "nines-topology-config-check", "-f", "compose.playground.yaml", "-f", "compose.playground.topology.yaml"}
			if mimir {
				args = append(args, "-f", "compose.playground.mimir.yaml", "--profile", "mimir")
			}
			args = append(args, "config", "--format", "json")
			cmd := exec.Command("docker", args...)
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "POSTGRES_PASSWORD=synthetic-topology-config", "NINES_PLAYGROUND_PASSWORD=synthetic-local-test")
			raw, e := cmd.Output()
			if e != nil {
				t.Fatal("topology compose configuration invalid", e)
			}
			var config struct {
				Services map[string]struct {
					Image   string `json:"image"`
					Volumes []struct {
						Source string `json:"source"`
						Target string `json:"target"`
					} `json:"volumes"`
				} `json:"services"`
			}
			if e = json.Unmarshal(raw, &config); e != nil {
				t.Fatal(e)
			}
			for _, name := range []string{"graph-gateway", "graph-checkout", "graph-payments", "graph-traffic"} {
				if config.Services[name].Image != "nines-service-graph:local" {
					t.Fatal("missing instrumented example", name)
				}
			}
			targetFound := false
			for _, v := range config.Services["prometheus"].Volumes {
				if v.Target == "/etc/prometheus/topology-targets.json" {
					targetFound = filepath.Base(v.Source) == "targets.json"
				}
			}
			if !targetFound {
				t.Fatal("topology target-file override not selected")
			}
			_, hasMimir := config.Services["mimir"]
			if hasMimir != mimir {
				t.Fatal("unexpected Mimir selection")
			}
		})
	}
}
