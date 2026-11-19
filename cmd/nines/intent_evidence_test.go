package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestIntentEvidenceCLIUsesScopedProviderReadAndStrictYAML(t *testing.T) {
	cliEnv(t)
	t.Setenv("NINES_API_KEY", "viewer")
	s, _, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(s, "viewer", slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer server.Close()
	in, err := testtelemetry.EvidenceInput(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	body, err := yaml.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "request.yaml")
	for _, suffix := range []string{"", "unknown: x\n", "---\nplan: {}\n"} {
		if err := os.WriteFile(path, append(bytes.Clone(body), []byte(suffix)...), 0600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		err = run([]string{"--agent", "--server", server.URL, "--tenant", "default", "intent", "evidence", "-f", path}, &out)
		if suffix == "" {
			if err != nil || !bytes.Contains(out.Bytes(), []byte("not_requested")) || !bytes.Contains(out.Bytes(), []byte("humanConfirmationRequired")) {
				t.Fatal(out.String(), err)
			}
		} else if err == nil {
			t.Fatal("ambiguous YAML accepted")
		}
	}
	goalBody, err := yaml.Marshal(in.Plan.Intent)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, goalBody, 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = run([]string{"--agent", "--server", server.URL, "--tenant", "default", "intent", "evidence", "-f", path}, &out); err != nil || !bytes.Contains(out.Bytes(), []byte(in.IntentDigest)) {
		t.Fatal("versioned intent workflow failed", out.String(), err)
	}

}
