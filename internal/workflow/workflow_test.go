package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/workflow"
	"github.com/ishantanu/nines/skills"
)

var ctx = context.Background()
var now = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

func manifest() model.Journey {
	return model.Journey{APIVersion: model.JourneyAPIVersion, Kind: "CriticalUserJourney", Metadata: model.JourneyMetadata{Name: "checkout", Project: "production"}, Spec: model.JourneySpec{DisplayName: "Complete checkout", Owner: "commerce", Criticality: "critical", SLOs: []model.JourneySLO{{ID: "availability", Dimension: "availability", Required: true}}, Steps: []model.JourneyStep{{ID: "pay", Name: "Pay", Actor: "user", SLORefs: []string{"availability"}}}}}
}
func platform(t *testing.T, st store.Store) *service.Service {
	t.Helper()
	s := service.New(st, nil)
	s.Now = func() time.Time { return now }
	if err := s.Create(ctx, model.SLO{ID: "availability", Project: "production", Service: "payments", Name: "Availability", Target: .999, WindowSeconds: 3600, StepSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	return s
}
func create(t *testing.T, s *workflow.Service, id string) workflow.Workflow {
	t.Helper()
	j := manifest()
	w, err := s.Create(ctx, workflow.CreateRequest{ID: id, Kind: "propose_journey", Manifest: &j})
	if err != nil {
		t.Fatal(err)
	}
	return w
}
func TestProposalReviewAndReadOnlyInvestigation(t *testing.T) {
	st := store.NewMemory()
	s := workflow.New(platform(t, st), workflow.NewMemory())
	w := create(t, s, "review")
	if w.State != "awaiting_approval" || w.Version != 1 || w.Skill.Digest == "" {
		t.Fatal(w)
	}
	if _, err := st.JourneyRevision(ctx, "checkout", 0); !errors.Is(err, model.ErrJourneyNotFound) {
		t.Fatal("proposal mutated journey", err)
	}
	if _, err := s.Approve(ctx, w.ID, workflow.ApprovalRequest{ExpectedVersion: 1, Digest: "0000000000000000000000000000000000000000000000000000000000000000"}); !errors.Is(err, workflow.ErrConflict) {
		t.Fatal("wrong digest accepted", err)
	}
	approval := workflow.ApprovalRequest{ExpectedVersion: 1, Digest: w.Proposal.Digest, Reason: "Reviewed exact checkout contract"}
	auditCtx := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "commerce", Role: "owner", Subject: "credential:123"})
	result, err := s.Approve(auditCtx, w.ID, approval)
	if err != nil || result.State != "applied" || result.Applied.Revision != 1 || result.Decision.Subject != "credential:123" {
		t.Fatal(result, err)
	}
	again, err := s.Approve(ctx, w.ID, approval)
	if err != nil || again.Version != result.Version {
		t.Fatal("approval retry changed state", again, err)
	}
	if _, err = s.Reject(ctx, w.ID, workflow.RejectionRequest{ExpectedVersion: again.Version, Reason: "Too late"}); !errors.Is(err, workflow.ErrConflict) {
		t.Fatal("applied proposal rejected", err)
	}
	investigation, err := s.Create(ctx, workflow.CreateRequest{ID: "inspect", Kind: "investigate", JourneyID: "checkout"})
	if err != nil || investigation.State != "completed" {
		t.Fatal(investigation, err)
	}
	var unknown bool
	for _, f := range investigation.Findings {
		if f.Code == "slo_status" && f.Snapshot.Reliability == nil && f.Severity == "warning" {
			unknown = true
		}
	}
	if !unknown {
		t.Fatal("missing data was not reported unknown", investigation)
	}
	history, _ := st.JourneyHistory(ctx, "checkout")
	if len(history) != 1 {
		t.Fatal("investigation mutated history", history)
	}
	same, err := s.Create(ctx, workflow.CreateRequest{ID: "inspect", Kind: "investigate", JourneyID: "checkout"})
	if err != nil || !same.CreatedAt.Equal(investigation.CreatedAt) {
		t.Fatal("retry not stable", err)
	}
	if _, err = s.Create(ctx, workflow.CreateRequest{ID: "inspect", Kind: "investigate", JourneyID: "checkout", Note: "different"}); !errors.Is(err, workflow.ErrConflict) {
		t.Fatal("ID reuse accepted", err)
	}
	investigation.Findings[0].Message = "tampered"
	saved, _ := s.Get(ctx, "inspect")
	if saved.Findings[0].Message == "tampered" {
		t.Fatal("returned findings alias stored data")
	}
}
func TestRejectAndConcurrentDecisions(t *testing.T) {
	for iteration := 0; iteration < 12; iteration++ {
		st := store.NewMemory()
		s := workflow.New(platform(t, st), workflow.NewMemory())
		w := create(t, s, "race")
		var wg sync.WaitGroup
		wg.Go(func() {
			_, err := s.Approve(ctx, w.ID, workflow.ApprovalRequest{ExpectedVersion: 1, Digest: w.Proposal.Digest})
			if err != nil && !errors.Is(err, workflow.ErrConflict) {
				t.Error(err)
			}
		})
		wg.Go(func() {
			_, err := s.Reject(ctx, w.ID, workflow.RejectionRequest{ExpectedVersion: 1, Reason: "Reject this change"})
			if err != nil && !errors.Is(err, workflow.ErrConflict) {
				t.Error(err)
			}
		})
		wg.Wait()
		result, err := s.Get(ctx, w.ID)
		if err != nil {
			t.Fatal(err)
		}
		_, journeyErr := st.JourneyRevision(ctx, "checkout", 0)
		switch result.State {
		case "applied":
			if journeyErr != nil {
				t.Fatal(journeyErr)
			}
		case "rejected":
			if !errors.Is(journeyErr, model.ErrJourneyNotFound) {
				t.Fatal("rejected workflow applied", journeyErr)
			}
		default:
			t.Fatal("unexpected terminal state", result)
		}
	}
	st := store.NewMemory()
	s := workflow.New(platform(t, st), workflow.NewMemory())
	w := create(t, s, "reject")
	first, err := s.Reject(ctx, w.ID, workflow.RejectionRequest{ExpectedVersion: 1, Reason: "No"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Reject(ctx, w.ID, workflow.RejectionRequest{ExpectedVersion: 1, Reason: "No"})
	if err != nil || again.Version != first.Version {
		t.Fatal(again, err)
	}
	if _, err = s.Approve(ctx, w.ID, workflow.ApprovalRequest{ExpectedVersion: first.Version, Digest: w.Proposal.Digest}); !errors.Is(err, workflow.ErrConflict) {
		t.Fatal("rejected workflow approved", err)
	}
}
func TestStaleProposalNeverRebases(t *testing.T) {
	st := store.NewMemory()
	p := platform(t, st)
	s := workflow.New(p, workflow.NewMemory())
	w := create(t, s, "stale")
	changed := manifest()
	changed.Spec.Description = "Manual concurrent change"
	if _, err := p.ApplyJourney(ctx, changed, 0); err != nil {
		t.Fatal(err)
	}
	result, err := s.Approve(ctx, w.ID, workflow.ApprovalRequest{ExpectedVersion: 1, Digest: w.Proposal.Digest})
	if err != nil || result.State != "conflicted" {
		t.Fatal(result, err)
	}
	current, _ := st.JourneyRevision(ctx, "checkout", 0)
	if current.Manifest.Spec.Description != changed.Spec.Description {
		t.Fatal("concurrent change overwritten")
	}
}

var errInjected = errors.New("simulated crash before workflow status commit")

type failRepository struct {
	workflow.Repository
	beforeApply bool
	used        atomic.Bool
}

func (r *failRepository) Update(ctx context.Context, id string, fn func(*workflow.Workflow) error) (workflow.Workflow, error) {
	return r.Repository.Update(ctx, id, func(w *workflow.Workflow) error {
		if r.beforeApply && w.State == "approved" && !r.used.Swap(true) {
			return errInjected
		}
		if err := fn(w); err != nil {
			return err
		}
		if !r.beforeApply && w.State == "applied" && !r.used.Swap(true) {
			return errInjected
		}
		return nil
	})
}
func TestApprovalCrashRecovery(t *testing.T) {
	t.Run("memory", func(t *testing.T) { crashRecovery(t, store.NewMemory(), workflow.NewMemory(), nil) })
	t.Run("postgres", func(t *testing.T) {
		st, repo, dsn := postgres(t)
		crashRecovery(t, st, repo, func() (store.Store, workflow.Repository) {
			st2, err := store.NewPostgres(ctx, dsn)
			if err != nil {
				t.Fatal(err)
			}
			repo2, err := workflow.NewPostgres(ctx, dsn)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(st2.Close)
			t.Cleanup(repo2.Close)
			return st2, repo2
		})
	})
}
func crashRecovery(t *testing.T, st store.Store, repo workflow.Repository, reopen func() (store.Store, workflow.Repository)) {
	t.Helper()
	p := platform(t, st)
	failing := &failRepository{Repository: repo}
	s := workflow.New(p, failing)
	w := create(t, s, "crash")
	if _, err := s.Approve(ctx, w.ID, workflow.ApprovalRequest{ExpectedVersion: 1, Digest: w.Proposal.Digest}); !errors.Is(err, errInjected) {
		t.Fatal("crash did not occur", err)
	}
	saved, err := repo.Get(ctx, w.ID)
	if err != nil || saved.State != "approved" || saved.Decision == nil {
		t.Fatal("approval not durable", saved, err)
	}
	applied, err := p.Journeys.JourneyRevision(ctx, "checkout", 0)
	if err != nil || applied.Revision != 1 {
		t.Fatal("journey not committed", applied, err)
	}
	// A later legitimate change cannot erase the receipt of the approved operation.
	later := manifest()
	later.Spec.Description = "after crash"
	if _, err = p.ApplyJourney(ctx, later, 1); err != nil {
		t.Fatal(err)
	}
	if reopen != nil {
		st, repo = reopen()
		p = service.New(st, nil)
		p.Now = func() time.Time { return now }
	}
	restarted := workflow.New(p, repo)
	if err = restarted.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err := restarted.Get(ctx, w.ID)
	if err != nil || recovered.State != "applied" || recovered.Applied.Revision != 1 {
		t.Fatal("did not recover exact receipt", recovered, err)
	}
	history, _ := p.Journeys.JourneyHistory(ctx, "checkout")
	if len(history) != 2 {
		t.Fatal("recovery applied twice", history)
	}
	if err = restarted.Recover(ctx); err != nil {
		t.Fatal(err)
	}
}
func TestIdenticalOtherWriterIsNotOurReceipt(t *testing.T) {
	st := store.NewMemory()
	p := platform(t, st)
	repo := workflow.NewMemory()
	failing := &failRepository{Repository: repo, beforeApply: true}
	s := workflow.New(p, failing)
	w := create(t, s, "precommit")
	if _, err := s.Approve(ctx, w.ID, workflow.ApprovalRequest{ExpectedVersion: 1, Digest: w.Proposal.Digest}); !errors.Is(err, errInjected) {
		t.Fatal(err)
	}
	if _, err := p.ApplyJourney(ctx, w.Proposal.Manifest, 0); err != nil {
		t.Fatal(err)
	}
	recovered := workflow.New(p, repo)
	if err := recovered.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	final, _ := recovered.Get(ctx, w.ID)
	if final.State != "conflicted" || final.Applied != nil {
		t.Fatal("identical other writer misattributed", final)
	}
}
func TestWorkflowHTTPAndSkillCatalog(t *testing.T) {
	st := store.NewMemory()
	s := workflow.New(platform(t, st), workflow.NewMemory())
	mux := http.NewServeMux()
	workflow.Mount(mux, s)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	response := request("GET", "/api/skills", "")
	var catalog []skills.Skill
	if err := json.NewDecoder(response.Body).Decode(&catalog); err != nil || len(catalog) != 2 || len(catalog[0].Digest) != 64 || catalog[0].Instructions == "" {
		t.Fatal(catalog, err)
	}
	for _, body := range []string{`{"id":"test","kind":"investigate","tenant":"other"}`, `{"id":"test","kind":"investigate"} {}`, `{"id":"test","kind":"run_shell"}`} {
		r := request("POST", "/api/workflows", body)
		if r.Code != 400 {
			t.Fatal("invalid request accepted", body, r.Code)
		}
	}
	r := request("POST", "/api/workflows", `{"id":"test","kind":"investigate"}`)
	if r.Code != 201 {
		t.Fatal(r.Code)
	}
	r = request("GET", "/api/workflows/missing", "")
	if r.Code != 404 {
		t.Fatal(r.Code)
	}
}
func TestTenantBoundWorkflowIsolation(t *testing.T) {
	aStore, bStore := store.NewMemory(), store.NewMemory()
	a := workflow.New(platform(t, aStore), workflow.NewMemory())
	b := workflow.New(platform(t, bStore), workflow.NewMemory())
	aProposal := create(t, a, "same-id")
	if _, err := b.Get(ctx, "same-id"); !errors.Is(err, workflow.ErrNotFound) {
		t.Fatal("cross-tenant workflow leaked", err)
	}
	bProposal := create(t, b, "same-id")
	if _, err := a.Approve(ctx, aProposal.ID, workflow.ApprovalRequest{ExpectedVersion: 1, Digest: aProposal.Proposal.Digest}); err != nil {
		t.Fatal(err)
	}
	saved, _ := b.Get(ctx, bProposal.ID)
	if saved.State != "awaiting_approval" {
		t.Fatal("cross-tenant approval", saved)
	}
	if _, err := bStore.JourneyRevision(ctx, "checkout", 0); !errors.Is(err, model.ErrJourneyNotFound) {
		t.Fatal("cross-tenant journey applied", err)
	}
}
func postgres(t *testing.T) (*store.Postgres, workflow.Repository, string) {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	schema := "workflow_test_" + strings.ReplaceAll(model.NewID(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	dsn := u.String()
	st, err := store.NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := workflow.NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		repo.Close()
		st.Close()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	return st, repo, dsn
}

func TestInvestigationBoundsAndAgentEvidence(t *testing.T) {
	st := store.NewMemory()
	p := platform(t, st)
	s := workflow.New(p, workflow.NewMemory())
	contract := manifest()
	cost := 1.0
	contract.Spec.Agentic = &model.AgenticContract{SuccessDefinition: "Complete checkout correctly", MaxCostUSD: &cost, ApprovalRequired: true}
	if _, err := p.ApplyJourney(ctx, contract, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := p.RecordRun(ctx, "checkout", model.RunEvidence{ID: "reported", JourneyRevision: 1, StartedAt: now.Add(-time.Minute), FinishedAt: now, Outcome: "success"}); err != nil {
		t.Fatal(err)
	}
	w, err := s.Create(ctx, workflow.CreateRequest{ID: "evidence", Kind: "investigate", JourneyID: "checkout"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range w.Findings {
		if f.Code == "agent_run_evidence" && f.Severity == "warning" && strings.Contains(f.Message, "incomplete=1") {
			found = true
		}
	}
	if !found {
		t.Fatal("missing evidence converted to success", w)
	}
	for i := 0; i < 200; i++ {
		j := manifest()
		j.Metadata.Name = fmt.Sprintf("journey-%03d", i)
		if _, err = p.ApplyJourney(ctx, j, 0); err != nil {
			t.Fatal(err)
		}
	}
	w, err = s.Create(ctx, workflow.CreateRequest{ID: "bounded", Kind: "investigate"})
	if err != nil {
		t.Fatal(err)
	}
	journeys, truncated := 0, false
	for _, f := range w.Findings {
		if f.Code == "journey_health" {
			journeys++
		}
		if f.Code == "scope_truncated" {
			truncated = true
		}
	}
	if journeys != 200 || !truncated || len(w.Findings) > 1402 {
		t.Fatal("investigation not bounded", journeys, truncated, len(w.Findings))
	}
}
func TestRecoveryCancellationPreservesApproval(t *testing.T) {
	st := store.NewMemory()
	p := platform(t, st)
	repo := workflow.NewMemory()
	s := workflow.New(p, &failRepository{Repository: repo, beforeApply: true})
	w := create(t, s, "cancelled")
	if _, err := s.Approve(ctx, w.ID, workflow.ApprovalRequest{ExpectedVersion: 1, Digest: w.Proposal.Digest}); !errors.Is(err, errInjected) {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	restarted := workflow.New(p, repo)
	if err := restarted.Recover(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled recovery continued", err)
	}
	saved, _ := restarted.Get(ctx, w.ID)
	if saved.State != "approved" {
		t.Fatal("cancel lost approval", saved)
	}
	if _, err := st.JourneyRevision(ctx, "checkout", 0); !errors.Is(err, model.ErrJourneyNotFound) {
		t.Fatal("cancelled recovery mutated journey", err)
	}
	if err := restarted.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	saved, _ = restarted.Get(ctx, w.ID)
	if saved.State != "applied" {
		t.Fatal(saved)
	}
}
