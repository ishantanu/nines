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

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestIntentEvidenceAttachmentCLIUsesExplicitOperationalYAML(t *testing.T) {
	cliEnv(t)
	t.Setenv("NINES_API_KEY", "fixture-token")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/intent-conversations/chat/evidence" || r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("unexpected evidence route or auth")
		}
		var in v1.RecordIntentEvidence
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || !in.ReadTelemetry || in.ExpectedVersion != 7 {
			t.Error("request lost explicit consent/version")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v1.IntentConversation{APIVersion: v1.APIVersion, Tenant: "alpha", IntentConversationSummary: v1.IntentConversationSummary{ID: "chat", Version: 8}, PreparationEvidence: &v1.IntentPreparationEvidence{CapturedVersion: 7}})
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "evidence.yaml")
	body := "expectedVersion: 7\nintentDigest: " + strings.Repeat("a", 64) + "\nreadTelemetry: true\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	args := []string{"--agent", "--server", server.URL, "--tenant", "alpha", "intent", "conversations", "evidence", "chat", "-f", path}
	if err := run(args, &out); err != nil || !bytes.Contains(out.Bytes(), []byte("preparationEvidence")) || calls != 1 {
		t.Fatal(out.String(), calls, err)
	}
	if err := os.WriteFile(path, []byte(body+"report: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(args, &out); err == nil || calls != 1 {
		t.Fatal("client supplied report accepted")
	}
}
