package intentflow

import (
	"context"
	"encoding/json"
	"strconv"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func discoveredIntentCandidates(r record) bool {
	if len(r.Session.Runs) == 0 {
		return false
	}
	for _, step := range current(&r).Steps {
		if step.Tool == "discover_measurement_candidates" {
			return true
		}
	}
	return false
}
func (s *Service) discoverIntentCandidates(ctx context.Context, r *record, raw json.RawMessage) (v1.IntentCandidateDiscovery, error) {
	if r.Session.Goal == nil || r.Session.Goal.Spec.Measurement != nil || r.Session.Selection != nil || current(r).Mode != "model" || r.RunRequest == nil || !r.RunRequest.ModelConsent || discoveredIntentCandidates(*r) {
		return v1.IntentCandidateDiscovery{}, model.Invalid("candidate discovery requires a consented, unconfirmed model run and is allowed once per run")
	}
	var args struct {
		Prefix   string `json:"prefix"`
		Category string `json:"category"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return v1.IntentCandidateDiscovery{}, err
	}
	goal := r.Session.Goal
	in := v1.IntentCandidateInput{Source: r.Session.Scope.Source, Service: r.Session.Scope.Service, Project: r.Session.Scope.Metadata.Project, Population: r.Session.Scope.Population, Outcome: goal.Spec.Outcome, Prefix: args.Prefix, Category: args.Category, Limit: 2}
	if goal.Spec.ThresholdSeconds != nil {
		in.Threshold = strconv.FormatFloat(*goal.Spec.ThresholdSeconds, 'g', -1, 64) + "s"
	}
	out, err := s.Platform.DiscoverIntentCandidates(ctx, in, s.Templates)
	if err != nil {
		return v1.IntentCandidateDiscovery{}, err
	}
	if out.CatalogDigest != current(r).TemplateCatalogDigest {
		return v1.IntentCandidateDiscovery{}, ErrConflict
	}
	return out, nil
}
