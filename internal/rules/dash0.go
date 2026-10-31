package rules

import (
	"context"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/dash0"
	"github.com/ishantanu/nines/internal/model"
)

func nativeClient(t Target) (*dash0.Client, error) {
	return dash0.New(t.QueryURL, t.Dataset, t.BearerToken)
}
func (m *Manager) planNative(ctx context.Context, s model.SLO, t Target) (Plan, error) {
	c, err := nativeClient(t)
	if err != nil {
		return Plan{}, err
	}
	p, err := c.Prepare(ctx, s, m.Tenant, t.Connection, t.Namespace)
	if err != nil {
		return Plan{}, err
	}
	b, err := yaml.Marshal(p.Document)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{Target: t, NativeSLO: &p, Group: Group{Name: p.Origin, Rules: []Rule{}}, YAML: string(b), SLORevision: model.ConfigRevision(s)}
	plan.Digest = model.DefinitionDigest(struct {
		Fingerprint string
		Tenant      string
		Definition  model.SLODefinition
		Revision    int64
		Target      Target
		Publication dash0.Publication
	}{c.SourceFingerprint(), m.Tenant, model.DefinitionFromSLO(s), model.ConfigRevision(s), t, p})
	return plan, nil
}
func (m *Manager) applyNative(ctx context.Context, s model.SLO, p Plan) (Deployment, error) {
	if err := m.authorizeWrite(ctx, s, p.Digest); err != nil {
		return Deployment{}, err
	}
	c, err := nativeClient(p.Target)
	if err != nil {
		return Deployment{}, err
	}
	state, err := c.Publish(ctx, *p.NativeSLO)
	if err != nil {
		unknown := nativeEvaluation()
		return Deployment{State: "unverified", Digest: p.Digest, Target: p.Target, Group: p.Group.Name, Message: "Native SLO publication outcome is unverified. Refresh and inspect before retrying.", Evaluation: &unknown}, err
	}
	stage := ruleStage("verified", "native_slo_stored", "The native SLO definition was read back and matches the reviewed content.", "Check native evaluation and Nines SLI evidence separately. Native check rules were not published.")
	evaluation := nativeEvaluation()
	return Deployment{State: state, Digest: p.Digest, Target: p.Target, Group: p.Group.Name, Message: "Dash0 native SLO publication; Nines collection and burn alerts remain independent.", Activation: &stage, Evaluation: &evaluation}, err
}
func nativeEvaluation() v1.IntegrationStage {
	return ruleStage("unknown", "native_evaluation_unverified", "Native Dash0 evaluation has not been verified.", "Inspect Dash0 SLO metrics and alert configuration; a stored definition does not prove evaluation or notification delivery.")
}
func (m *Manager) inspectNative(ctx context.Context, s model.SLO, t Target, _ time.Time) (v1.IntegrationStage, v1.IntegrationStage) {
	p, err := m.planNative(ctx, s, t)
	if err != nil {
		return ruleStage("unknown", "native_slo_unavailable", "Native SLO inspection failed.", "Check credentials, ownership and the supported definition subset."), nativeEvaluation()
	}
	state, code, message := "out_of_date", "native_slo_drifted", "Native SLO content differs from the saved definition."
	if p.NativeSLO.PriorVersion == "" {
		state, code, message = "not_deployed", "native_slo_missing", "No native SLO was found at the pinned origin."
	} else if p.NativeSLO.Matches {
		state, code, message = "stored", "native_slo_matches", "Native SLO content matches the saved definition."
	}
	return ruleStage(state, code, message, "Review a fresh native SLO plan before publishing changes. Check evaluation independently."), nativeEvaluation()
}

func (m *Manager) withdrawNative(ctx context.Context, s model.SLO, p Plan) (Deployment, error) {
	if err := m.authorizeWrite(ctx, s, p.Digest); err != nil {
		return Deployment{}, err
	}
	c, err := nativeClient(p.Target)
	if err != nil {
		return Deployment{}, err
	}
	if err = c.Disable(ctx, *p.NativeSLO); err != nil {
		return Deployment{}, err
	}
	evaluation := nativeEvaluation()
	return Deployment{State: "withdrawn", Digest: p.Digest, Target: p.Target, Group: p.Group.Name, Message: "The owned native SLO is absent or its disabled setting was verified. It is retained in Dash0; runtime evaluation cessation and metric expiry were not verified.", Evaluation: &evaluation}, nil
}
