package intentflow

import v1 "github.com/ishantanu/nines/api/v1alpha1"

// reviewPacket reads the already prepared snapshot only. Keeping it inside the
// recommendation binds the complete summary to the existing approval digest.
func reviewPacket(r *record, c v1.IntentChangeSet) *v1.IntentReviewPacket {
	v := r.Session
	packet := &v1.IntentReviewPacket{
		Goal: clone(*v.Goal), Confirmation: clone(*v.Selection), Checks: preparationChecks(v),
		Destination: v1.IntentReviewDestination{Tenant: v.Tenant, Project: v.Scope.Metadata.Project, Service: v.Scope.Service, Source: v.Scope.Source, SourceDigest: c.SourceDigest, State: "not_verified"},
		Resources:   []v1.IntentReviewResource{},
		Limitations: []string{
			"User-authored descriptions and observation points are review context, not independently verified business semantics.",
			"Observation exclusions describe reviewed boundaries only; they never change population selectors, discard samples or establish observability outside the measurement. Missing exclusions are not an assertion of none.",
			"Candidate discovery is saved context, not confirmation that a candidate became this measurement. A recorded receipt does not prove fresh traffic, measurement quality or current catalog/provider configuration.",
			"The destination identifies a logical Nines telemetry source. Provider endpoints, upstream tenant routing, publication permissions, loaded rules and observed SLI require separate activation verification.",
			"Checks describe saved evidence at proposal preparation. Recorded evidence is an as-of snapshot; gaps do not authorize automatic target or policy changes.",
		},
	}
	if v.Goal.Spec.Measurement == nil {
		packet.LegacyMeasurement = clone(&v.Plan.Compiled.Definition)
	}
	exclusionState := "not_recorded"
	if v.Goal.Spec.Exclusions != nil {
		exclusionState = "unconfirmed"
		if savedCompilationMatches(v, hash(*v.Goal)) && v.Selection != nil && v.Selection.IntentDigest == hash(*v.Goal) && selectionMatchesCompilation(v) {
			exclusionState = "recorded"
		}
		packet.Exclusions = &v1.IntentReviewExclusions{State: exclusionState, Definition: clone(*v.Goal.Spec.Exclusions), IntentDigest: hash(*v.Goal), Confirmation: clone(*v.Selection)}
	}
	packet.Checks = append(packet.Checks, v1.IntentPreparationCheck{Code: "exclusions", State: exclusionState, Message: "Observation exclusions are review context, not executable filters. Only exact measurement confirmation records them; omitted exclusions do not mean none."})
	packet.CandidateDiscovery = candidateReviewReceipt(v)
	state, message := "not_recorded", "No saved candidate discovery receipt exists. Candidate discovery is optional."
	if packet.CandidateDiscovery != nil {
		state = packet.CandidateDiscovery.State
		message = "Newest saved candidate discovery receipt; inspect state and limitations. This does not select or confirm a candidate, revalidate a provider, or grant approval."
	}
	packet.Checks = append(packet.Checks, v1.IntentPreparationCheck{Code: "candidate_provenance", State: state, Message: message})
	for _, change := range c.Changes {
		packet.Resources = append(packet.Resources, v1.IntentReviewResource{Kind: change.Kind, Name: change.Name, Action: change.Action, ExpectedRevision: change.ExpectedRevision, Digest: change.Digest})
	}
	return packet
}
