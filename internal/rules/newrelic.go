package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/newrelic"
)

func newrelicClient(t Target) (*newrelic.Client, error) {
	return newrelic.New(t.QueryURL, t.NewRelicAccountID, t.NewRelicUserKey)
}
func (m *Manager) planNewRelic(ctx context.Context, s model.SLO, t Target) (Plan, error) {
	c, err := newrelicClient(t)
	if err != nil {
		return Plan{}, err
	}
	p, err := c.Prepare(ctx, s, m.Tenant, t.Connection, t.Namespace, t.NewRelicEntityGUID)
	if err != nil {
		return Plan{}, err
	}
	data, err := json.Marshal(p.Document)
	if err != nil {
		return Plan{}, err
	}
	var object map[string]any
	if err = yaml.Unmarshal(data, &object); err != nil {
		return Plan{}, err
	}
	data, err = yaml.Marshal(object)
	if err != nil {
		return Plan{}, err
	}
	out := Plan{Target: t, NativeNewRelic: &p, Group: Group{Name: p.Origin, Rules: []Rule{}}, YAML: string(data), SLORevision: model.ConfigRevision(s)}
	out.Digest = model.DefinitionDigest(struct {
		Identity    string
		Target      Target
		Publication newrelic.Publication
	}{c.SourceFingerprint(), t, p})
	return out, nil
}
func newrelicEvaluation() v1.IntegrationStage {
	return ruleStage("unknown", "newrelic_evaluation_unverified", "Native New Relic SLO evaluation has not been verified.", "Read native provider results in Nines or inspect the SLO in New Relic. Nines SLI and burn alerts are independent; native monitors and notification delivery are not published or verified.")
}
func (m *Manager) applyNewRelic(ctx context.Context, s model.SLO, p Plan) (Deployment, error) {
	if err := m.authorizeWrite(ctx, s, p.Digest); err != nil {
		return Deployment{}, err
	}
	c, err := newrelicClient(p.Target)
	if err != nil {
		return Deployment{}, err
	}
	saved, state, err := c.Publish(ctx, *p.NativeNewRelic)
	evaluation := newrelicEvaluation()
	out := Deployment{State: state, Digest: p.Digest, Target: p.Target, Group: p.Group.Name, NativeID: saved.ID, NativeURL: c.NativeURL(saved.GUID), NativeQueries: &saved.ResultQueries, Evaluation: &evaluation, Message: "Native New Relic SLO revision was read back. Previous revisions are retained; native burn monitors were not published."}
	if err != nil {
		out.State = "unverified"
		return out, err
	}
	stage := ruleStage("verified", "newrelic_slo_stored", fmt.Sprintf("Native New Relic SLO %s matches the reviewed definition.", saved.ID), "Inspect native evaluation and notification delivery separately.")
	out.Activation = &stage
	return out, nil
}
func (m *Manager) inspectNewRelic(ctx context.Context, s model.SLO, t Target, _ time.Time) (v1.IntegrationStage, v1.IntegrationStage) {
	p, err := m.planNewRelic(ctx, s, t)
	if err != nil {
		return ruleStage("unknown", "newrelic_slo_unavailable", "Native New Relic SLO content could not be verified.", "Check credentials, supported windows and ownership; inspect duplicate or edited native resources."), newrelicEvaluation()
	}
	if p.NativeNewRelic.Matches {
		return ruleStage("stored", "newrelic_slo_matches", fmt.Sprintf("Native New Relic SLO %s matches the current definition. %s", p.NativeNewRelic.NativeID, p.NativeNewRelic.URL), "Inspect provider evaluation separately."), newrelicEvaluation()
	}
	return ruleStage("not_deployed", "newrelic_slo_missing", "The current native New Relic SLO revision was not found.", "Review and publish its plan."), newrelicEvaluation()
}
func (m *Manager) withdrawNewRelic(ctx context.Context, s model.SLO, p Plan) (Deployment, error) {
	if err := m.authorizeWrite(ctx, s, p.Digest); err != nil {
		return Deployment{}, err
	}
	c, err := newrelicClient(p.Target)
	if err != nil {
		return Deployment{}, err
	}
	rows, err := c.OwnedRevisions(ctx, p.NativeNewRelic.EntityGUID, p.NativeNewRelic.Owner)
	if err != nil {
		return Deployment{}, err
	}
	if len(rows) > 0 {
		return Deployment{}, model.Invalid("New Relic native revisions remain; remove them in New Relic and refresh the plan to verify removal. Nines does not unconditionally delete native SLOs")
	}
	return Deployment{State: "withdrawn", Digest: p.Digest, Target: p.Target, Group: p.Group.Name, Message: "Absence of all native New Relic SLO revisions in this ownership scope was verified."}, nil
}
