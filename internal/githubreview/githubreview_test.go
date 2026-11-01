package githubreview

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func artifact() v1.IntentReviewArtifact {
	digest := strings.Repeat("a", 64)
	return v1.IntentReviewArtifact{
		APIVersion: v1.APIVersion, Tenant: "alpha", Conversation: "conversation", Version: 3, CapturedAt: time.Now(), AuditIntegrity: "verified", AuditThrough: 1,
		Status:    v1.IntentWorkflowStatus{APIVersion: v1.APIVersion, Tenant: "alpha", Conversation: "conversation", Version: 3, ChangeSetDigest: digest, Phase: "approval_required", HumanRequired: true},
		ChangeSet: v1.IntentChangeSet{Recommendation: &v1.IntentRecommendation{Summary: "Review the availability objective"}, Digest: digest, State: "proposed", Changes: []v1.IntentDefinitionChange{{Kind: "ServiceLevelObjective", Name: "availability", Action: "update", ExpectedRevision: 2, Digest: digest, Manifest: json.RawMessage(`{"spec":{"target":0.999}}`), Previous: json.RawMessage(`{"spec":{"target":0.99}}`)}}, ApprovalReason: "untrusted ```\n@everyone <script>"},
		Audit:     []v1.IntentAuditEvent{{Sequence: 1, Hash: strings.Repeat("b", 64)}}, Limitations: []string{"Configuration rollback cannot recover consumed error budget."},
	}
}
func plan(t *testing.T) Plan {
	t.Helper()
	p, err := Prepare(artifact(), Target{Repository: "owner/repo", PullRequest: 7, Head: strings.Repeat("c", 40)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestPlanIsStableBoundedAndKeepsDataInJSON(t *testing.T) {
	a := artifact()
	target := Target{"owner/repo", 7, strings.Repeat("c", 40)}
	p, err := Prepare(a, target)
	if err != nil {
		t.Fatal(err)
	}
	a.CapturedAt = a.CapturedAt.Add(time.Hour)
	same, err := Prepare(a, target)
	if err != nil || p.ReportDigest != same.ReportDigest || p.Markdown != same.Markdown {
		t.Fatal("timestamp changed stable review", err)
	}
	if strings.Contains(p.Markdown, "<script>") || !strings.Contains(p.Markdown, "````json") || !strings.Contains(p.Markdown, "0.999") || !strings.Contains(p.Markdown, "0.99") {
		t.Fatal("missing exact diff or unsafe rendering")
	}
	p.Markdown += "changed"
	if p.Validate() == nil {
		t.Fatal("modified plan accepted")
	}
	a.ChangeSet.ApprovalReason = strings.Repeat("x", 60000)
	if _, err = Prepare(a, target); err == nil {
		t.Fatal("oversized evidence accepted")
	}
	a = artifact()
	a.Status.Version++
	if _, err = Prepare(a, target); err == nil {
		t.Fatal("inconsistent version accepted")
	}
	for _, repository := range []string{"../repo", "owner/..", "owner/repo/other", "https://example.com/repo"} {
		if (Target{repository, 7, strings.Repeat("c", 40)}).Validate() == nil {
			t.Fatal(repository)
		}
	}
}
func newServerPublisher(t *testing.T, handler http.HandlerFunc) *Publisher {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	g, err := New("github-fixture-token", nil)
	if err != nil {
		t.Fatal(err)
	}
	g.origin = server.URL
	return g
}
func writePull(w http.ResponseWriter, head string) {
	_ = json.NewEncoder(w).Encode(map[string]any{"number": 7, "state": "open", "head": map[string]string{"sha": head}, "base": map[string]any{"repo": map[string]string{"full_name": "owner/repo"}}})
}
func TestPublishAndRetryAreReadOnlyForExistingSnapshot(t *testing.T) {
	p := plan(t)
	posts := 0
	fresh := 0
	saved := []comment{}
	g := newServerPublisher(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer github-fixture-token" || r.Header.Get("X-GitHub-Api-Version") != "2026-03-10" {
			t.Error("missing fixed authentication/protocol")
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/repos/owner/repo/pulls/7":
			writePull(w, p.Head)
		case r.Method == "GET" && r.URL.Path == "/repos/owner/repo/issues/7/comments":
			_ = json.NewEncoder(w).Encode(saved)
		case r.Method == "POST" && r.URL.Path == "/repos/owner/repo/issues/7/comments":
			posts++
			var in struct{ Body string }
			if json.NewDecoder(r.Body).Decode(&in) != nil || in.Body != p.Markdown {
				t.Error("wrong report")
			}
			saved = []comment{{ID: 42, Body: in.Body}}
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(saved[0])
		default:
			t.Error("unexpected GitHub operation", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	})
	check := func(context.Context) error { fresh++; return nil }
	first, err := g.Publish(context.Background(), p, check)
	if err != nil || first.State != "published" || first.URL != "https://github.com/owner/repo/pull/7#issuecomment-42" {
		t.Fatal(first, err)
	}
	second, err := g.Publish(context.Background(), p, check)
	if err != nil || second.State != "already_published" || posts != 1 || fresh != 2 {
		t.Fatal(second, err, posts, fresh)
	}
}
func TestPublishRejectsStaleHeadOrNinesEvidence(t *testing.T) {
	for _, scenario := range []string{"initial-head", "changed-head", "nines-conflict", "changed-comment"} {
		t.Run(scenario, func(t *testing.T) {
			p := plan(t)
			posts := 0
			pulls := 0
			g := newServerPublisher(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts++
					w.WriteHeader(500)
					return
				}
				if strings.Contains(r.URL.Path, "/pulls/") {
					pulls++
					head := p.Head
					if scenario == "initial-head" || scenario == "changed-head" && pulls > 1 {
						head = strings.Repeat("d", 40)
					}
					writePull(w, head)
					return
				}
				comments := []comment{}
				if scenario == "changed-comment" {
					comments = []comment{{ID: 10, Body: marker(p) + "\nmodified"}}
				}
				_ = json.NewEncoder(w).Encode(comments)
			})
			_, err := g.Publish(context.Background(), p, func(context.Context) error {
				if scenario == "nines-conflict" {
					return ErrConflict
				}
				return nil
			})
			if !errors.Is(err, ErrConflict) || posts != 0 {
				t.Fatal(err, posts)
			}
		})
	}
}
func TestPublicationFailureDoesNotLeakProviderErrorsOrFollowRedirects(t *testing.T) {
	p := plan(t)
	t.Run("post uncertain", func(t *testing.T) {
		g := newServerPublisher(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/pulls/") {
				writePull(w, p.Head)
				return
			}
			if r.Method == "GET" {
				_, _ = w.Write([]byte("[]"))
				return
			}
			w.WriteHeader(500)
			_, _ = w.Write([]byte("private-upstream-response"))
		})
		_, err := g.Publish(context.Background(), p, func(context.Context) error { return nil })
		if !errors.Is(err, ErrUncertain) || strings.Contains(err.Error(), "private-") {
			t.Fatal(err)
		}
	})
	t.Run("redirect", func(t *testing.T) {
		destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("followed redirect") }))
		defer destination.Close()
		g := newServerPublisher(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) })
		if _, err := g.Publish(context.Background(), p, func(context.Context) error { return nil }); err == nil {
			t.Fatal("redirect accepted")
		}
	})
	t.Run("missing freshness", func(t *testing.T) {
		g := newServerPublisher(t, func(w http.ResponseWriter, r *http.Request) { t.Error("request before freshness validation") })
		if _, err := g.Publish(context.Background(), p, nil); err == nil {
			t.Fatal("no current Nines check")
		}
	})
}

func TestBoundedHistoryAndCredentialEchoStopBeforePublication(t *testing.T) {
	t.Run("history", func(t *testing.T) {
		p := plan(t)
		pages := 0
		posts := 0
		g := newServerPublisher(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/pulls/") {
				writePull(w, p.Head)
				return
			}
			if r.Method == "POST" {
				posts++
				w.WriteHeader(500)
				return
			}
			pages++
			comments := make([]comment, 100)
			for i := range comments {
				comments[i] = comment{ID: int64(pages*100 + i + 1), Body: "other"}
			}
			_ = json.NewEncoder(w).Encode(comments)
		})
		if _, err := g.Publish(context.Background(), p, func(context.Context) error { return nil }); err == nil || pages != 10 || posts != 0 {
			t.Fatal(err, pages, posts)
		}
	})
	t.Run("credential", func(t *testing.T) {
		a := artifact()
		a.ChangeSet.ApprovalReason = "github-fixture-token"
		p, err := Prepare(a, Target{"owner/repo", 7, strings.Repeat("c", 40)})
		if err != nil {
			t.Fatal(err)
		}
		g := newServerPublisher(t, func(w http.ResponseWriter, r *http.Request) { t.Error("credential reached GitHub") })
		if _, err = g.Publish(context.Background(), p, func(context.Context) error { return nil }); err == nil {
			t.Fatal("credential echo accepted")
		}
		if !ContainsSecret(`{"value":"quote\\\"secret"}`, "quote\\\"secret") {
			t.Fatal("escaped credential not found")
		}
	})
}
