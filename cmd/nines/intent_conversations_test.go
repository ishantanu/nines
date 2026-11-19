package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestIntentConversationCLIUsesScopedYAMLRequests(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var in struct {
			SystemOneConsent bool `json:"systemOneConsent"`
			ModelConsent     bool `json:"modelConsent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || !in.SystemOneConsent || in.ModelConsent {
			t.Error("CLI lost separate System One consent", err)
		}

		if r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Authorization") != "Bearer agent" {
			t.Error("unscoped conversation operation")
		}
		_, _ = fmt.Fprint(w, `{"apiVersion":"nines.dev/v1alpha1","tenant":"alpha","id":"chat","version":2}`)
	}))
	defer server.Close()
	t.Setenv("NINES_URL", server.URL)
	t.Setenv("NINES_TENANT", "alpha")
	t.Setenv("API_TOKEN", "agent")
	file := filepath.Join(t.TempDir(), "run.yaml")
	if err := os.WriteFile(file, []byte("id: prepare\nexpectedVersion: 1\nmode: guided\nmodelConsent: false\nsystemOneConsent: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if e := run([]string{"intent", "conversations", "run", "chat", "-f", file, "-o", "yaml"}, &out); e != nil || !strings.Contains(out.String(), "apiVersion: nines.dev/v1alpha1") {
		t.Fatal(out.String(), e)
	}
	if err := os.WriteFile(file, []byte("id: prepare\nexpectedVersion: 1\nmode: guided\nmodelConsent: false\ntenant: beta\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if e := run([]string{"intent", "conversations", "run", "chat", "-f", file}, &out); e == nil || calls != 1 {
		t.Fatal("unknown scope field reached API", e)
	}
}

func TestIntentReviewMarkdownKeepsUntrustedTextInFence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/intent-conversations/chat/review-artifact" || r.Header.Get("X-Nines-Tenant") != "alpha" {
			t.Error("unexpected request")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "nines.dev/v1alpha1", "tenant": "alpha", "conversation": "chat", "version": 3, "auditIntegrity": "verified", "changeSet": map[string]any{"approvalReason": "triple ``` followed by <script>", "recommendation": map[string]any{"reviewPacket": map[string]any{"goal": map[string]any{"spec": map[string]any{"description": "reviewed goal"}}, "destination": map[string]any{"state": "not_verified"}}}}})
	}))
	defer server.Close()
	t.Setenv("NINES_URL", server.URL)
	t.Setenv("NINES_TENANT", "alpha")
	t.Setenv("API_TOKEN", "fixture")
	var out bytes.Buffer
	if e := run([]string{"intent", "conversations", "report", "chat", "--markdown", "--agent"}, &out); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "reviewPacket") || !strings.Contains(out.String(), "reviewed goal") || !strings.Contains(out.String(), "not_verified") || !strings.Contains(out.String(), "````json\n") || !strings.HasSuffix(out.String(), "\n````\n") || strings.Contains(out.String(), "<script>") {
		t.Fatal(out.String())
	}
}

func TestIntentClarificationCLIPreservesGoalExplicitly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			PreserveGoal bool            `json:"preserveGoal"`
			Goal         json.RawMessage `json:"goal"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || !in.PreserveGoal || len(in.Goal) > 0 || r.URL.Path != "/api/intent-conversations/chat/messages" || r.Header.Get("X-Nines-Tenant") != "alpha" {
			t.Error("clarification request lost preservation or scope", err)
		}
		_, _ = fmt.Fprint(w, `{"apiVersion":"nines.dev/v1alpha1","tenant":"alpha","id":"chat","version":3}`)
	}))
	defer server.Close()
	t.Setenv("NINES_URL", server.URL)
	t.Setenv("NINES_TENANT", "alpha")
	t.Setenv("API_TOKEN", "agent")
	file := filepath.Join(t.TempDir(), "answer.yaml")
	if err := os.WriteFile(file, []byte("id: retries\nexpectedVersion: 2\ntext: Retries count as separate attempts.\npreserveGoal: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"intent", "conversations", "message", "chat", "-f", file, "--agent"}, &out); err != nil {
		t.Fatal(err)
	}
}

func TestUsageCommandNeedsNoRequestFile(t *testing.T) {
	root := rootCommand(&options{}, io.Discard, io.Discard)
	var usage *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "intent" {
			for _, g := range c.Commands() {
				if g.Name() == "conversations" {
					for _, a := range g.Commands() {
						if a.Name() == "usage" {
							usage = a
						}
					}
				}
			}
		}
	}
	if usage == nil {
		t.Fatal("usage command missing")
	}
	if usage.Flags().Lookup("file") != nil {
		t.Fatal("usage must not require a request file")
	}
	if usage.Flags().Lookup("days") == nil {
		t.Fatal("usage must accept --days")
	}
}
