package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlaygroundSingleFileProfiles(t *testing.T) {
	if os.Getenv("NINES_TEST_COMPOSE_CONFIG") != "1" {
		t.Skip("set NINES_TEST_COMPOSE_CONFIG=1 for Docker Compose configuration acceptance")
	}
	root, e := filepath.Abs("../..")
	if e != nil {
		t.Fatal(e)
	}
	empty := filepath.Join(t.TempDir(), "empty.env")
	if e = os.WriteFile(empty, nil, 0600); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name                     string
		mimir, jev, ai, topology bool
	}{
		{"base", false, false, false, false}, {"mimir", true, false, false, false}, {"jev", false, true, false, false}, {"mimir-jev", true, true, false, false}, {"all-profiles", true, true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"compose", "--project-directory", root, "--env-file", empty, "-p", "nines-config-check", "-f", "compose.playground.yaml"}
			variant, auth, prom, tenants := "tenants", "auth.yaml", "prometheus.yaml", "prometheus"
			if tc.mimir {
				args = append(args, "--profile", "mimir")
				variant = "tenants-mimir"
				auth = "auth-mimir.yaml"
				prom = "prometheus-mimir.yaml"
				tenants = "prometheus,mimir"
			}
			if tc.jev {
				variant += "-jev"
			}
			if tc.ai {
				args = append(args, "--profile", "ai")
			}
			if tc.topology {
				args = append(args, "--profile", "topology")
			}
			args = append(args, "config", "--format", "json")
			cmd := exec.Command("docker", args...)
			cmd.Dir = root
			for _, entry := range os.Environ() {
				key, _, _ := strings.Cut(entry, "=")
				if !strings.HasPrefix(key, "NINES_PLAYGROUND_") && key != "POSTGRES_PASSWORD" && key != "JEV_API_KEY" && key != "DD_API_KEY" && key != "DD_APP_KEY" && key != "NEWRELIC_API_KEY" {
					cmd.Env = append(cmd.Env, entry)
				}
			}
			cmd.Env = append(cmd.Env, "POSTGRES_PASSWORD=synthetic-compose-fixture", "NINES_PLAYGROUND_TENANTS_FILE=./deploy/playground/"+variant+".yaml", "NINES_PLAYGROUND_AUTH_FILE=./deploy/playground/"+auth, "NINES_PLAYGROUND_PROMETHEUS_CONFIG=./deploy/playground/"+prom, "NINES_PLAYGROUND_TENANTS="+tenants)
			aiTarget, topTarget := "disabled.json", "disabled.json"
			if tc.ai {
				aiTarget = "targets.json"
			}
			if tc.topology {
				topTarget = "targets.json"
			}
			cmd.Env = append(cmd.Env, "NINES_PLAYGROUND_AI_TARGETS=./deploy/playground/ai/"+aiTarget, "NINES_PLAYGROUND_TOPOLOGY_TARGETS=./deploy/playground/topology/"+topTarget)
			if tc.jev {
				cmd.Env = append(cmd.Env, "JEV_API_KEY=synthetic-configuration-key")
			}
			out, err := cmd.Output()
			if err != nil {
				t.Fatal("single-file Compose rejected", err)
			}
			var config struct {
				Services map[string]struct {
					Environment map[string]string
					Volumes     []struct{ Source, Target string }
				}
			}
			if err = json.Unmarshal(out, &config); err != nil {
				t.Fatal(err)
			}
			server := config.Services["nines"]
			if server.Environment["TENANTS_FILE"] != "/etc/nines/tenants.yaml" || config.Services["setup"].Environment["NINES_PLAYGROUND_TENANTS"] != tenants {
				t.Fatal("incorrect tenant selection")
			}
			if (server.Environment["JEV_API_KEY"] != "") != tc.jev {
				t.Fatal("unrequested model key")
			}
			for name, want := range map[string]bool{"mimir": tc.mimir, "ai-sample": tc.ai, "graph-gateway": tc.topology} {
				if _, ok := config.Services[name]; ok != want {
					t.Fatalf("profile %s present=%v want=%v", name, ok, want)
				}
			}
			checkMount := func(service, target, name string) {
				t.Helper()
				for _, v := range config.Services[service].Volumes {
					if v.Target == target && filepath.Base(v.Source) == name {
						return
					}
				}
				t.Fatalf("%s missing %s", service, target)
			}
			checkMount("nines", "/etc/nines/tenants.yaml", variant+".yaml")
			checkMount("nines", "/etc/nines/auth.yaml", auth)
			checkMount("prometheus", "/etc/prometheus/prometheus.yml", prom)
			checkMount("prometheus", "/etc/prometheus/ai-targets.json", aiTarget)
			checkMount("prometheus", "/etc/prometheus/topology-targets.json", topTarget)
		})
	}
}
