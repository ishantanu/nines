package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitCommandsRequireServerAndExplicitReviewedValues(t *testing.T) {
	for _, args := range [][]string{{"git", "info"}, {"git", "proposals", "api"}, {"git", "propose", "-f", "x.yaml", "--digest", strings.Repeat("a", 64), "--expected-revision", "0"}} {
		var out bytes.Buffer
		if e := run(append(args, "--offline", "--agent"), &out); e == nil || !strings.Contains(e.Error(), "requires a server") {
			t.Fatalf("%v: %v", args, e)
		}
	}
	var out bytes.Buffer
	if e := run([]string{"git", "propose", "--server", "http://127.0.0.1:1", "--tenant", "default", "--agent"}, &out); e == nil {
		t.Fatal("propose without a file and digest must be refused")
	}
	file := filepath.Join(t.TempDir(), "slo.yaml")
	if err := os.WriteFile(file, []byte("kind: ServiceLevelObjective\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if e := run([]string{"git", "propose", "-f", file, "--digest", strings.Repeat("a", 64), "--server", "http://127.0.0.1:1", "--tenant", "default", "--agent"}, &out); e == nil || !strings.Contains(e.Error(), "expected-revision") {
		t.Fatalf("a missing expected revision must be refused, not defaulted to 0: %v", e)
	}
}

func TestGitProposeSendsTheReviewedRevisionAndDigest(t *testing.T) {
	digest := strings.Repeat("b", 64)
	var header http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Clone()
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"receipt":{"created":true,"path":"slos/x.slo.yaml","pullRequest":{"number":9,"state":"open"}},"definitionDigest":"` + digest + `","expectedRevision":4}`))
	}))
	defer srv.Close()
	file := filepath.Join(t.TempDir(), "slo.yaml")
	if err := os.WriteFile(file, []byte("kind: ServiceLevelObjective\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if e := run([]string{"git", "propose", "-f", file, "--digest", digest, "--expected-revision", "4", "--server", srv.URL, "--tenant", "alpha", "--agent"}, &out); e != nil {
		t.Fatal(e)
	}
	var got struct {
		Receipt struct{ PullRequest struct{ Number int } }
	}
	if json.Unmarshal(out.Bytes(), &got) != nil || got.Receipt.PullRequest.Number != 9 {
		t.Fatalf("%s", out.String())
	}
	if header.Get("If-Match") != `"4"` || header.Get("X-Nines-Plan-Digest") != digest {
		t.Fatalf("%v", header)
	}
}
