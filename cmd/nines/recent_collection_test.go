package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCollectRecentFlagAndExclusiveRange(t *testing.T) {
	t.Setenv("NINES_CONFIG", t.TempDir()+"/contexts.yaml")
	t.Setenv("NINES_API_KEY", "fixture-key")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/slos/api/collect" || r.Header.Get("X-Nines-Tenant") != "alpha" {
			t.Error(r.Method, r.URL.Path, r.Header.Get("X-Nines-Tenant"))
		}
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || body["recent"] != true {
			t.Error(body)
		}
		_, _ = w.Write([]byte(`{"collected":15}`))
	}))
	defer server.Close()
	var out bytes.Buffer
	if err := run([]string{"collect", "api", "--recent", "--server", server.URL, "--tenant", "alpha", "--agent"}, &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"collected":15`)) && !bytes.Contains(out.Bytes(), []byte(`"collected": 15`)) {
		t.Fatal(out.String())
	}
	if err := run([]string{"collect", "api", "--recent", "--from", "2026-10-23T10:00:00Z", "--server", server.URL, "--tenant", "alpha"}, &out); err == nil {
		t.Fatal("ambiguous range accepted")
	}
	if calls != 1 {
		t.Fatal("invalid request reached source", calls)
	}
}
