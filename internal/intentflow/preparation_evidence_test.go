package intentflow

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func readyInlineEvidenceFixture(t *testing.T, pg bool) (*Service, context.Context, v1.IntentConversation) {
	t.Helper()
	s, ctx, _ := fixture(t, pg)
	in := input()
	in.Scope.Journey = nil
	in.Goal.Spec.Journey = nil
	in.Goal.Spec.StepSeconds = 60
	in.Goal.Spec.Measurement = templateMeasurement()
	v, err := s.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "discovery", ExpectedVersion: v.Version, Mode: "guided"})
	if err != nil {
		t.Fatal(err)
	}
	work(t, s, ctx, 1)
	v, err = s.Read(ctx, v.ID)
	if err != nil || v.Plan == nil || v.Plan.Compiled == nil {
		t.Fatalf("missing compilation: %v %#v", err, v.Plan)
	}
	v, err = s.Select(ctx, v.ID, v1.ConfirmIntentBinding{ExpectedVersion: v.Version, IntentDigest: v.Plan.IntentDigest, MeasurementDigest: v.Plan.Compiled.MeasurementDigest, Confirm: true})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "compile", ExpectedVersion: v.Version, Mode: "guided"})
	if err != nil {
		t.Fatal(err)
	}
	work(t, s, ctx, 2)
	v, err = s.Read(ctx, v.ID)
	if err != nil || v.State != "ready" {
		t.Fatalf("not ready: %s %v", v.State, err)
	}
	return s, ctx, v
}
func evidenceRequest(v v1.IntentConversation) v1.RecordIntentEvidence {
	return v1.RecordIntentEvidence{ExpectedVersion: v.Version, IntentDigest: hash(*v.Goal), ReadTelemetry: true}
}
func TestPreparationEvidenceRecordedAndFrozen(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, v := readyInlineEvidenceFixture(t, pg)
			before := clone(v)
			agent := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "alpha", Role: "agent", Subject: "agent", Kind: "api_key"})
			v, err := s.RecordPreparationEvidence(agent, v.ID, evidenceRequest(v))
			if err != nil {
				t.Fatal(err)
			}
			if !preparationEvidenceMatches(v) || v.PreparationEvidence.CapturedVersion != before.Version || v.PreparationEvidence.RequestedBy.Subject != "agent" || hash(v.Selection) != hash(before.Selection) || hash(v.Goal) != hash(before.Goal) || v.ChangeSet != nil {
				t.Fatal("receipt changed reviewed semantics or lacked provenance")
			}
			if checkState(t, v, "preparation_evidence") != "recorded" || checkState(t, v, "current_provider_evidence") != "not_checked" {
				t.Fatal("snapshot presented as current health")
			}
			if _, err = s.RecordPreparationEvidence(ctx, v.ID, evidenceRequest(before)); !errors.Is(err, ErrConflict) {
				t.Fatalf("stale version accepted: %v", err)
			}
			v, err = s.PrepareChangeSet(ctx, v.ID, v.Version)
			if err != nil {
				t.Fatal(err)
			}
			if v.ChangeSet.Recommendation.PreparationEvidence == nil || hash(v.ChangeSet.Recommendation.PreparationEvidence) != hash(v.PreparationEvidence) {
				t.Fatal("evidence absent from frozen recommendation")
			}
			artifact, err := s.ReviewArtifact(ctx, v.ID)
			if err != nil || artifact.ChangeSet.Recommendation.PreparationEvidence == nil || artifact.AuditIntegrity != "verified" {
				t.Fatalf("receipt absent from audited review artifact: %v", err)
			}
			original := changeDigest(v.Tenant, v.ID, *v.ChangeSet)
			changed := clone(*v.ChangeSet)
			changed.Recommendation.PreparationEvidence.Report.State = "forged"
			if changeDigest(v.Tenant, v.ID, changed) == original {
				t.Fatal("approval digest does not bind receipt")
			}
			if _, err = s.RecordPreparationEvidence(ctx, v.ID, evidenceRequest(v)); !errors.Is(err, ErrConflict) {
				t.Fatalf("frozen receipt refreshed: %v", err)
			}
			edited, err := s.Append(ctx, v.ID, v1.AppendIntentConversation{ID: "clarification", ExpectedVersion: v.Version, Text: "Clarify exclusions before the next review", Goal: clone(v.Goal)})
			if err != nil || edited.PreparationEvidence != nil || len(edited.ChangeSetHistory) != 1 || edited.ChangeSetHistory[0].Recommendation.PreparationEvidence == nil {
				t.Fatalf("edit did not clear current receipt and retain frozen history: %v", err)
			}

		})
	}
}
func TestPreparationEvidenceRejectsUnreviewedRequests(t *testing.T) {
	s, ctx, v := readyInlineEvidenceFixture(t, false)
	for _, name := range []string{"no-consent", "wrong-digest", "stale-version", "viewer", "wrong-tenant", "revoked"} {
		t.Run(name, func(t *testing.T) {
			in := evidenceRequest(v)
			callctx := ctx
			switch name {
			case "no-consent":
				in.ReadTelemetry = false
			case "wrong-digest":
				in.IntentDigest = strings.Repeat("a", 64)
			case "stale-version":
				in.ExpectedVersion--
			case "viewer":
				callctx = tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "alpha", Role: "viewer", Subject: "viewer"})
			case "wrong-tenant":
				callctx = tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "beta", Role: "owner", Subject: "owner"})
			case "revoked":
				s.Authorize = func(context.Context, v1.InvestigationActor) error { return ErrForbidden }
				defer func() { s.Authorize = func(context.Context, v1.InvestigationActor) error { return nil } }()
			}
			if _, err := s.RecordPreparationEvidence(callctx, v.ID, in); err == nil {
				t.Fatal("invalid evidence request accepted")
			}
			saved, err := s.Read(ctx, v.ID)
			if err != nil || saved.PreparationEvidence != nil {
				t.Fatal("failed request saved a receipt")
			}
		})
	}
	if preparationEvidenceMatches(v1.IntentConversation{PreparationEvidence: &v1.IntentPreparationEvidence{CapturedVersion: 1}}) {
		t.Fatal("nil goal accepted")
	}
}
func TestPreparationEvidenceHTTPRejectsAmbiguousInput(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	mux := http.NewServeMux()
	Mount(mux, s)
	for _, body := range []string{`null`, `{"readTelemetry":true,"readTelemetry":false}`, `{"report":{}}`, `{"ReadTelemetry":true}`, `{"history":{"from":"x","from":"y"}}`, `{} {}`, strings.Repeat(" ", 4097)} {
		req := httptest.NewRequest(http.MethodPost, "/api/intent-conversations/conversation-one/evidence", strings.NewReader(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != 400 {
			t.Fatalf("ambiguous request status %d", w.Code)
		}
	}
}

type evidenceReadHook struct {
	*testtelemetry.Source
	calls int
	after func()
}

func (c *evidenceReadHook) CollectWindowRange(ctx context.Context, slo model.SLO, from, to time.Time, resolution int64) ([]model.WindowObservation, error) {
	c.calls++
	result, err := c.Source.CollectWindowRange(ctx, slo, from, to, resolution)
	if c.after != nil {
		f := c.after
		c.after = nil
		f()
	}
	return result, err
}
func TestPreparationEvidenceConcurrentChangesAndNoImplicitHistory(t *testing.T) {
	for _, kind := range []string{"none", "conversation", "source", "authorization"} {
		t.Run(kind, func(t *testing.T) {
			s, ctx, v := readyInlineEvidenceFixture(t, false)
			hook := &evidenceReadHook{Source: s.Platform.Collector.(*testtelemetry.Source)}
			s.Platform.Collector = hook
			// Changing collector type alters the fallback route fingerprint; recompile
			// against the new fixture route before collecting.
			_, err := s.Repository.Update(ctx, v.ID, func(r *record, _ time.Time) error {
				r.Session.Plan.SourceDigest = s.Platform.SourceFingerprint("primary")
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			hook.after = func() {
				switch kind {
				case "conversation":
					_, err := s.Repository.Update(ctx, v.ID, func(r *record, now time.Time) error { touch(r, now); return nil })
					if err != nil {
						t.Error(err)
					}
				case "source":
					hook.Route = "changed"
				case "authorization":
					s.Authorize = func(context.Context, v1.InvestigationActor) error { return ErrForbidden }
				}
			}
			saved, err := s.RecordPreparationEvidence(ctx, v.ID, evidenceRequest(v))
			if kind == "none" {
				if err != nil || saved.PreparationEvidence == nil || hook.calls != 1 {
					t.Fatalf("unexpected read count %d or missing receipt: %v", hook.calls, err)
				}
				tampered := clone(saved)
				tampered.PreparationEvidence.Report.State = "forged"
				if preparationEvidenceMatches(tampered) {
					t.Fatal("tampered report accepted")
				}
				hook.Route = "changed-after-record"
				if _, err = s.PrepareChangeSet(ctx, saved.ID, saved.Version); !errors.Is(err, ErrConflict) {
					t.Fatalf("source drift accepted during freeze: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("concurrent change accepted")
				}
				current, err := s.Read(ctx, v.ID)
				if err != nil || current.PreparationEvidence != nil {
					t.Fatal("rejected evidence persisted")
				}
			}
		})
	}
}
