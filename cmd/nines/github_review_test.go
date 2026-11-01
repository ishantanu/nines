package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/githubreview"
)

type reviewTransport func(*http.Request) (*http.Response, error)

func (f reviewTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGitHubReviewCLIExplicitPlanAndPublish(t *testing.T) {
	digest := strings.Repeat("a", 64)
	head := strings.Repeat("b", 40)
	artifact := v1.IntentReviewArtifact{APIVersion: v1.APIVersion, Tenant: "alpha", Conversation: "chat", Version: 3, AuditIntegrity: "verified", AuditThrough: 1, Status: v1.IntentWorkflowStatus{APIVersion: v1.APIVersion, Tenant: "alpha", Conversation: "chat", Version: 3, ChangeSetDigest: digest}, ChangeSet: v1.IntentChangeSet{Recommendation: &v1.IntentRecommendation{Summary: "Review the availability objective"}, Digest: digest, Changes: []v1.IntentDefinitionChange{{Kind: "ServiceLevelObjective", Name: "api", Manifest: json.RawMessage(`{"spec":{"target":0.999}}`)}}}, Audit: []v1.IntentAuditEvent{{Sequence: 1}}}
	ninesWrites := 0
	ninesReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			ninesWrites++
			w.WriteHeader(500)
			return
		}
		ninesReads++
		if r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Authorization") != "Bearer nines-fixture-credential" {
			t.Error("lost Nines scope")
		}
		w.Header().Set("X-Nines-Tenant", "alpha")
		switch r.URL.Path {
		case "/api/intent-conversations/chat/review-artifact":
			_ = json.NewEncoder(w).Encode(artifact)
		case "/api/intent-conversations/chat/status":
			_ = json.NewEncoder(w).Encode(artifact.Status)
		default:
			t.Error("unexpected Nines endpoint", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	t.Setenv("NINES_URL", server.URL)
	t.Setenv("NINES_TENANT", "alpha")
	t.Setenv("NINES_API_KEY", "nines-fixture-credential")
	t.Setenv("API_TOKEN", "")
	t.Setenv("NINES_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))
	t.Setenv("NINES_CONTEXT", "")
	t.Setenv("NINES_GITHUB_TOKEN", "github-fixture-credential")
	githubCalls := 0
	posts := 0
	body := ""
	transport := reviewTransport(func(r *http.Request) (*http.Response, error) {
		githubCalls++
		if r.URL.Host != "api.github.com" || r.Header.Get("Authorization") != "Bearer github-fixture-credential" {
			t.Error("lost GitHub credential isolation")
		}
		var value any
		status := 200
		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/pulls/"):
			value = map[string]any{"number": 7, "state": "open", "head": map[string]string{"sha": head}, "base": map[string]any{"repo": map[string]string{"full_name": "owner/repo"}}}
		case r.Method == "GET":
			value = []any{}
		case r.Method == "POST":
			posts++
			var in struct{ Body string }
			if json.NewDecoder(r.Body).Decode(&in) != nil {
				t.Fatal("bad GitHub body")
			}
			body = in.Body
			status = 201
			value = map[string]any{"id": 42, "body": in.Body}
		default:
			t.Error("unexpected external mutation", r.Method)
		}
		data, _ := json.Marshal(value)
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(data)), Request: r}, nil
	})
	runCommand := func(args ...string) ([]byte, error) {
		var output, errors bytes.Buffer
		root := rootCommand(&options{githubTransport: transport}, &output, &errors)
		root.SetArgs(args)
		err := root.Execute()
		return output.Bytes(), err
	}
	common := []string{"--repository", "owner/repo", "--pull-request", "7", "--head", head, "--agent"}
	data, err := runCommand(append([]string{"intent", "conversations", "github-plan", "chat"}, common...)...)
	if err != nil {
		t.Fatal(err)
	}
	var plan githubreview.Plan
	if json.Unmarshal(data, &plan) != nil || plan.Validate() != nil || githubCalls != 0 || ninesWrites != 0 {
		t.Fatal("planning changed an external system", string(data), githubCalls, ninesWrites)
	}
	publish := append([]string{"intent", "conversations", "github-publish", "chat"}, common...)
	if _, err = runCommand(publish...); err == nil || githubCalls != 0 || ninesReads != 1 {
		t.Fatal("missing consent reached an API", err)
	}
	confirmed := append(publish, "--confirm-sharing", "--expected-version", "3", "--expected-digest", digest, "--expected-report-digest", plan.ReportDigest)
	data, err = runCommand(confirmed...)
	if err != nil || posts != 1 || body != plan.Markdown || ninesWrites != 0 {
		t.Fatal("publication did not preserve exact review or touched Nines", err, posts, ninesWrites, string(data))
	}
	artifact.Version++
	artifact.Status.Version++
	if _, err = runCommand(confirmed...); err == nil || posts != 1 {
		t.Fatal("stale proposal was published", err, posts)
	}
}
