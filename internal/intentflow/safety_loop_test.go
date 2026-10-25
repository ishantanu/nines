package intentflow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestApplyRefreshesSafetyAndJournalsBlockedEvidence(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, v := updatedChangeFixture(t, pg)
			v, err := s.PrepareChangeSet(ctx, v.ID, v.Version)
			if err != nil {
				t.Fatal(err)
			}
			v, err = s.ApproveChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Reviewed baseline and proposed target"))
			if err != nil {
				t.Fatal(err)
			}
			later := s.Platform.Now().Add(5 * time.Minute)
			s.Platform.Now = func() time.Time { return later }
			_, err = s.ApplyChangeSet(independentApplier(ctx), v.ID, reviewInput(v, v.ChangeSet.Digest, ""))
			if err == nil {
				t.Fatal("stale baseline was allowed")
			}
			actual, _ := s.Platform.Store.Read(ctx, "intent-created")
			if actual.SLO.Revision != 1 {
				t.Fatal("blocked check mutated SLO")
			}
			page, err := s.ListAudit(ctx, v1.AuditQuery{Conversation: v.ID, Action: "change_set.applied", Outcome: "blocked"})
			if err != nil || len(page.Items) != 1 || page.Items[0].SafetyCheck == nil || page.Items[0].SafetyCheck.Quality.Freshness.State != "stale" {
				t.Fatal(page, err)
			}
			item, err := s.AuditRecord(ctx, page.Items[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			item.SafetyCheck.State = "allowed"
			if validateJournalRecord(item) == nil {
				t.Fatal("tampered evidence accepted")
			}
			v, _ = s.Read(ctx, v.ID)
			v, err = s.RejectChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Withdraw for a separately reviewed corrective exception"))
			if err != nil {
				t.Fatal(err)
			}
			v, err = s.PrepareChangeSet(ctx, v.ID, v.Version)
			if err != nil {
				t.Fatal(err)
			}
			in := reviewInput(v, v.ChangeSet.Digest, "Reviewed instrumentation recovery and accepted uncertainty")
			in.SafetyExceptionReason = "Correct measurement definition while telemetry collection is being repaired"
			editor := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "alpha", Subject: "editor", Role: "editor", Kind: "user"})
			if _, err = s.ApproveChangeSet(editor, v.ID, in); !errors.Is(err, ErrForbidden) {
				t.Fatal("editor granted exception", err)
			}
			v, err = s.ApproveChangeSet(ctx, v.ID, in)
			if err != nil {
				t.Fatal(err)
			}
			owner, _ := tenancy.FromContext(independentApplier(ctx))
			owner.Role = "owner"
			v, err = s.ApplyChangeSet(tenancy.WithPrincipal(ctx, owner), v.ID, reviewInput(v, v.ChangeSet.Digest, ""))
			if err != nil || v.ChangeSet.SafetyCheck.State != "exception" {
				t.Fatal(v.ChangeSet, err)
			}
			original := v.ChangeSet.SafetyCheck.CheckedAt
			later = later.Add(time.Hour)
			again, err := s.ApplyChangeSet(independentApplier(ctx), v.ID, reviewInput(v, v.ChangeSet.Digest, ""))
			if err != nil || !again.ChangeSet.SafetyCheck.CheckedAt.Equal(original) {
				t.Fatal("receipt retry replaced safety evidence", err)
			}
		})
	}
}
func TestApplyRejectsRevokedReviewer(t *testing.T) {
	s, ctx, v := readyChangeFixture(t, false)
	v, err := s.PrepareChangeSet(ctx, v.ID, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.ApproveChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Reviewed exact creation"))
	if err != nil {
		t.Fatal(err)
	}
	s.AuthorizeReview = func(context.Context, v1.InvestigationActor, bool) error { return ErrForbidden }
	if _, err = s.ApplyChangeSet(independentApplier(ctx), v.ID, reviewInput(v, v.ChangeSet.Digest, "")); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
}

// This destination double proves the observer only inspects; mutation attempts fail.
type outcomeDestination struct {
	t       *testing.T
	now     *time.Time
	failure bool
	hook    func()
}

func (d *outcomeDestination) Plan(context.Context, model.SLO, rules.Target) (rules.Plan, error) {
	d.t.Fatal("observer planned a mutation")
	return rules.Plan{}, nil
}
func (d *outcomeDestination) Apply(context.Context, model.SLO, rules.Plan) (rules.Deployment, error) {
	d.t.Fatal("observer mutated destination")
	return rules.Deployment{}, nil
}
func (d *outcomeDestination) Inspect(_ context.Context, _ model.SLO, _ rules.Target, _ time.Time) (v1.IntegrationStage, v1.IntegrationStage) {
	if d.hook != nil {
		d.hook()
	}
	state := "evaluating"
	if d.failure {
		state = "failed"
	}
	return v1.IntegrationStage{State: "configured", Message: "Current definition configured"}, v1.IntegrationStage{State: state, ObservedAt: d.now, Message: "Runtime fixture"}
}
func (d *outcomeDestination) Identity(_ context.Context, s model.SLO, _ rules.Target) (model.RuleOutputIdentity, error) {
	return model.RuleOutputIdentity{Tenant: "alpha", Source: s.Source, SLO: s.ID, Revision: s.Revision, DefinitionDigest: hash(model.DefinitionFromSLO(s)), StepSeconds: s.StepSeconds}, nil
}

type outcomeTelemetry struct {
	*collectingTelemetry
	now     *time.Time
	good    float64
	missing bool
}

func (c *outcomeTelemetry) CollectWindows(_ context.Context, s model.SLO, from, to time.Time) ([]model.WindowObservation, error) {
	out := []model.WindowObservation{}
	for at := from.Add(time.Duration(s.StepSeconds) * time.Second); !at.After(to); at = at.Add(time.Duration(s.StepSeconds) * time.Second) {
		for _, w := range s.Windows() {
			out = append(out, model.WindowObservation{Time: at, Revision: s.Revision, WindowSeconds: w, Good: c.good, Total: 1000, Coverage: 1})
		}
	}
	return out, nil
}
func (c *outcomeTelemetry) RuleOutput(context.Context, model.RuleOutputIdentity, time.Time) v1.IntegrationStage {
	state := "verified"
	if c.missing {
		state = "not_observed"
	}
	return v1.IntegrationStage{State: state, ObservedAt: c.now, Message: "Output fixture"}
}
func TestOutcomeRequiresSustainedEvidenceAndJournalsResults(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, v := appliedUpdate(t, pg)
			tick := v.ChangeSet.AppliedAt.Add(4 * time.Minute)
			s.Platform.Now = func() time.Time { return tick }
			telemetry := &outcomeTelemetry{collectingTelemetry: s.Platform.Collector.(*collectingTelemetry), now: &tick, good: 999}
			s.Platform.Collector = telemetry
			destination := &outcomeDestination{t: t, now: &tick}
			s.Platform.RuleManager = &rules.Manager{Tenant: "alpha", External: destination, Targets: map[string]rules.Target{"primary": {Kind: "operator", Connection: "primary"}}}
			for i := 0; i < 6; i++ {
				collectFreshBaseline(t, s, ctx, "intent-created")
				var err error
				v, err = s.ObserveChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, ""))
				if err != nil {
					t.Fatal(err)
				}
				want := "pending"
				if i == 5 {
					want = "verified"
				}
				if v.ChangeSet.Outcome.State != want {
					t.Fatalf("check %d: %+v", i, v.ChangeSet.Outcome)
				}
				tick = tick.Add(time.Minute)
			}
			page, err := s.ListAudit(ctx, v1.AuditQuery{Conversation: v.ID, Action: "change_set.observed"})
			if err != nil || len(page.Items) != 6 {
				t.Fatal(page, err)
			}
			item, err := s.AuditRecord(ctx, page.Items[0].ID)
			if err != nil || item.Outcome == nil {
				t.Fatal(item, err)
			}
			item.Outcome.State = "regressed"
			if validateJournalRecord(item) == nil {
				t.Fatal("outcome evidence tampering accepted")
			}
			verified, err := s.VerifyChangeSet(ctx, v.ID)
			if err != nil || verified.Outcome.State != "verified" {
				t.Fatal(verified, err)
			}
			if pg {
				db := s.Repository.(*postgres)
				repo, err := NewPostgres(ctx, db.pool.Config().ConnString())
				if err != nil {
					t.Fatal(err)
				}
				defer repo.Close()
				saved, err := repo.Read(ctx, v.ID)
				if err != nil || saved.Session.ChangeSet.Outcome.State != "verified" {
					t.Fatal(saved, err)
				}
			}
		})
	}
}
func TestOutcomeDriftMissingOutputDeadlineAndRegression(t *testing.T) {
	for _, scenario := range []string{"missing_output", "regression", "source_drift", "definition_drift", "runtime_failure", "observation_gap", "deadline", "active_burn"} {
		t.Run(scenario, func(t *testing.T) {
			s, ctx, v := appliedUpdate(t, false)
			tick := v.ChangeSet.AppliedAt.Add(4 * time.Minute)
			s.Platform.Now = func() time.Time { return tick }
			telemetry := &outcomeTelemetry{collectingTelemetry: s.Platform.Collector.(*collectingTelemetry), now: &tick, good: 999}
			s.Platform.Collector = telemetry
			dest := &outcomeDestination{t: t, now: &tick}
			s.Platform.RuleManager = &rules.Manager{Tenant: "alpha", External: dest, Targets: map[string]rules.Target{"primary": {Kind: "operator", Connection: "primary"}}}
			if scenario == "regression" {
				telemetry.good = 900
			}
			if scenario == "missing_output" {
				telemetry.missing = true
			}
			if scenario == "runtime_failure" {
				dest.failure = true
			}

			if scenario == "definition_drift" {
				dest.hook = func() {
					_ = s.Platform.Store.Update(ctx, "intent-created", func(r *model.Record) error { r.SLO.Revision++; return nil })
				}
			}
			collectFreshBaseline(t, s, ctx, "intent-created")
			if scenario == "active_burn" {
				if err := s.Platform.Store.Update(ctx, "intent-created", func(r *model.Record) error { r.Alert.Phase = "firing"; return nil }); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "source_drift" {
				telemetry.Route = "changed"
			}
			v, err := s.ObserveChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, ""))
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "active_burn":
				if v.ChangeSet.Outcome.State != "review_required" || v.ChangeSet.Outcome.Code != "active_burn_alert" {
					t.Fatal(v.ChangeSet.Outcome)
				}
			case "regression":
				if v.ChangeSet.Outcome.State != "regressed" {
					t.Fatal(v.ChangeSet.Outcome)
				}
			case "source_drift", "definition_drift":
				if v.ChangeSet.Outcome.State != "drifted" {
					t.Fatal(v.ChangeSet.Outcome)
				}
			default:
				if v.ChangeSet.Outcome.State != "pending" {
					t.Fatal(v.ChangeSet.Outcome)
				}
				if scenario == "observation_gap" {
					first := *v.ChangeSet.Outcome.HealthySince
					tick = tick.Add(10 * time.Minute)
					collectFreshBaseline(t, s, ctx, "intent-created")
					v, err = s.ObserveChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, ""))
					if err != nil || v.ChangeSet.Outcome.State != "pending" || !v.ChangeSet.Outcome.HealthySince.After(first) {
						t.Fatal(v.ChangeSet.Outcome, err)
					}
				} else {
					tick = v.ChangeSet.Outcome.DeadlineAt.Add(time.Minute)
					collectFreshBaseline(t, s, ctx, "intent-created")
					v, err = s.ObserveChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, ""))
					if err != nil || v.ChangeSet.Outcome.State != "insufficient_evidence" {
						t.Fatal(v.ChangeSet.Outcome, err)
					}
				}
			}
			raw, _ := json.Marshal(v.ChangeSet.Outcome)
			if len(raw) > 32<<10 {
				t.Fatal("report unexpectedly unbounded")
			}
		})
	}
}

func TestApplyRejectsForgedApprovalMetadata(t *testing.T) {
	s, ctx, v := updatedChangeFixture(t, false)
	v, err := s.PrepareChangeSet(ctx, v.ID, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.ApproveChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Reviewed exact proposal"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Repository.Update(ctx, v.ID, func(r *record, now time.Time) error {
		c := r.Session.ChangeSet
		at := s.Platform.Now()
		c.SafetyException = &v1.IntentSafetyException{Reason: "Forged exception", ApprovedBy: *c.ApprovedBy, ApprovedAt: at, ExpiresAt: at.Add(30 * time.Minute)}
		c.ApprovalDigest = approvalDigest(*c)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	v, _ = s.Read(ctx, v.ID)
	applier, _ := tenancy.FromContext(independentApplier(ctx))
	applier.Role = "owner"
	if _, err = s.ApplyChangeSet(tenancy.WithPrincipal(ctx, applier), v.ID, reviewInput(v, v.ChangeSet.Digest, "")); !errors.Is(err, ErrConflict) {
		t.Fatal("forged approval allowed", err)
	}
}
