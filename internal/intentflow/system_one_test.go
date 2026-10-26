package intentflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	rm "github.com/ishantanu/nines/internal/reviewmodel"
	"github.com/ishantanu/nines/internal/systemone"
	"github.com/ishantanu/nines/internal/tenancy"
)

type fakeSystemOne struct {
	calls int
	run   func(context.Context, systemone.Input) (v1.SystemOneAssessment, error)
	route string
}

func (f *fakeSystemOne) Descriptor() v1.SystemOneRoute {
	return v1.SystemOneRoute{Provider: "typesafe", Model: f.route, ContractDigest: "fixture-contract"}
}
func (f *fakeSystemOne) Assess(ctx context.Context, in systemone.Input) (v1.SystemOneAssessment, error) {
	f.calls++
	if f.run != nil {
		return f.run(ctx, in)
	}
	return v1.SystemOneAssessment{Status: "assessed", InputDigest: model.DefinitionDigest(in), ResolvedModel: f.route, Judgments: &v1.SystemOneJudgments{Category: v1.SystemOneChoice{Choice: "request-serving", Confidence: .8, Probabilities: map[string]float64{"request-serving": .8, "none": .2}}, PopulationDefined: .9, GoodEventsDefined: .8, ObservationPointDefined: .1}}, nil
}
func TestSystemOneConsentAuditAndDurability(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, dsn := fixture(t, pg)
			assessor := &fakeSystemOne{route: "jev-fixture"}
			s.SystemOne = assessor
			in := input()
			in.Scope.Journey = nil
			in.Goal.Spec.Journey = nil
			v, err := s.Create(ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			before := hash(v.Goal)
			v, err = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "consented", Mode: "guided", ExpectedVersion: v.Version, SystemOneConsent: true})
			if err != nil {
				t.Fatal(err)
			}
			if assessor.calls != 0 {
				t.Fatal("start sent data")
			}
			work(t, s, ctx, 1)
			v, err = s.Read(ctx, v.ID)
			if err != nil || assessor.calls != 1 || v.Runs[0].SystemOne == nil || v.Runs[0].SystemOneCalls != 1 || len(v.Runs[0].Steps) != 0 || hash(v.Goal) != before || v.Selection != nil {
				t.Fatal(v, err)
			}
			if len(v.Audit) != 2 || v.Audit[0].Action != "systemone.requested" || v.Audit[1].Action != "systemone.assessed" || !auditChainValid(v.Audit) {
				t.Fatal(v.Audit)
			}
			page, err := s.ListAudit(ctx, v1.AuditQuery{Action: "systemone.assessed", Limit: 10})
			if err != nil || len(page.Items) != 1 {
				t.Fatal("assessment missing from tenant journal", page, err)
			}
			saved, err := s.AuditRecord(ctx, page.Items[0].ID)
			if err != nil || saved.SystemOne == nil || hash(saved.SystemOne.Assessment) != hash(v.Runs[0].SystemOne) {
				t.Fatal("audit receipt unavailable", saved, err)
			}
			tampered := clone(saved)
			tampered.SystemOne.Assessment.Status = "tampered"
			if validateJournalRecord(tampered) == nil {
				t.Fatal("altered assessment passed integrity check")
			}
			if pg {
				s.Repository.Close()
				s.Repository, err = NewPostgres(ctx, dsn)
				if err != nil {
					t.Fatal(err)
				}
				defer s.Repository.Close()
			}
			work(t, s, ctx, 1)
			v, err = s.Read(ctx, v.ID)
			if err != nil || assessor.calls != 1 || v.State != "needs_selection" || v.Plan == nil || v.Selection != nil || hash(v.Goal) != before {
				t.Fatal(v, err)
			}
			if _, err = s.Platform.Store.Read(ctx, in.Scope.Metadata.Name); !errors.Is(err, model.ErrNotFound) {
				t.Fatal("assessment created SLO", err)
			}
		})
	}
}
func TestSystemOneDisabledConsentAndProviderIsolation(t *testing.T) {
	for _, consent := range []bool{false, true} {
		t.Run(map[bool]string{false: "not-consented", true: "disabled"}[consent], func(t *testing.T) {
			s, ctx, _ := fixture(t, false)
			v, _ := s.Create(ctx, input())
			assessor := &fakeSystemOne{route: "jev-fixture"}
			if !consent {
				s.SystemOne = assessor
			}
			v, err := s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "discovery", Mode: "guided", ExpectedVersion: v.Version, SystemOneConsent: consent})
			if consent {
				if err == nil {
					t.Fatal("disabled consent accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			work(t, s, ctx, 1)
			v, _ = s.Read(ctx, v.ID)
			if assessor.calls != 0 || v.Runs[0].SystemOne != nil {
				t.Fatal("unconsented call")
			}
		})
	}
}
func TestSystemOneUnavailableAndCrashDoNotRetry(t *testing.T) {
	for _, kind := range []string{"provider-error", "reserved", "route-changed"} {
		t.Run(kind, func(t *testing.T) {
			s, ctx, _ := fixture(t, false)
			f := &fakeSystemOne{route: "original", run: func(context.Context, systemone.Input) (v1.SystemOneAssessment, error) {
				return v1.SystemOneAssessment{}, errors.New("private provider diagnostic")
			}}
			s.SystemOne = f
			v, _ := s.Create(ctx, input())
			v, err := s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "assess", ExpectedVersion: v.Version, Mode: "guided", SystemOneConsent: true})
			if err != nil {
				t.Fatal(err)
			}
			if kind == "reserved" {
				_, err = s.Repository.Update(ctx, v.ID, func(r *record, _ time.Time) error { current(r).SystemOneCalls = 1; return nil })
				if err != nil {
					t.Fatal(err)
				}
			}
			if kind == "route-changed" {
				f.route = "changed"
			}
			work(t, s, ctx, 2)
			v, _ = s.Read(ctx, v.ID)
			raw, _ := json.Marshal(v)
			if v.Runs[0].SystemOne == nil || v.Runs[0].SystemOne.Status != "unavailable" || f.calls > 1 || v.State != "needs_selection" || strings.Contains(string(raw), "private provider") {
				t.Fatal(v, f.calls)
			}
			if kind != "provider-error" && f.calls != 0 {
				t.Fatal("replayed or rerouted call")
			}
		})
	}
}
func TestSystemOneAdviceReachesModelWithoutChangingTools(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	s.SystemOne = &fakeSystemOne{route: "jev"}
	s.Model = &scriptedModel{route: "fixture", turn: func(_ context.Context, req rm.Request) (rm.Response, error) {
		if !strings.Contains(req.System, "Optional System One semantic advice") || !strings.Contains(req.System, "not authorization") {
			t.Error("missing advisory context")
		}
		if len(req.Tools) != 4 {
			t.Error("System One changed permitted tools", req.Tools)
		}
		return response("ask_user", map[string]string{"question": "Please confirm where you measure completions."}), nil
	}}
	v, _ := s.Create(ctx, input())
	v, err := s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "model", ExpectedVersion: v.Version, Mode: "model", ModelConsent: true, SystemOneConsent: true})
	if err != nil {
		t.Fatal(err)
	}
	work(t, s, ctx, 3)
	v, _ = s.Read(ctx, v.ID)
	if v.State != "needs_input" || v.Selection != nil || v.Runs[0].ModelCalls != 1 {
		t.Fatal(v)
	}
}
func TestSystemOneCancellationAndTenantFences(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	f := &fakeSystemOne{route: "jev"}
	s.SystemOne = f
	v, _ := s.Create(ctx, input())
	foreign := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "other", Role: "owner", Subject: "owner"})
	if _, err := s.Start(foreign, v.ID, v1.CreateIntentFlowRun{ID: "foreign", ExpectedVersion: v.Version, Mode: "guided", SystemOneConsent: true}); err == nil {
		t.Fatal("cross tenant start")
	}
	v, err := s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "cancel", ExpectedVersion: v.Version, Mode: "guided", SystemOneConsent: true})
	if err != nil {
		t.Fatal(err)
	}
	f.run = func(_ context.Context, _ systemone.Input) (v1.SystemOneAssessment, error) {
		latest, err := s.Read(ctx, v.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.Cancel(ctx, v.ID, latest.Version); err != nil {
			t.Fatal(err)
		}
		return v1.SystemOneAssessment{Status: "assessed"}, nil
	}
	work(t, s, ctx, 1)
	v, _ = s.Read(ctx, v.ID)
	if v.State != "cancelled" || v.Runs[0].SystemOne != nil {
		t.Fatal("published after cancellation", v)
	}
}

func TestSystemOneAdviceCannotConfirmOrApplyAndIsFrozenForReview(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	s.SystemOne = &fakeSystemOne{route: "jev-fixture"}
	v, err := s.Create(ctx, input())
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "discover", ExpectedVersion: v.Version, Mode: "guided", SystemOneConsent: true})
	if err != nil {
		t.Fatal(err)
	}
	work(t, s, ctx, 2)
	v, _ = s.Read(ctx, v.ID)
	confirm := v1.ConfirmIntentBinding{ExpectedVersion: v.Version, IntentDigest: v.Plan.IntentDigest, Binding: v1.IntentSelection{Name: "requests", ExpectedRevision: 1}, Confirm: true}
	agent := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "alpha", Role: "agent", Subject: "agent", Kind: "api_token"})
	if _, err = s.Select(agent, v.ID, confirm); !errors.Is(err, ErrForbidden) {
		t.Fatal("System One bypassed human confirmation", err)
	}
	v, err = s.Select(ctx, v.ID, confirm)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "compile", ExpectedVersion: v.Version, Mode: "guided", SystemOneConsent: true})
	if err != nil {
		t.Fatal(err)
	}
	work(t, s, ctx, 3)
	v, err = s.Read(ctx, v.ID)
	if err != nil || v.State != "ready" {
		t.Fatal(v, err)
	}
	v, err = s.PrepareChangeSet(ctx, v.ID, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	if hash(v.ChangeSet.Recommendation.SystemOne) != hash(v.Runs[1].SystemOne) || hash(v.ChangeSet.Recommendation.SystemOneRoute) != hash(v.Runs[1].SystemOneRoute) {
		t.Fatal("proposal lost semantic provenance")
	}
	if _, err = s.Platform.Store.Read(ctx, input().Scope.Metadata.Name); !errors.Is(err, model.ErrNotFound) {
		t.Fatal("assessment preparation wrote SLO", err)
	}
	modified := clone(v)
	modified.ChangeSet.Recommendation.SystemOne.Judgments.PopulationDefined = .123
	if frozenReviewsValid(modified) {
		t.Fatal("modified assessment retained approval digest")
	}
}

func TestSystemOneLocalPolicyFailuresAreExplainedWithoutProviderDiagnostics(t *testing.T) {
	for _, cause := range []error{systemone.ErrPaused, systemone.ErrBudgetExhausted, systemone.ErrPolicyUnavailable} {
		t.Run(cause.Error(), func(t *testing.T) {
			s, ctx, _ := fixture(t, false)
			s.SystemOne = &fakeSystemOne{route: "jev", run: func(context.Context, systemone.Input) (v1.SystemOneAssessment, error) {
				return v1.SystemOneAssessment{}, cause
			}}
			v, err := s.Create(ctx, input())
			if err != nil {
				t.Fatal(err)
			}
			v, err = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "policy", ExpectedVersion: v.Version, Mode: "guided", SystemOneConsent: true})
			if err != nil {
				t.Fatal(err)
			}
			work(t, s, ctx, 2)
			v, err = s.Read(ctx, v.ID)
			if err != nil || v.Runs[0].SystemOne == nil || v.Runs[0].SystemOne.ErrorCode != cause.Error() || v.State != "needs_selection" {
				t.Fatal(v, err)
			}
		})
	}
}
