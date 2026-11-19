package intentflow

import (
	"encoding/json"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func candidateReceiptFixture(v v1.IntentConversation) v1.IntentFlowRun {
	catalog := hash("candidate-catalog")
	report := v1.IntentCandidateDiscovery{APIVersion: v1.APIVersion, Tenant: v.Tenant, ObservedAt: v.UpdatedAt, Scope: v1.IntentCandidateInput{Source: v.Scope.Source, Service: v.Scope.Service, Project: v.Scope.Metadata.Project, Population: clone(v.Scope.Population), Outcome: v.Goal.Spec.Outcome}, SourceDigest: hash(struct{ Tenant, Source, Route string }{v.Tenant, v.Scope.Source, v.Plan.SourceDigest}), CatalogDigest: catalog, State: "unsupported", HumanConfirmationRequired: true, Candidates: []v1.IntentMeasurementCandidate{}, Names: []string{}, Questions: []string{}, Warnings: []string{}}
	raw, _ := json.Marshal(report)
	return v1.IntentFlowRun{ID: "candidate-receipt", InputDigest: hash(v.Scope), Mode: "model", ModelConsent: true, TemplateCatalogDigest: catalog, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, Steps: []v1.IntentFlowStep{{Tool: "discover_measurement_candidates", At: v.UpdatedAt, IntentDigest: hash(*v.Goal), Digest: bytesDigest(raw), Result: raw}}}
}
func mutateCandidateReport(run *v1.IntentFlowRun, change func(*v1.IntentCandidateDiscovery)) {
	var report v1.IntentCandidateDiscovery
	_ = json.Unmarshal(run.Steps[0].Result, &report)
	change(&report)
	raw, _ := json.Marshal(report)
	run.Steps[0].Result = raw
	run.Steps[0].Digest = bytesDigest(raw)
}
func TestIntentReviewPacketCandidateReceiptStates(t *testing.T) {
	_, _, v := readyInlineEvidenceFixture(t, false)
	base := candidateReceiptFixture(v)
	for _, tc := range []struct {
		name, state string
		edit        func(*v1.IntentFlowRun)
		report      bool
	}{
		{"matching", "recorded", func(*v1.IntentFlowRun) {}, true},
		{"older-goal", "prior_goal", func(r *v1.IntentFlowRun) { r.Steps[0].IntentDigest = hash("older-goal") }, true},
		{"unstamped-goal", "unverified", func(r *v1.IntentFlowRun) { r.Steps[0].IntentDigest = "" }, true},
		{"tampered", "invalid", func(r *v1.IntentFlowRun) { r.Steps[0].Result = []byte(`{}`) }, false},
		{"cross-tenant", "invalid", func(r *v1.IntentFlowRun) {
			mutateCandidateReport(r, func(p *v1.IntentCandidateDiscovery) { p.Tenant = "other" })
		}, false},
		{"source-route", "stale", func(r *v1.IntentFlowRun) {
			mutateCandidateReport(r, func(p *v1.IntentCandidateDiscovery) { p.SourceDigest = hash("other-route") })
		}, false},
		{"population", "stale", func(r *v1.IntentFlowRun) {
			mutateCandidateReport(r, func(p *v1.IntentCandidateDiscovery) { p.Scope.Population = map[string]string{"service": "other"} })
		}, false},
		{"catalog", "invalid", func(r *v1.IntentFlowRun) { r.TemplateCatalogDigest = hash("other-catalog") }, false},
		{"timestamp", "invalid", func(r *v1.IntentFlowRun) { r.Steps[0].At = r.CreatedAt.Add(-1) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := clone(v)
			run := clone(base)
			tc.edit(&run)
			snapshot.Runs = append(snapshot.Runs, run)
			receipt := candidateReviewReceipt(snapshot)
			if receipt == nil || receipt.State != tc.state || (receipt.Report != nil) != tc.report {
				t.Fatalf("%s: %#v", tc.name, receipt)
			}
			if receipt.Report != nil {
				receipt.Report.Scope.Population["service"] = "mutated"
				again := candidateReviewReceipt(snapshot)
				if again.Report.Scope.Population["service"] == "mutated" {
					t.Fatal("report aliases saved evidence")
				}
			}
		})
	}
	snapshot := clone(v)
	invalid := clone(base)
	invalid.ID = "newer-invalid"
	invalid.Steps[0].Digest = "bad"
	snapshot.Runs = append(snapshot.Runs, base, invalid)
	if out := candidateReviewReceipt(snapshot); out == nil || out.State != "invalid" || out.RunID != "newer-invalid" {
		t.Fatal("silently fell back to older discovery", out)
	}
}
func TestIntentReviewPacketCandidateReceiptFrozenInApprovalDigest(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) { runTestIntentReviewPacketCandidateReceiptFrozenInApprovalDigest(t, pg) })
	}
}
func runTestIntentReviewPacketCandidateReceiptFrozenInApprovalDigest(t *testing.T, pg bool) {
	s, ctx, v := readyInlineEvidenceFixture(t, pg)
	_, err := s.Repository.Update(ctx, v.ID, func(r *record, _ time.Time) error {
		r.Session.Runs = append(r.Session.Runs, candidateReceiptFixture(r.Session))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.PrepareChangeSet(ctx, v.ID, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	packet := v.ChangeSet.Recommendation.ReviewPacket
	if packet.CandidateDiscovery == nil || packet.CandidateDiscovery.State != "recorded" || packet.CandidateDiscovery.Report.State != "unsupported" {
		t.Fatal("lost receipt or upgraded adapter capability", packet.CandidateDiscovery)
	}
	changed := clone(*v.ChangeSet)
	changed.Recommendation.ReviewPacket.CandidateDiscovery.Report.State = "discovered"
	if changeDigest(v.Tenant, v.ID, changed) == v.ChangeSet.Digest {
		t.Fatal("candidate receipt not covered by approval digest")
	}
	artifact, err := s.ReviewArtifact(ctx, v.ID)
	if err != nil || hash(artifact.ChangeSet.Recommendation.ReviewPacket.CandidateDiscovery) != hash(packet.CandidateDiscovery) {
		t.Fatal("read-only export lost candidate provenance", err)
	}
}
