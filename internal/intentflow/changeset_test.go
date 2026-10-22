package intentflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func readyChangeFixture(t *testing.T, pg bool) (*Service, context.Context, v1.IntentConversation) {
	t.Helper()
	s, ctx, dsn := fixture(t, pg)
	if pg {
		persistent, e := store.NewPostgres(ctx, dsn)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(persistent.Close)
		slos, e := s.Platform.Store.List(ctx)
		if e != nil {
			t.Fatal(e)
		}
		for _, v := range slos {
			if e = persistent.Create(ctx, v); e != nil {
				t.Fatal(e)
			}
		}
		journeys, e := s.Platform.Journeys.Journeys(ctx)
		if e != nil {
			t.Fatal(e)
		}
		for _, v := range journeys {
			if _, e = persistent.ApplyJourney(ctx, v, 0); e != nil {
				t.Fatal(e)
			}
		}
		topology, e := s.Platform.Store.(store.TopologyStore).ListTopology(ctx, "", "", 101)
		if e != nil {
			t.Fatal(e)
		}
		for _, v := range topology {
			if _, e = persistent.ApplyTopology(ctx, v, 0); e != nil {
				t.Fatal(e)
			}
		}
		s.Platform.Store = persistent
		s.Platform.Journeys = persistent
	}
	v, e := s.Create(ctx, input())
	if e != nil {
		t.Fatal(e)
	}
	v, e = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "discovery", ExpectedVersion: v.Version, Mode: "guided"})
	if e != nil {
		t.Fatal(e)
	}
	work(t, s, ctx, 1)
	v, _ = s.Read(ctx, v.ID)
	v, e = s.Select(ctx, v.ID, v1.ConfirmIntentBinding{ExpectedVersion: v.Version, IntentDigest: v.Plan.IntentDigest, Binding: v1.IntentSelection{Name: "requests", ExpectedRevision: 1}, Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	v, e = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "compile", ExpectedVersion: v.Version, Mode: "guided"})
	if e != nil {
		t.Fatal(e)
	}
	work(t, s, ctx, 2)
	v, _ = s.Read(ctx, v.ID)
	if v.State != "ready" {
		t.Fatal(v.State)
	}
	return s, ctx, v
}
func TestReviewedChangesAtomicApplyRetryAndDrift(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, v := readyChangeFixture(t, pg)
			v, e := s.PrepareChangeSet(ctx, v.ID, v.Version)
			if e != nil {
				t.Fatal(e)
			}
			if v.ChangeSet == nil || len(v.ChangeSet.Changes) != 2 {
				t.Fatal(v.ChangeSet)
			}
			exported, e := s.ExportKubernetes(ctx, v.ID, "team-commerce", v.ChangeSet.Digest)
			if e != nil {
				t.Fatal(e)
			}
			if exported.Tenant != "alpha" || !strings.Contains(exported.YAML, "nines.dev/project") || strings.Contains(exported.YAML, "token") {
				t.Fatal(exported)
			}
			var list struct {
				Items []struct {
					APIVersion string `yaml:"apiVersion"`
					Kind       string `yaml:"kind"`
					Metadata   struct {
						Namespace string `yaml:"namespace"`
					} `yaml:"metadata"`
				} `yaml:"items"`
			}
			if yaml.Unmarshal([]byte(exported.YAML), &list) != nil || len(list.Items) != 2 {
				t.Fatal("invalid Kubernetes export")
			}
			for _, item := range list.Items {
				if item.APIVersion != v1.APIVersion || item.Metadata.Namespace != "team-commerce" {
					t.Fatal(item)
				}
			}
			if _, e = s.ExportKubernetes(ctx, v.ID, "team-commerce", strings.Repeat("0", 64)); !errors.Is(e, ErrConflict) {
				t.Fatal("stale export accepted", e)
			}

			if _, e = s.Platform.Store.Read(ctx, "intent-created"); !errors.Is(e, model.ErrNotFound) {
				t.Fatal("planning wrote SLO", e)
			}
			j, _ := s.Platform.Journeys.JourneyRevision(ctx, "checkout", 0)
			if j.Revision != 1 {
				t.Fatal("planning changed CUJ")
			}
			approve := v1.ReviewIntentChangeSet{ExpectedVersion: v.Version, Digest: v.ChangeSet.Digest, Confirm: true}
			agent := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "alpha", Role: "agent", Subject: "agent", Kind: "api_token"})
			if _, e = s.ApproveChangeSet(agent, v.ID, approve); !errors.Is(e, ErrForbidden) {
				t.Fatal("agent approved", e)
			}
			if _, e = s.ApplyChangeSet(ctx, v.ID, approve); !errors.Is(e, ErrConflict) {
				t.Fatal("unapproved apply", e)
			}
			v, e = s.ApproveChangeSet(ctx, v.ID, approve)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = s.Platform.Store.Read(ctx, "intent-created"); !errors.Is(e, model.ErrNotFound) {
				t.Fatal("approval wrote SLO", e)
			}
			apply := v1.ReviewIntentChangeSet{ExpectedVersion: v.Version, Digest: v.ChangeSet.Digest, Confirm: true}
			if _, e = s.ApplyChangeSet(agent, v.ID, apply); !errors.Is(e, ErrForbidden) {
				t.Fatal("agent applied", e)
			}
			v, e = s.ApplyChangeSet(ctx, v.ID, apply)
			if e != nil {
				t.Fatal(e)
			}
			if v.ChangeSet.State != "applied" || len(v.ChangeSet.Applied) != 2 {
				t.Fatal(v.ChangeSet)
			}
			verified, e := s.VerifyChangeSet(ctx, v.ID)
			if e != nil {
				t.Fatal(e)
			}
			for _, r := range verified.Resources {
				if r.State != "matches" {
					t.Fatal(r)
				}
			}
			if verified.Resources[0].Health != "unknown" {
				t.Fatal("new SLO claimed healthy")
			}
			again, e := s.ApplyChangeSet(ctx, v.ID, apply)
			if e != nil || again.Version != v.Version {
				t.Fatal("retry not idempotent", e)
			}
			// Replaying the durable store receipt must not invoke the mutation again.
			body, e := s.Platform.Store.(store.DefinitionBatchStore).DefinitionBatch(ctx, v.ID+"/"+v.ChangeSet.ID, v.ChangeSet.Digest, true, func(store.Store) (json.RawMessage, error) { t.Error("receipt re-executed"); return nil, nil })
			if e != nil || !json.Valid(body) {
				t.Fatal(e)
			}
			old, _ := s.Platform.Store.Read(ctx, "intent-created")
			d := model.DefinitionFromSLO(old.SLO)
			d.Spec.Target = .998
			d.Spec.Intent.Target = .998
			if _, e = s.Platform.ApplyDefinition(ctx, model.Definition{SLO: &d}, 1); e != nil {
				t.Fatal(e)
			}
			verified, e = s.VerifyChangeSet(ctx, v.ID)
			if e != nil || verified.Resources[0].State != "drifted" {
				t.Fatal(verified, e)
			}
			if _, e = s.ApplyChangeSet(ctx, v.ID, apply); e != nil {
				t.Fatal(e)
			}
			now, _ := s.Platform.Store.Read(ctx, "intent-created")
			if now.SLO.Target != .998 {
				t.Fatal("retry overwrote external edit")
			}
		})
	}
}
func TestChangesRollbackAndApprovalInvalidation(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, v := readyChangeFixture(t, pg)
			v, e := s.PrepareChangeSet(ctx, v.ID, v.Version)
			if e != nil {
				t.Fatal(e)
			}
			v, e = s.ApproveChangeSet(ctx, v.ID, v1.ReviewIntentChangeSet{ExpectedVersion: v.Version, Digest: v.ChangeSet.Digest, Confirm: true})
			if e != nil {
				t.Fatal(e)
			}
			j, _ := s.Platform.Journeys.JourneyRevision(ctx, "checkout", 0)
			j.Manifest.Spec.Description = "Externally changed"
			j.Digest = hash(j.Manifest)
			if _, e = s.Platform.Journeys.ApplyJourney(ctx, j, 1); e != nil {
				t.Fatal(e)
			}
			if _, e = s.ApplyChangeSet(ctx, v.ID, v1.ReviewIntentChangeSet{ExpectedVersion: v.Version, Digest: v.ChangeSet.Digest, Confirm: true}); !errors.Is(e, model.ErrRevisionConflict) {
				t.Fatal("stale CUJ applied", e)
			}
			if _, e = s.Platform.Store.Read(ctx, "intent-created"); !errors.Is(e, model.ErrNotFound) {
				t.Fatal("failed CUJ left partial SLO", e)
			}
			old := v.ChangeSet.Digest
			v, e = s.Append(ctx, v.ID, v1.AppendIntentConversation{ID: "clarification", ExpectedVersion: v.Version, Text: "Use a stricter target"})
			if e != nil {
				t.Fatal(e)
			}
			if v.ChangeSet != nil || len(v.ChangeSetHistory) != 1 || v.ChangeSetHistory[0].Digest != old {
				t.Fatal("approval history lost", v)
			}
			if _, e = s.ApplyChangeSet(ctx, v.ID, v1.ReviewIntentChangeSet{ExpectedVersion: v.Version, Digest: old, Confirm: true}); !errors.Is(e, ErrConflict) {
				t.Fatal("invalidated approval applied", e)
			}
		})
	}
}

type failConversationCommit struct {
	Repository
	fail bool
}

func (r *failConversationCommit) Update(ctx context.Context, id string, fn func(*record, time.Time) error) (record, error) {
	return r.Repository.Update(ctx, id, func(v *record, now time.Time) error {
		if e := fn(v, now); e != nil {
			return e
		}
		if r.fail {
			r.fail = false
			return errors.New("simulated conversation commit failure")
		}
		return nil
	})
}
func TestApplyRecoversAfterConversationCommitFailure(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, v := readyChangeFixture(t, pg)
			v, e := s.PrepareChangeSet(ctx, v.ID, v.Version)
			if e != nil {
				t.Fatal(e)
			}
			v, e = s.ApproveChangeSet(ctx, v.ID, v1.ReviewIntentChangeSet{ExpectedVersion: v.Version, Digest: v.ChangeSet.Digest, Confirm: true})
			if e != nil {
				t.Fatal(e)
			}
			in := v1.ReviewIntentChangeSet{ExpectedVersion: v.Version, Digest: v.ChangeSet.Digest, Confirm: true}
			original := s.Repository
			s.Repository = &failConversationCommit{Repository: original, fail: true}
			if _, e = s.ApplyChangeSet(ctx, v.ID, in); e == nil {
				t.Fatal("failure not simulated")
			}
			if _, e = s.Platform.Store.Read(ctx, "intent-created"); e != nil {
				t.Fatal("resource transaction failed", e)
			}
			v, e = s.Read(ctx, v.ID)
			if e != nil || v.ChangeSet.State != "approved" {
				t.Fatal(v.ChangeSet, e)
			}
			s.Repository = original
			s.Platform.Collector.(*testtelemetry.Source).Route = "changed-after-commit"
			resume := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "alpha", Role: "editor", Subject: "recovering-reviewer", Kind: "user"})
			v, e = s.ApplyChangeSet(resume, v.ID, in)
			if e != nil || v.ChangeSet.State != "applied" {
				t.Fatal(v.ChangeSet, e)
			}
			if v.ChangeSet.AppliedBy == nil || v.ChangeSet.AppliedBy.Subject != "owner" {
				t.Fatal("original applying identity was lost", v.ChangeSet.AppliedBy)
			}
			history, e := s.Platform.Journeys.JourneyHistory(ctx, "checkout")
			if e != nil || len(history) != 2 {
				t.Fatal("retry duplicated CUJ mutation", len(history), e)
			}
		})
	}
}

func TestConfiguredSourceChangeInvalidatesPendingReview(t *testing.T) {
	s, ctx, v := readyChangeFixture(t, false)
	v, e := s.PrepareChangeSet(ctx, v.ID, v.Version)
	if e != nil {
		t.Fatal(e)
	}
	v, e = s.ApproveChangeSet(ctx, v.ID, v1.ReviewIntentChangeSet{ExpectedVersion: v.Version, Digest: v.ChangeSet.Digest, Confirm: true})
	if e != nil {
		t.Fatal(e)
	}
	s.Platform.Collector.(*testtelemetry.Source).Route = "another-dataset"
	if _, e = s.ApplyChangeSet(ctx, v.ID, v1.ReviewIntentChangeSet{ExpectedVersion: v.Version, Digest: v.ChangeSet.Digest, Confirm: true}); !errors.Is(e, ErrConflict) && !errors.Is(e, model.ErrRevisionConflict) {
		t.Fatal("changed source accepted", e)
	}
	if _, e = s.Platform.Store.Read(ctx, "intent-created"); !errors.Is(e, model.ErrNotFound) {
		t.Fatal("source change wrote SLO", e)
	}
}
