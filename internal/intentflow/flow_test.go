package intentflow

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	rm "github.com/ishantanu/nines/internal/reviewmodel"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func fixture(t *testing.T, pg bool) (*Service, context.Context, string) {
	t.Helper()
	platform, _, e := testtelemetry.New()
	if e != nil {
		t.Fatal(e)
	}
	platform.TenantID = "alpha"
	var repo Repository
	dsn := ""
	if pg {
		base := os.Getenv("TEST_DATABASE_URL")
		if base == "" {
			t.Skip("set TEST_DATABASE_URL for intent conversation PostgreSQL contracts")
		}
		admin, e := pgxpool.New(context.Background(), base)
		if e != nil {
			t.Fatal(e)
		}
		schema := "intent_flow_" + model.NewID()
		if _, e = admin.Exec(context.Background(), "CREATE SCHEMA "+schema); e != nil {
			t.Fatal(e)
		}
		u, _ := url.Parse(base)
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		dsn = u.String()
		repo, e = NewPostgres(context.Background(), dsn)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() {
			repo.Close()
			if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
				t.Error(err)
				return
			}
			admin.Close()
		})
	} else {
		repo = NewMemory()
	}
	s := New(platform, repo)
	s.Authorize = func(context.Context, v1.InvestigationActor) error { return nil }
	ctx := tenancy.WithPrincipal(context.Background(), tenancy.Principal{TenantID: "alpha", Role: "owner", Subject: "owner", Kind: "user", Name: "Owner"})
	return s, ctx, dsn
}
func input() v1.CreateIntentConversation {
	scope := v1.IntentAuthoringScope{Metadata: v1.Metadata{Name: "intent-created", Project: "test"}, Service: "api", Source: "primary", Population: map[string]string{"service": "api"}, NoDataPolicy: "unknown", Journey: &v1.IntentJourney{Name: "checkout", Step: "request", ExpectedRevision: 1, Required: true}}
	goal := scopeGoal(scope, "99.9% of API attempts succeed over 1 hour")
	goal.Spec.Target = .999
	goal.Spec.WindowSeconds = 3600
	return v1.CreateIntentConversation{ID: "conversation-one", Title: "Checkout intent", Scope: scope, Text: goal.Spec.Description, Goal: &goal}
}
func work(t *testing.T, s *Service, ctx context.Context, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		found, e := s.WorkOne(ctx)
		if !found || e != nil {
			t.Fatalf("step %d: found=%v err=%v", i, found, e)
		}
	}
}
func TestDurableGuidedIntentSelectionBacktestAndNoResourceWrites(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, dsn := fixture(t, pg)
			in := input()
			v, e := s.Create(ctx, in)
			if e != nil {
				t.Fatal(e)
			}
			retry, e := s.Create(ctx, in)
			if e != nil || retry.Version != v.Version {
				t.Fatal(retry, e)
			}
			v, e = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "discovery", ExpectedVersion: v.Version, Mode: "guided"})
			if e != nil {
				t.Fatal(e)
			}
			work(t, s, ctx, 1)
			v, _ = s.Read(ctx, v.ID)
			if v.State != "needs_selection" || v.Plan == nil || len(v.Plan.Candidates) != 1 || v.Selection != nil {
				t.Fatal(v)
			}
			agent := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "alpha", Role: "agent", Subject: "agent", Kind: "api_token"})
			confirm := v1.ConfirmIntentBinding{ExpectedVersion: v.Version, IntentDigest: v.Plan.IntentDigest, Binding: v1.IntentSelection{Name: "requests", ExpectedRevision: 1}, Confirm: true}
			if _, e = s.Select(agent, v.ID, confirm); !errors.Is(e, ErrForbidden) {
				t.Fatal("agent confirmed human semantics", e)
			}
			v, e = s.Select(ctx, v.ID, confirm)
			if e != nil {
				t.Fatal(e)
			}
			cutoff := time.Now().UTC().Truncate(time.Minute).Add(-2 * time.Minute)
			v, e = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "compile", ExpectedVersion: v.Version, Mode: "guided", Backtest: &v1.IntentFlowBacktestWindow{From: cutoff.Add(-10 * time.Minute), To: cutoff, ResolutionSeconds: 300}})
			if e != nil {
				t.Fatal(e)
			}
			work(t, s, ctx, 1)
			v, _ = s.Read(ctx, v.ID)
			first := clone(v.Runs[1].Steps[0])
			if v.Plan == nil || v.Plan.Compiled == nil || v.Plan.Journey == nil {
				t.Fatal(v)
			}
			if pg {
				s.Repository.Close()
				repo, e := NewPostgres(ctx, dsn)
				if e != nil {
					t.Fatal(e)
				}
				defer repo.Close()
				s = New(s.Platform, repo)
				s.Authorize = func(context.Context, v1.InvestigationActor) error { return nil }
			}
			work(t, s, ctx, 2)
			v, e = s.Read(ctx, v.ID)
			if e != nil || v.State != "ready" || v.Backtest == nil || len(v.Backtest.Points) != 2 || len(v.Runs[1].Steps) != 3 || v.Runs[1].Steps[0].Digest != first.Digest || string(v.Runs[1].Steps[0].Result) != string(first.Result) {
				t.Fatal(v, e)
			}
			for _, run := range v.Runs {
				for _, step := range run.Steps {
					if bytesDigest(step.Result) != step.Digest {
						t.Fatal("evidence changed")
					}
				}
			}
			if _, e = s.Platform.Store.Read(ctx, in.Scope.Metadata.Name); !errors.Is(e, model.ErrNotFound) {
				t.Fatal("preparation applied an SLO", e)
			}
			j, e := s.Platform.Journeys.JourneyRevision(ctx, "checkout", 0)
			if e != nil || j.Revision != 1 {
				t.Fatal("preparation changed CUJ", e)
			}
			list, e := s.List(ctx, "")
			b, _ := json.Marshal(list)
			if e != nil || strings.Contains(string(b), "steps") || strings.Contains(string(b), "messages") {
				t.Fatal("list exposed conversation payload", e)
			}
		})
	}
}

type scriptedModel struct {
	route string
	turn  func(context.Context, rm.Request) (rm.Response, error)
}

func (m *scriptedModel) Descriptor() rm.Descriptor {
	return rm.Descriptor{Provider: "fixture", Model: "local-fixture", Protocol: "fixture", RouteDigest: m.route, Local: true}
}
func (m *scriptedModel) Turn(ctx context.Context, r rm.Request, _ func(rm.Progress) error) (rm.Response, error) {
	return m.turn(ctx, r)
}
func response(name string, args any) rm.Response {
	b, _ := json.Marshal(args)
	return rm.Response{Native: json.RawMessage(`{"role":"assistant","private":"provider-private-state"}`), Calls: []rm.Call{{ID: "call-one", Name: name, Arguments: b}}, OutputTokens: 20}
}
func TestModelFlowUsesBoundedToolsAndRequiresExplicitSelection(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	v, e := s.Create(ctx, input())
	if e != nil {
		t.Fatal(e)
	}
	m := &scriptedModel{route: "pinned"}
	m.turn = func(_ context.Context, r rm.Request) (rm.Response, error) {
		if len(r.Tools) > 4 {
			t.Fatal("provider tool limit exceeded")
		}
		if len(r.Messages) == 1 {
			return response("find_measurements", map[string]any{}), nil
		}
		return response("ask_user", map[string]string{"question": "Which observation point matches eligible user attempts?"}), nil
	}
	s.Model = m
	if _, e = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "missing-consent", ExpectedVersion: v.Version, Mode: "model"}); e == nil {
		t.Fatal("model call without consent")
	}
	v, e = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "agent-run", ExpectedVersion: v.Version, Mode: "model", ModelConsent: true})
	if e != nil {
		t.Fatal(e)
	}
	work(t, s, ctx, 4)
	v, e = s.Read(ctx, v.ID)
	if e != nil || v.State != "needs_selection" || v.Selection != nil || v.Plan == nil {
		t.Fatal(v, e)
	}
	public, _ := json.Marshal(v)
	if strings.Contains(string(public), "provider-private-state") || strings.Contains(string(public), "leaseUntil") {
		t.Fatal("private provider state exposed")
	}
	confirm := v1.ConfirmIntentBinding{ExpectedVersion: v.Version, IntentDigest: v.Plan.IntentDigest, Binding: v1.IntentSelection{Name: "requests", ExpectedRevision: 1}, Confirm: true}
	v, e = s.Select(ctx, v.ID, confirm)
	if e != nil {
		t.Fatal(e)
	}
	m.turn = func(_ context.Context, r rm.Request) (rm.Response, error) {
		if len(r.Messages) == 1 {
			return response("compile_intent", map[string]any{}), nil
		}
		return response("finish_proposal", map[string]string{"summary": "Review this attempt-success measurement and its critical user journey link."}), nil
	}
	v, e = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "proposal-run", ExpectedVersion: v.Version, Mode: "model", ModelConsent: true})
	if e != nil {
		t.Fatal(e)
	}
	work(t, s, ctx, 4)
	v, _ = s.Read(ctx, v.ID)
	if v.State != "ready" || v.Plan == nil || v.Plan.Compiled == nil {
		t.Fatal(v)
	}
}
func TestUnsafeModelCallsAndStaleModelSkillRoutesFailClosed(t *testing.T) {
	for _, mode := range []string{"unsafe", "route", "skill", "repeated-model"} {
		t.Run(mode, func(t *testing.T) {
			s, ctx, _ := fixture(t, false)
			v, _ := s.Create(ctx, input())
			m := &scriptedModel{route: "one", turn: func(context.Context, rm.Request) (rm.Response, error) {
				return response("apply_slo", map[string]string{"tenant": "beta"}), nil
			}}
			s.Model = m
			v, e := s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "run-one", ExpectedVersion: v.Version, Mode: "model", ModelConsent: true})
			if e != nil {
				t.Fatal(e)
			}
			switch mode {
			case "route":
				m.route = "changed"
			case "skill":
				_, e = s.Repository.Update(ctx, v.ID, func(r *record, _ time.Time) error { current(r).Skill.Digest = "changed"; return nil })
				if e != nil {
					t.Fatal(e)
				}
			case "repeated-model":
				m.turn = func(context.Context, rm.Request) (rm.Response, error) {
					return response("find_measurements", map[string]any{}), nil
				}
			}
			steps := 1
			if mode == "repeated-model" {
				steps = 13
			}
			work(t, s, ctx, steps)
			v, _ = s.Read(ctx, v.ID)
			if v.State != "failed" {
				t.Fatal(v)
			}
			if _, e = s.Platform.Store.Read(ctx, "intent-created"); !errors.Is(e, model.ErrNotFound) {
				t.Fatal("unsafe call changed resource")
			}
		})
	}
}
func TestConcurrentMessagesCASAndScopeCannotChange(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	v, _ := s.Create(ctx, input())
	var wg sync.WaitGroup
	errorsOut := make(chan error, 2)
	for _, id := range []string{"first", "second"} {
		wg.Go(func() {
			_, e := s.Append(ctx, v.ID, v1.AppendIntentConversation{ID: id, ExpectedVersion: v.Version, Text: "Use 99.95% over 1 hour"})
			errorsOut <- e
		})
	}
	wg.Wait()
	close(errorsOut)
	success, conflict := 0, 0
	for e := range errorsOut {
		if e == nil {
			success++
		} else if errors.Is(e, ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal(success, conflict)
	}
	v, _ = s.Read(ctx, v.ID)
	goal := *input().Goal
	goal.Spec.Source = "other-tenant"
	if _, e := s.Append(ctx, v.ID, v1.AppendIntentConversation{ID: "bad-scope", ExpectedVersion: v.Version, Text: "Change scope", Goal: &goal}); e == nil {
		t.Fatal("scope changed")
	}
}
func TestLeaseRecoveryFencesLateResults(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, _ := fixture(t, pg)
			v, _ := s.Create(ctx, input())
			v, e := s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "lease-run", ExpectedVersion: v.Version, Mode: "guided"})
			if e != nil {
				t.Fatal(e)
			}
			old, e := s.Repository.Claim(ctx, "old")
			if e != nil {
				t.Fatal(e)
			}
			_, e = s.Repository.Update(ctx, v.ID, func(r *record, now time.Time) error { r.LeaseUntil = now.Add(-time.Second); return nil })
			if e != nil {
				t.Fatal(e)
			}
			_, e = s.Repository.Claim(ctx, "new")
			if e != nil {
				t.Fatal(e)
			}
			_, e = s.Repository.Update(ctx, v.ID, func(r *record, now time.Time) error { return fenced(*r, old.Token, now) })
			if !errors.Is(e, errLease) {
				t.Fatal("late result was accepted", e)
			}
		})
	}
}
func TestCancellationInterruptsModelAndAccessIsRechecked(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	v, _ := s.Create(ctx, input())
	started := make(chan struct{})
	stopped := make(chan struct{})
	s.Model = &scriptedModel{route: "blocking", turn: func(ctx context.Context, _ rm.Request) (rm.Response, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return rm.Response{}, ctx.Err()
	}}
	v, e := s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "cancel-run", ExpectedVersion: v.Version, Mode: "model", ModelConsent: true})
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := s.WorkOne(ctx); done <- e }()
	<-started
	v, _ = s.Read(ctx, v.ID)
	if _, e = s.Cancel(ctx, v.ID, v.Version); e != nil {
		t.Fatal(e)
	}
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("model did not stop")
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	v, _ = s.Read(ctx, v.ID)
	if v.State != "cancelled" || len(v.Runs[0].Steps) != 0 {
		t.Fatal(v)
	}
	s.Authorize = func(context.Context, v1.InvestigationActor) error { return ErrForbidden }
	v, e = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "revoked", ExpectedVersion: v.Version, Mode: "guided"})
	if e != nil {
		t.Fatal(e)
	}
	work(t, s, ctx, 1)
	v, _ = s.Read(ctx, v.ID)
	if v.Runs[1].ErrorCode != "access_denied" {
		t.Fatal(v)
	}
}

func TestChangedBindingAndNewAnswerInvalidateSelection(t *testing.T) {
	for _, mode := range []string{"binding", "answer"} {
		t.Run(mode, func(t *testing.T) {
			s, ctx, _ := fixture(t, false)
			v, _ := s.Create(ctx, input())
			v, e := s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "discover", ExpectedVersion: v.Version, Mode: "guided"})
			if e != nil {
				t.Fatal(e)
			}
			work(t, s, ctx, 1)
			v, _ = s.Read(ctx, v.ID)
			v, e = s.Select(ctx, v.ID, v1.ConfirmIntentBinding{ExpectedVersion: v.Version, IntentDigest: v.Plan.IntentDigest, Binding: v1.IntentSelection{Name: "requests", ExpectedRevision: 1}, Confirm: true})
			if e != nil {
				t.Fatal(e)
			}
			if mode == "answer" {
				goal := *input().Goal
				goal.Spec.Target = .9995
				v, e = s.Append(ctx, v.ID, v1.AppendIntentConversation{ID: "revision", ExpectedVersion: v.Version, Text: "Use 99.95% over 1 hour", Goal: &goal})
				if e != nil || v.Selection != nil || v.Plan != nil {
					t.Fatal("answer retained old selection", v, e)
				}
				return
			}
			b := testtelemetry.Binding()
			b.Spec.SuccessLabels = map[string]string{"status": "500"}
			if _, e = s.Platform.ApplyDefinition(ctx, model.Definition{Binding: &b}, 1); e != nil {
				t.Fatal(e)
			}
			v, e = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "compile-stale", ExpectedVersion: v.Version, Mode: "guided"})
			if e != nil {
				t.Fatal(e)
			}
			work(t, s, ctx, 1)
			v, _ = s.Read(ctx, v.ID)
			if v.State != "failed" || v.Runs[1].ErrorCode != "revision_conflict" {
				t.Fatal(v)
			}
		})
	}
}

type draftingFixture struct{}

func (draftingFixture) ModelID() string { return "synthetic-goal-extractor" }
func (draftingFixture) Draft(_ context.Context, text string) (v1.ModelGoal, error) {
	target, window := "99.9%", "1 hour"
	return v1.ModelGoal{Outcome: "availability", TargetText: &target, WindowText: &window, Clarifications: []string{}}, nil
}
func TestGroundedDraftAndModelContextExcludeIdentity(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	s.Platform.IntentDrafter = draftingFixture{}
	in := input()
	in.Goal = nil
	v, _ := s.Create(ctx, in)
	v, e := s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "draft", ExpectedVersion: v.Version, Mode: "guided", ModelConsent: true})
	if e != nil {
		t.Fatal(e)
	}
	work(t, s, ctx, 2)
	v, _ = s.Read(ctx, v.ID)
	if v.State != "needs_selection" || v.Goal == nil || v.Goal.Spec.Target != .999 || v.Goal.Spec.WindowSeconds != 3600 || v.Runs[0].ModelCalls != 1 {
		t.Fatal(v)
	}
	s.Model = &scriptedModel{route: "private", turn: func(_ context.Context, r rm.Request) (rm.Response, error) {
		if strings.Contains(r.Messages[0].Text, `"actor"`) || strings.Contains(r.Messages[0].Text, `"subject"`) {
			t.Fatal("identity sent to model")
		}
		return response("ask_user", map[string]string{"question": "Review the goal semantics."}), nil
	}}
	v, e = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "private-context", ExpectedVersion: v.Version, Mode: "model", ModelConsent: true})
	if e != nil {
		t.Fatal(e)
	}
	work(t, s, ctx, 2)
}

type routedDrafter struct {
	draftingFixture
	route string
}

func (d *routedDrafter) Descriptor() rm.Descriptor {
	return rm.Descriptor{Provider: "fixture", Model: "extractor", Protocol: "fixture", RouteDigest: d.route, Local: true}
}
func TestDraftModelRouteIsPinnedIndependently(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	in := input()
	in.Goal = nil
	d := &routedDrafter{route: "original"}
	s.Platform.IntentDrafter = d
	v, _ := s.Create(ctx, in)
	v, e := s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "draft-route", ExpectedVersion: v.Version, Mode: "guided", ModelConsent: true})
	if e != nil {
		t.Fatal(e)
	}
	d.route = "different-endpoint"
	work(t, s, ctx, 1)
	v, _ = s.Read(ctx, v.ID)
	if v.State != "failed" || v.Runs[0].ErrorCode != "draft_model_configuration_changed" || v.Runs[0].ModelCalls != 0 {
		t.Fatal(v)
	}
}

func TestEmbeddedToolSchemasAreValidAndStrict(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	v, _ := s.Create(ctx, input())
	r, _ := s.Repository.Read(ctx, v.ID)
	for _, selection := range []bool{false, true} {
		if selection {
			r.Session.Selection = &v1.IntentBindingConfirmation{Binding: v1.IntentSelection{Name: "requests", ExpectedRevision: 1}}
		}
		for _, tool := range toolset(r) {
			if !json.Valid(tool.Schema) {
				t.Fatal("invalid schema", tool.Name)
			}
			var schema map[string]any
			if err := json.Unmarshal(tool.Schema, &schema); err != nil {
				t.Fatal(err)
			}
			if schema["additionalProperties"] != false {
				t.Fatal("open tool schema", tool.Name)
			}
		}
	}
	if e := decodeArgs(json.RawMessage(`{"source":"other","tenant":"beta"}`), &struct{}{}); e == nil {
		t.Fatal("scope-changing tool arguments accepted")
	}
}
