package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/examples"
	"github.com/ishantanu/nines/internal/gitops"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

type fakeGit struct {
	opened []gitops.Proposal
	pulls  []gitops.PullRequest
	err    error
	again  bool
	// file is the repository content; nil means the file is absent.
	file       []byte
	unreadable bool
	fileErr    error
}

func (f *fakeGit) Info() gitops.Info {
	return gitops.Info{Configured: true, Provider: "github", Repository: "acme/slos", BaseBranch: "main", Path: "slos"}
}
func (f *fakeGit) Open(_ context.Context, p gitops.Proposal) (gitops.Receipt, error) {
	if f.err != nil {
		return gitops.Receipt{}, f.err
	}
	f.opened = append(f.opened, p)
	return gitops.Receipt{APIVersion: "nines.dev/v1alpha1", Created: !f.again, Path: "slos/" + p.ID + ".slo.yaml", PullRequest: gitops.PullRequest{Number: 7, URL: "https://github.example/pull/7", State: "open", Branch: "nines/alpha/x"}}, nil
}
func (f *fakeGit) File(_ context.Context, _ string, id string) ([]byte, string, bool, error) {
	if f.fileErr != nil {
		return nil, "slos/" + id + ".slo.yaml", false, f.fileErr
	}
	return f.file, "slos/" + id + ".slo.yaml", f.file != nil || f.unreadable, nil
}
func (f *fakeGit) Proposals(context.Context, string, string, string) ([]gitops.PullRequest, error) {
	return f.pulls, f.err
}

func TestGitProposalsAreReviewedExactlyAuthorizedAndNeverApplied(t *testing.T) {
	git := &fakeGit{}
	a, b := service.New(store.NewMemory(), nil), service.New(store.NewMemory(), nil)
	h, err := NewMulti([]Workspace{
		{ID: "alpha", Name: "Alpha", Service: a, GitOps: git, Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}, {Token: "editor", Role: "editor"}, {Token: "viewer", Role: "viewer"}, {Token: "agent", Role: "agent"}}},
		{ID: "beta", Name: "Beta", Service: b, Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}}},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	call := func(token, tenant, method, path, body string, headers map[string]string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set(tenancy.Header, tenant)
		r.Header.Set("Content-Type", "application/yaml")
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body)
		}
		return w
	}
	plan := func(body string) (digest string, revision string) {
		t.Helper()
		var p struct {
			Digest           string
			ExpectedRevision int64
		}
		w := call("owner", "alpha", "POST", "/api/resources/plan", body, nil, 200)
		if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p.Digest, `"` + string(rune('0'+p.ExpectedRevision)) + `"`
	}
	digest, rev := plan(examples.APIAvailability)
	good := map[string]string{"If-Match": rev, "X-Nines-Plan-Digest": digest}

	// Only owners and editors may propose; viewers and agents may not.
	for _, token := range []string{"viewer", "agent"} {
		call(token, "alpha", "POST", "/api/gitops/proposals", examples.APIAvailability, good, 403)
	}
	// What is proposed must be exactly what was reviewed.
	call("editor", "alpha", "POST", "/api/gitops/proposals", examples.APIAvailability, map[string]string{"If-Match": rev}, 409)
	call("editor", "alpha", "POST", "/api/gitops/proposals", examples.APIAvailability, map[string]string{"If-Match": rev, "X-Nines-Plan-Digest": strings.Repeat("0", 64)}, 409)
	call("editor", "alpha", "POST", "/api/gitops/proposals", examples.APIAvailability, map[string]string{"If-Match": `"5"`, "X-Nines-Plan-Digest": digest}, 409)
	call("editor", "alpha", "POST", "/api/gitops/proposals", examples.APIAvailability, map[string]string{"X-Nines-Plan-Digest": digest}, 400)
	if len(git.opened) != 0 {
		t.Fatal("a refused proposal reached the Git provider")
	}

	w := call("editor", "alpha", "POST", "/api/gitops/proposals", examples.APIAvailability, good, 201)
	if len(git.opened) != 1 || git.opened[0].Tenant != "alpha" || git.opened[0].ID != "api-availability" || git.opened[0].Digest != digest || git.opened[0].Action != "create" || !strings.Contains(string(git.opened[0].Manifest), "api-availability") {
		t.Fatalf("%+v", git.opened)
	}
	var out struct {
		Receipt     gitops.Receipt
		Limitations []string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.Receipt.PullRequest.Number != 7 || len(out.Limitations) == 0 {
		t.Fatalf("%s %v", w.Body, err)
	}
	// Proposing never applies: the SLO does not exist in Nines.
	call("owner", "alpha", "GET", "/api/slos/api-availability/manifest", "", nil, 404)
	// A retry that finds the existing pull request is a 200, not a duplicate.
	git.again = true
	call("owner", "alpha", "POST", "/api/gitops/proposals", examples.APIAvailability, good, 200)
	git.again = false

	// Provider failures are 502 and carry only the generic message.
	git.err = errors.New("GitHub API returned HTTP 403")
	w = call("owner", "alpha", "POST", "/api/gitops/proposals", examples.APIAvailability, good, 502)
	if strings.Contains(w.Body.String(), "secret") {
		t.Fatal(w.Body)
	}
	git.err = nil

	// Status: merged counts as applied only when the live digest matches.
	call("owner", "alpha", "POST", "/api/resources/apply", examples.APIAvailability, map[string]string{"If-Match": `"0"`}, 200)
	rec, err := a.Store.Read(context.Background(), "api-availability")
	if err != nil {
		t.Fatal(err)
	}
	live := model.DefinitionDigest(model.DefinitionFromSLO(rec.SLO))
	git.pulls = []gitops.PullRequest{
		{Number: 1, State: "merged", DefinitionDigest: live},
		{Number: 2, State: "merged", DefinitionDigest: strings.Repeat("a", 64)},
		{Number: 3, State: "open", DefinitionDigest: live},
		{Number: 4, State: "closed", DefinitionDigest: live},
	}
	w = call("viewer", "alpha", "GET", "/api/slos/api-availability/gitops/proposals", "", nil, 200)
	var status struct {
		Configured bool
		Proposals  []struct {
			Number  int
			Applied bool
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil || !status.Configured || len(status.Proposals) != 4 {
		t.Fatalf("%s %v", w.Body, err)
	}
	for i, applied := range []bool{true, false, false, false} {
		if status.Proposals[i].Applied != applied {
			t.Errorf("proposal %d applied=%v, want %v", status.Proposals[i].Number, status.Proposals[i].Applied, applied)
		}
	}

	// A workspace without Git delivery says so and refuses proposals.
	call("owner", "beta", "POST", "/api/gitops/proposals", examples.APIAvailability, good, 404)
	var info gitops.Info
	betaInfo := call("owner", "beta", "GET", "/api/gitops", "", nil, 200)
	if err := json.Unmarshal(betaInfo.Body.Bytes(), &info); err != nil || info.Configured || !strings.Contains(betaInfo.Body.String(), `"tenant":"beta"`) {
		t.Fatal(info, err, betaInfo.Body)
	}
	if err := json.Unmarshal(call("viewer", "alpha", "GET", "/api/gitops", "", nil, 200).Body.Bytes(), &info); err != nil || !info.Configured || info.Repository != "acme/slos" {
		t.Fatal(info, err)
	}
}

func TestGitDriftComparesRepositoryAndLiveByDigestWithoutChoosingASide(t *testing.T) {
	git := &fakeGit{}
	svc := service.New(store.NewMemory(), nil)
	h, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: svc, GitOps: git, Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}, {Token: "viewer", Role: "viewer"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	get := func(want int) v1.GitDrift {
		t.Helper()
		r := httptest.NewRequest("GET", "/api/slos/api-availability/gitops/drift", nil)
		r.Header.Set("Authorization", "Bearer viewer")
		r.Header.Set(tenancy.Header, "alpha")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		var out v1.GitDrift
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	// Neither exists.
	get(404)
	// In the repository but not applied.
	git.file = []byte(examples.APIAvailability)
	if d := get(200); d.State != "not_applied" || d.RepositoryDigest == "" || d.LiveDigest != "" {
		t.Fatalf("%+v", d)
	}
	// Apply the same definition: in sync.
	r := httptest.NewRequest("POST", "/api/resources/apply", strings.NewReader(examples.APIAvailability))
	r.Header.Set("Authorization", "Bearer owner")
	r.Header.Set(tenancy.Header, "alpha")
	r.Header.Set("Content-Type", "application/yaml")
	r.Header.Set("If-Match", `"0"`)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	if d := get(200); d.State != "in_sync" || d.RepositoryDigest != d.LiveDigest || len(d.Limitations) == 0 {
		t.Fatalf("%+v", d)
	}
	// The repository changes: drift is reported as a difference.
	git.file = []byte(strings.Replace(examples.APIAvailability, "target: 0.999", "target: 0.99", 1))
	if d := get(200); d.State != "drifted" || d.RepositoryDigest == d.LiveDigest {
		t.Fatalf("%+v", d)
	}
	// A file for a different SLO, or garbage, is unreadable rather than in sync.
	git.file = []byte("not: a definition")
	if d := get(200); d.State != "unreadable" {
		t.Fatalf("%+v", d)
	}
	git.file = []byte(strings.Replace(examples.APIAvailability, "name: api-availability", "name: other", 1))
	if d := get(200); d.State != "unreadable" {
		t.Fatalf("a file naming another SLO must not count: %+v", d)
	}
	// Absent from the repository while live.
	git.file = nil
	if d := get(200); d.State != "not_in_repo" || d.LiveDigest == "" {
		t.Fatalf("%+v", d)
	}
	// Provider failure is a 502.
	git.fileErr = errors.New("GitHub read unavailable")
	get(502)
}

func TestGitJourneysUseTheSameReviewedProposalDriftAndStatusFlow(t *testing.T) {
	git := &fakeGit{}
	svc := service.New(store.NewMemory(), nil)
	h, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: svc, GitOps: git, Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body string, headers map[string]string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer owner")
		r.Header.Set(tenancy.Header, "alpha")
		r.Header.Set("Content-Type", "application/yaml")
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body)
		}
		return w
	}
	for _, id := range []string{"api-availability", "background-jobs", "checkout-success"} {
		slo := strings.Replace(examples.APIAvailability, "name: api-availability", "name: "+id, 1)
		call("POST", "/api/resources/apply", slo, map[string]string{"If-Match": `"0"`}, 200)
	}
	var plan struct{ Digest string }
	if err := json.Unmarshal(call("POST", "/api/resources/plan", examples.Checkout, nil, 200).Body.Bytes(), &plan); err != nil || plan.Digest == "" {
		t.Fatal(err)
	}
	good := map[string]string{"If-Match": `"0"`, "X-Nines-Plan-Digest": plan.Digest}
	call("POST", "/api/gitops/proposals", examples.Checkout, good, 201)
	if len(git.opened) != 1 || git.opened[0].Kind != "journey" || git.opened[0].ID != "complete-checkout" || git.opened[0].Digest != plan.Digest {
		t.Fatalf("%+v", git.opened[0].ID)
	}
	// The journey is not applied by proposing it.
	call("GET", "/api/journeys/complete-checkout/gitops/proposals", "", nil, 404)

	// Apply it; the repository file then matches, and a merged proposal counts as applied.
	call("POST", "/api/resources/apply", examples.Checkout, map[string]string{"If-Match": `"0"`}, 200)
	git.file = []byte(examples.Checkout)
	var drift v1.GitDrift
	if err := json.Unmarshal(call("GET", "/api/journeys/complete-checkout/gitops/drift", "", nil, 200).Body.Bytes(), &drift); err != nil || drift.State != "in_sync" || drift.RepositoryDigest != plan.Digest || drift.LiveDigest != plan.Digest {
		t.Fatalf("%+v %v", drift, err)
	}
	git.pulls = []gitops.PullRequest{{Number: 3, State: "merged", DefinitionDigest: plan.Digest}}
	var list v1.GitProposalList
	if err := json.Unmarshal(call("GET", "/api/journeys/complete-checkout/gitops/proposals", "", nil, 200).Body.Bytes(), &list); err != nil || len(list.Proposals) != 1 || !list.Proposals[0].Applied {
		t.Fatalf("%+v %v", list, err)
	}
	// A repository file that is an SLO, not this journey, is unreadable.
	git.file = []byte(examples.APIAvailability)
	if err := json.Unmarshal(call("GET", "/api/journeys/complete-checkout/gitops/drift", "", nil, 200).Body.Bytes(), &drift); err != nil || drift.State != "unreadable" {
		t.Fatalf("%+v", drift)
	}
	// Bindings and dependency maps are still not proposed.
	call("POST", "/api/gitops/proposals", "apiVersion: nines.dev/v1alpha1\nkind: nope\n", good, 400)
}

func TestMigrationConvertIsStatelessReadOnlyAndAvailableToEveryRole(t *testing.T) {
	svc := service.New(store.NewMemory(), nil)
	h, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: svc, Grants: []tenancy.Grant{{Token: "viewer", Role: "viewer"}, {Token: "agent", Role: "agent"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	doc := "version: prometheus/v1\nservice: svc\nslos:\n  - name: a\n    objective: 99.9\n    sli:\n      events:\n        error_query: sum(rate(e[{{.window}}]))\n        total_query: sum(rate(t[{{.window}}]))\n"
	call := func(token, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("POST", "/api/migrations/convert", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set(tenancy.Header, "alpha")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		return w
	}
	payload, _ := json.Marshal(v1.MigrationRequest{Tool: "sloth", Content: doc, Project: "payments"})
	for _, token := range []string{"viewer", "agent"} {
		var got v1.MigrationResult
		if err := json.Unmarshal(call(token, string(payload), 200).Body.Bytes(), &got); err != nil || got.Tenant != "alpha" || len(got.Definitions) != 1 || !strings.Contains(got.Definitions[0].YAML, "project: payments") {
			t.Fatalf("%+v %v", got, err)
		}
	}
	// Converting saves nothing.
	if list, _ := svc.Store.List(context.Background()); len(list) != 0 {
		t.Fatal("conversion must not create SLOs")
	}
	call("viewer", `{"tool":"datadog","content":"x"}`, 400)
	call("viewer", `{"tool":"sloth","content":""}`, 400)
	call("viewer", `not json`, 400)
	call("viewer", `{"tool":"sloth","content":"`+strings.Repeat("a", migrateLimit)+`"}`, 400)
}

const migrateLimit = 1<<20 + 100<<10
