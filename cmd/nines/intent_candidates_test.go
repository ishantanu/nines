package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestIntentCandidatesCLIUnitsAndStrictYAML(t *testing.T) {
	cliEnv(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var in v1.IntentCandidateInput
		if json.NewDecoder(r.Body).Decode(&in) != nil || r.Method != "POST" || r.URL.Path != "/api/intents/candidates" || r.Header.Get("X-Nines-Tenant") != "alpha" || in.Threshold != "30ms" || in.Population["service"] != "api" {
			t.Error("scope or units lost")
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(v1.IntentCandidateDiscovery{APIVersion: v1.APIVersion, Tenant: "alpha", State: "needs_input", HumanConfirmationRequired: true}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "request.yaml")
	body := "service: api\nsource: primary\nproject: production\npopulation:\n  service: api\noutcome: latency\nthreshold: 30ms\n"
	for _, extra := range []string{"", "unknown: x\n", "source: other\n", "---\nservice: api\n"} {
		if err := os.WriteFile(path, []byte(body+extra), 0600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		err := run([]string{"--agent", "--server", server.URL, "--tenant", "alpha", "intent", "candidates", "-f", path}, &out)
		if extra == "" {
			if err != nil || !bytes.Contains(out.Bytes(), []byte("needs_input")) {
				t.Fatal(err, out.String())
			}
		} else if err == nil {
			t.Fatal("ambiguous YAML accepted")
		}
	}
	if calls != 1 {
		t.Fatal("invalid YAML reached server", calls)
	}
}
