package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReviewSkillCLIExactVersionYAML(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/review-skills/measurement-review/versions/1.0.0" || r.Header.Get("X-Nines-Tenant") != "alpha" {
			t.Error("unscoped skill request")
		}
		_, _ = fmt.Fprint(w, `{"apiVersion":"nines.dev/v1alpha1","tenant":"alpha","item":{"apiVersion":"nines.dev/v1alpha1","kind":"ReviewSkill","metadata":{"name":"measurement-review","version":"1.0.0"}}}`)
	}))
	defer server.Close()
	var out bytes.Buffer
	args := []string{"review-skills", "show", "measurement-review", "-version", "1.0.0", "-server", server.URL, "-tenant", "alpha", "-o", "yaml"}
	if err := run(args, &out); err != nil || !strings.Contains(out.String(), "kind: ReviewSkill") || !strings.Contains(out.String(), "apiVersion: nines.dev/v1alpha1") {
		t.Fatal(out.String(), err)
	}
	args[4] = "latest"
	if err := run(args, &out); err == nil || calls != 1 {
		t.Fatal("non-exact version reached server")
	}
}
