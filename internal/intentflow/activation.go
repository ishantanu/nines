package intentflow

import (
	"context"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// InspectActivation reads current runtime evidence without publishing, collecting,
// updating the conversation or changing the separately governed outcome.
func (s *Service) InspectActivation(ctx context.Context, id string) (v1.IntentActivation, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	v, err := s.Read(ctx, id)
	if err != nil {
		return v1.IntentActivation{}, err
	}
	if v.ChangeSet == nil || v.ChangeSet.State != "applied" {
		return v1.IntentActivation{}, model.Invalid("apply a reviewed change set before activation inspection")
	}
	verification, err := s.VerifyChangeSet(ctx, id)
	if err != nil {
		return v1.IntentActivation{}, err
	}
	out := v1.IntentActivation{APIVersion: v1.APIVersion, Tenant: v.Tenant, Conversation: id, Version: v.Version, ChangeSetDigest: verification.Digest, Operation: verification.Operation, OperationDigest: verification.Digest, SourceDigest: v.ChangeSet.SourceDigest, CheckedAt: s.Platform.Now().UTC(), Resources: verification.Resources, Limitations: []string{
		"This is an as-of inspection, not a persisted post-change outcome or authorization. Repeat this operation to refresh it.",
		"Collected SLI, external rule evaluation and provider-native SLO results are independent. Activation does not mean the target is met.",
		"No rules are published, no samples are collected, and no definitions are changed. Notification delivery and causality are not verified.",
	}}
	if verification.Operation == "rollback" {
		out.OperationDigest = verification.RollbackDigest
	}
	if verification.SourceState != "matches" {
		setActivation(&out, "drifted", "source_changed", "The configured source differs from the approved source.", "Review the current source and prepare a new approved change before relying on runtime evidence.")
		return out, nil
	}
	for _, resource := range verification.Resources {
		if resource.State != "matches" {
			setActivation(&out, "drifted", "definition_changed", "An applied resource is missing or differs from its exact applied revision.", "Inspect the resource revisions and prepare a separately reviewed correction.")
			return out, nil
		}
	}
	var slo *v1.IntentResourceVerification
	for i := range out.Resources {
		if out.Resources[i].Kind == v1.SLOKind {
			if slo != nil {
				return v1.IntentActivation{}, model.Invalid("activation inspection requires exactly one applied SLO")
			}
			slo = &out.Resources[i]
		}
	}
	if slo == nil {
		return v1.IntentActivation{}, model.Invalid("no applied SLO is available for activation inspection")
	}
	view, err := s.Platform.ReadSLO(ctx, slo.Name)
	if err != nil {
		return v1.IntentActivation{}, err
	}
	if model.ConfigRevision(view.SLO) != slo.Revision || hash(model.DefinitionFromSLO(view.SLO)) != slo.Digest {
		return v1.IntentActivation{}, ErrConflict
	}
	integration, err := s.Platform.IntegrationStatus(ctx, slo.Name)
	if err != nil {
		return v1.IntentActivation{}, err
	}
	if integration.Revision != slo.Revision || integration.SLOID != slo.Name || integration.Tenant != v.Tenant {
		return v1.IntentActivation{}, ErrConflict
	}
	// Fence all resources and the conversation after external I/O; do not return
	// provider evidence for an operation that changed while it was inspected.
	after, err := s.Read(ctx, id)
	if err != nil {
		return v1.IntentActivation{}, err
	}
	if after.Version != v.Version {
		return v1.IntentActivation{}, ErrConflict
	}
	check, err := s.VerifyChangeSet(ctx, id)
	if err != nil {
		return v1.IntentActivation{}, err
	}
	if check.Digest != verification.Digest || check.Operation != verification.Operation || check.RollbackDigest != verification.RollbackDigest || check.SourceState != "matches" || !sameActivationResources(check.Resources, verification.Resources) {
		return v1.IntentActivation{}, ErrConflict
	}
	out.Integration = &integration
	out.Measurement = view.Quality
	out.CheckedAt = s.Platform.Now().UTC()
	classifyActivation(&out, view.SLO.StepSeconds)
	return out, nil
}

func setActivation(out *v1.IntentActivation, state, code, message, action string) {
	out.State, out.Code, out.Message, out.NextAction = state, code, message, action
	out.RetryAt = nil
	if state == "pending" {
		at := out.CheckedAt.Add(30 * time.Second)
		out.RetryAt = &at
	}
}
func classifyActivation(out *v1.IntentActivation, step int64) {
	i := out.Integration
	if i == nil {
		setActivation(out, "pending", "inspection_unavailable", "No runtime inspection is available.", "Repeat activation inspection.")
		return
	}
	stage := func(code string, v v1.IntegrationStage) {
		state := "pending"
		if v.State == "unsupported" {
			state = "unsupported"
		}
		setActivation(out, state, code, v.Message, v.NextAction)
	}
	if i.Deployment.State != "written" && i.Deployment.State != "stored" && i.Deployment.State != "configured" {
		stage("publication_unverified", i.Deployment)
		return
	}
	fresh := func(at *time.Time) bool {
		return at != nil && !at.After(out.CheckedAt) && out.CheckedAt.Sub(*at) <= max(90*time.Second, 2*time.Duration(step)*time.Second)
	}
	if i.Evaluation.State != "evaluating" {
		stage("evaluation_unverified", i.Evaluation)
		return
	}
	if !fresh(i.Evaluation.ObservedAt) {
		setActivation(out, "pending", "evaluation_unverified", "Evaluation has no verified recent timestamp.", "Check evaluator timestamps and availability, then inspect activation again.")
		return
	}
	if i.RuleOutput == nil {
		setActivation(out, "pending", "output_unverified", "Query-dataset output has not been verified.", "Check current rule output through the configured query source.")
		return
	}
	if i.RuleOutput.State == "unsupported" && i.RuleOutput.Code == "native_slo_no_recordings" {
		// Native publication has no recording-rule output. Its independently
		// observed evaluation signal remains required above.
		out.Limitations = append(out.Limitations, "Native SLO publication installs no Nines recording rules. The provider evaluation signal is separate from Nines collected SLI.")
	} else if i.RuleOutput.State != "verified" {
		stage("output_unverified", *i.RuleOutput)
		return
	}
	if i.RuleOutput.State == "verified" && !fresh(i.RuleOutput.ObservedAt) {
		setActivation(out, "pending", "output_unverified", "Recorded output has no verified recent timestamp.", "Check the current output series and query dataset, then inspect activation again.")
		return
	}
	if i.Collection.State != "collected" {
		stage("collection_unverified", i.Collection)
		return
	}
	if !fresh(i.Collection.ObservedAt) {
		setActivation(out, "pending", "collection_unverified", "Collection has no verified recent completion timestamp.", "Check collection scheduling and provider timestamps.")
		return
	}
	q := out.Measurement
	if q == nil || q.State != "usable" || q.Total <= 0 || !fresh(&q.EvaluatedAt) || q.SourceDigest != out.SourceDigest {
		setActivation(out, "pending", "measurement_unusable", "Fresh, usable SLI evidence is not available for the approved source.", "Inspect measurement quality and traffic coverage; collect current intervals through the normal collector.")
		return
	}
	setActivation(out, "verified", "activation_verified", "Current publication, evaluation and collected SLI evidence are verified for the applied revision.", "Inspect SLI attainment and the separately recorded observation policy before judging the change outcome.")
}

func sameActivationResources(a, b []v1.IntentResourceVerification) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].IntentAppliedResource != b[i].IntentAppliedResource || a[i].State != b[i].State || a[i].CurrentRevision != b[i].CurrentRevision || a[i].CurrentDigest != b[i].CurrentDigest {
			return false
		}
	}
	return true
}
