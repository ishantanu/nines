package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/datadog"
	"github.com/ishantanu/nines/internal/model"
)

func datadogClient(t Target) (*datadog.Client, error) {
	return datadog.New(t.QueryURL, t.DatadogAPIKey, t.DatadogApplicationKey)
}
func (m *Manager) planDatadog(ctx context.Context, s model.SLO, t Target) (Plan, error) {
	c, err := datadogClient(t)
	if err != nil {
		return Plan{}, err
	}
	p, err := c.Prepare(ctx, s, m.Tenant, t.Connection, t.Namespace)
	if err != nil {
		return Plan{}, err
	}
	data, err := json.Marshal(p.Document)
	if err != nil {
		return Plan{}, err
	}
	var object map[string]any
	if err = json.Unmarshal(data, &object); err != nil {
		return Plan{}, err
	}
	data, err = yaml.Marshal(object)
	if err != nil {
		return Plan{}, err
	}
	out := Plan{Target: t, NativeDatadog: &p, Group: Group{Name: p.Origin, Rules: []Rule{}}, YAML: string(data), SLORevision: model.ConfigRevision(s)}
	out.Digest = model.DefinitionDigest(struct {
		Identity    string
		Target      Target
		Publication datadog.Publication
	}{c.SourceFingerprint(), t, p})
	return out, nil
}
func datadogEvaluation() v1.IntegrationStage {
	return ruleStage("unknown", "datadog_evaluation_unverified", "Native Datadog SLO evaluation has not been verified.", "Read native provider results in Nines or inspect the SLO in Datadog. Nines SLI and burn alerts are independent; native monitors and notification delivery are not published or verified.")
}
func (m *Manager) applyDatadog(ctx context.Context, s model.SLO, p Plan) (Deployment, error) {
	if err := m.authorizeWrite(ctx, s, p.Digest); err != nil {
		return Deployment{}, err
	}
	c, err := datadogClient(p.Target)
	if err != nil {
		return Deployment{}, err
	}
	saved, state, err := c.Publish(ctx, *p.NativeDatadog)
	evaluation := datadogEvaluation()
	out := Deployment{State: state, Digest: p.Digest, Target: p.Target, Group: p.Group.Name, NativeID: saved.ID, NativeURL: c.NativeURL(saved.ID), Evaluation: &evaluation, Message: "Native Datadog SLO revision was read back. Previous revisions are retained; native burn monitors were not published."}
	if err != nil {
		out.State = "unverified"
		return out, err
	}
	stage := ruleStage("verified", "datadog_slo_stored", fmt.Sprintf("Native Datadog SLO %s matches the reviewed definition.", saved.ID), "Inspect native evaluation and notification delivery separately.")
	out.Activation = &stage
	return out, nil
}
func (m *Manager) inspectDatadog(ctx context.Context, s model.SLO, t Target, _ time.Time) (v1.IntegrationStage, v1.IntegrationStage) {
	p, err := m.planDatadog(ctx, s, t)
	if err != nil {
		return ruleStage("unknown", "datadog_slo_unavailable", "Native Datadog SLO content could not be verified.", "Check credentials, supported windows and ownership; inspect duplicate or edited native resources."), datadogEvaluation()
	}
	if p.NativeDatadog.Matches {
		return ruleStage("stored", "datadog_slo_matches", fmt.Sprintf("Native Datadog SLO %s matches the current definition. %s", p.NativeDatadog.NativeID, p.NativeDatadog.URL), "Inspect provider evaluation separately."), datadogEvaluation()
	}
	return ruleStage("not_deployed", "datadog_slo_missing", "The current native Datadog SLO revision was not found.", "Review and publish its plan."), datadogEvaluation()
}
func (m *Manager) withdrawDatadog(ctx context.Context, s model.SLO, p Plan) (Deployment, error) {
	if err := m.authorizeWrite(ctx, s, p.Digest); err != nil {
		return Deployment{}, err
	}
	c, err := datadogClient(p.Target)
	if err != nil {
		return Deployment{}, err
	}
	rows, err := c.OwnedRevisions(ctx, p.NativeDatadog.Owner)
	if err != nil {
		return Deployment{}, err
	}
	if len(rows) > 0 {
		return Deployment{}, model.Invalid("Datadog native revisions remain; remove them in Datadog and refresh the plan to verify removal. Nines does not unconditionally delete native SLOs")
	}
	return Deployment{State: "withdrawn", Digest: p.Digest, Target: p.Target, Group: p.Group.Name, Message: "Absence of all native Datadog SLO revisions in this ownership scope was verified."}, nil
}
