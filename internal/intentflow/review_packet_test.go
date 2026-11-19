package intentflow

import (
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestIntentReviewPacketFreezesGoalConfirmationAndGaps(t *testing.T) {
	s, ctx, v := readyInlineEvidenceFixture(t, false)
	v, err := s.RecordPreparationEvidence(ctx, v.ID, evidenceRequest(v))
	if err != nil {
		t.Fatal(err)
	}
	originalGoal := clone(*v.Goal)
	originalConfirmation := clone(*v.Selection)
	// A review packet must use saved preparation only, even if the provider is
	// currently unavailable. It must not claim deployment or current health.
	s.Platform.Collector.(*testtelemetry.Source).Fail = true
	v, err = s.PrepareChangeSet(ctx, v.ID, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	packet := v.ChangeSet.Recommendation.ReviewPacket
	if packet == nil || hash(packet.Goal) != hash(originalGoal) || hash(packet.Confirmation) != hash(originalConfirmation) || packet.LegacyMeasurement != nil {
		t.Fatal("lost exact inline authoring context")
	}
	if packet.Destination.Tenant != "alpha" || packet.Destination.Project != originalGoal.Metadata.Project || packet.Destination.Source != originalGoal.Spec.Source || packet.Destination.Service != originalGoal.Spec.Service || packet.Destination.SourceDigest != v.ChangeSet.SourceDigest || packet.Destination.State != "not_verified" {
		t.Fatal("invented publication or routing evidence")
	}
	if len(packet.Resources) != len(v.ChangeSet.Changes) {
		t.Fatal("resource summary incomplete")
	}
	for i, resource := range packet.Resources {
		change := v.ChangeSet.Changes[i]
		if resource.Kind != change.Kind || resource.Name != change.Name || resource.Action != change.Action || resource.ExpectedRevision != change.ExpectedRevision || resource.Digest != change.Digest {
			t.Fatal("resource summary changed exact proposal")
		}
	}
	states := map[string]string{}
	for _, check := range packet.Checks {
		states[check.Code] = check.State
	}
	if states["measurement_confirmation"] != "recorded" || states["preparation_evidence"] != "recorded" || states["historical_evidence"] != "not_recorded" || states["current_provider_evidence"] != "not_checked" || states["exclusions"] != "not_recorded" || states["candidate_provenance"] != "not_recorded" {
		t.Fatal("invented or omitted evidence", states)
	}
	artifact, err := s.ReviewArtifact(ctx, v.ID)
	if err != nil || hash(artifact.ChangeSet.Recommendation.ReviewPacket) != hash(packet) {
		t.Fatal("export changed frozen packet", err)
	}
	changed := clone(*v.ChangeSet)
	changed.Recommendation.ReviewPacket.Goal.Spec.Target = .9
	if changeDigest(v.Tenant, v.ID, changed) == v.ChangeSet.Digest {
		t.Fatal("target summary not covered by approval digest")
	}
	changed = clone(*v.ChangeSet)
	changed.Recommendation.ReviewPacket.Destination.Source = "other"
	if changeDigest(v.Tenant, v.ID, changed) == v.ChangeSet.Digest {
		t.Fatal("destination summary not covered by approval digest")
	}
	// Mutating a returned copy cannot change the saved packet.
	packet.Goal.Spec.Measurement.Labels["service"] = "other"
	saved, err := s.ReviewArtifact(ctx, v.ID)
	if err != nil || saved.ChangeSet.Recommendation.ReviewPacket.Goal.Spec.Measurement.Labels["service"] != "api" {
		t.Fatal("returned packet aliases storage", err)
	}
}

func TestIntentReviewPacketLegacyMeasurementAndCUJResources(t *testing.T) {
	s, ctx, v := readyChangeFixture(t, false)
	v, err := s.PrepareChangeSet(ctx, v.ID, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	packet := v.ChangeSet.Recommendation.ReviewPacket
	if packet == nil || packet.Goal.Spec.Measurement != nil || packet.LegacyMeasurement == nil || hash(*packet.LegacyMeasurement) != hash(v.Plan.Compiled.Definition) || packet.Confirmation.Binding.Name != "requests" || len(packet.Resources) != 2 {
		t.Fatal("legacy or CUJ context missing")
	}
	kinds := map[string]bool{}
	for _, resource := range packet.Resources {
		kinds[resource.Kind] = true
	}
	if !kinds[v1.SLOKind] || !kinds[v1.JourneyKind] {
		t.Fatal("SLO/CUJ diff references missing", kinds)
	}
}

func TestIntentReviewPacketExclusionsRequireExactHumanConfirmation(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) { runTestIntentReviewPacketExclusionsRequireExactHumanConfirmation(t, pg) })
	}
}
func runTestIntentReviewPacketExclusionsRequireExactHumanConfirmation(t *testing.T, pg bool) {
	for _, definition := range []v1.IntentExclusions{{None: true}, {ObservationBoundaries: []string{"Requests not reaching the server are unobserved."}}} {
		s, ctx, v := readyInlineEvidenceFixture(t, pg)
		goal := clone(*v.Goal)
		goal.Spec.Exclusions = &definition
		v, err := s.Append(ctx, v.ID, v1.AppendIntentConversation{ID: "boundaries", ExpectedVersion: v.Version, Text: "Review these observation boundaries.", Goal: &goal})
		if err != nil {
			t.Fatal(err)
		}
		v, err = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "exclusion-discovery", ExpectedVersion: v.Version, Mode: "guided"})
		if err != nil {
			t.Fatal(err)
		}
		work(t, s, ctx, 1)
		v, _ = s.Read(ctx, v.ID)
		if _, err = s.PrepareChangeSet(ctx, v.ID, v.Version); err == nil {
			t.Fatal("unconfirmed exclusions reached frozen proposal")
		}
		v, err = s.Select(ctx, v.ID, v1.ConfirmIntentBinding{ExpectedVersion: v.Version, IntentDigest: v.Plan.IntentDigest, MeasurementDigest: v.Plan.Compiled.MeasurementDigest, Confirm: true})
		if err != nil {
			t.Fatal(err)
		}
		v, err = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "exclusion-compile", ExpectedVersion: v.Version, Mode: "guided"})
		if err != nil {
			t.Fatal(err)
		}
		work(t, s, ctx, 2)
		v, _ = s.Read(ctx, v.ID)
		v, err = s.PrepareChangeSet(ctx, v.ID, v.Version)
		if err != nil {
			t.Fatal(err)
		}
		exclusions := v.ChangeSet.Recommendation.ReviewPacket.Exclusions
		if exclusions == nil || exclusions.State != "recorded" || exclusions.IntentDigest != hash(*v.Goal) || hash(exclusions.Definition) != hash(definition) || exclusions.Confirmation.Actor.Subject == "" {
			t.Fatal("observation exclusions lost confirmation", exclusions)
		}
		changed := clone(*v.ChangeSet)
		changed.Recommendation.ReviewPacket.Exclusions.Definition.None = !definition.None
		if changeDigest(v.Tenant, v.ID, changed) == v.ChangeSet.Digest {
			t.Fatal("exclusions not covered by approval digest")
		}
	}
}
