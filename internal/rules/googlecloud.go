package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/googlecloud"
	"github.com/ishantanu/nines/internal/model"
)

func googleClient(t Target) (*googlecloud.Client, error) {
	auth := t.GoogleAuth
	if t.BearerToken != "" {
		auth = "access-token"
	}
	return googlecloud.New(t.QueryURL, t.GoogleProjectID, t.GoogleServiceID, auth, t.BearerToken)
}
func (m *Manager) planGoogle(ctx context.Context, s model.SLO, t Target) (Plan, error) {
	c, err := googleClient(t)
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
	out := Plan{Target: t, NativeGoogleCloud: &p, Group: Group{Name: p.Origin, Rules: []Rule{}}, YAML: string(data), SLORevision: model.ConfigRevision(s)}
	out.Digest = model.DefinitionDigest(struct {
		Identity    string
		Target      Target
		Publication googlecloud.Publication
	}{c.SourceFingerprint(), t, p})
	return out, nil
}
func googleEvaluation() v1.IntegrationStage {
	return ruleStage("unknown", "google_evaluation_unverified", "Native Google Cloud SLO evaluation has not been verified.", "Inspect Cloud Monitoring native SLO signals and notification delivery separately from the stored definition and Nines SLI.")
}
func (m *Manager) applyGoogle(ctx context.Context, s model.SLO, p Plan) (Deployment, error) {
	if err := m.authorizeWrite(ctx, s, p.Digest); err != nil {
		return Deployment{}, err
	}
	c, err := googleClient(p.Target)
	if err != nil {
		return Deployment{}, err
	}
	state, err := c.Publish(ctx, *p.NativeGoogleCloud)
	evaluation := googleEvaluation()
	out := Deployment{State: state, Digest: p.Digest, Target: p.Target, Group: p.Group.Name, Evaluation: &evaluation, Message: "Native Google Cloud SLO revision was stored. Previous revisions are retained; native alert policies were not published."}
	if err != nil {
		out.State = "unverified"
		return out, err
	}
	stage := ruleStage("verified", "google_slo_stored", "The immutable native SLO revision was read back and matches the reviewed content.", "Verify provider evaluation and notification delivery independently.")
	out.Activation = &stage
	return out, nil
}
func (m *Manager) inspectGoogle(ctx context.Context, s model.SLO, t Target, now time.Time) (v1.IntegrationStage, v1.IntegrationStage) {
	p, err := m.planGoogle(ctx, s, t)
	if err != nil {
		return ruleStage("unknown", "google_slo_unavailable", "Native SLO content could not be verified.", "Check credentials, IAM, ownership and the supported definition subset."), googleEvaluation()
	}
	if p.NativeGoogleCloud.Matches {
		evaluation := googleEvaluation()
		c, e := googleClient(t)
		if e == nil {
			if value, at, e := c.NativeHealth(ctx, p.NativeGoogleCloud.Origin, now); e == nil {
				evaluation = ruleStage("evaluating", "google_slo_health_observed", fmt.Sprintf("A fresh native Google Cloud SLI signal was observed: %.3f%%. This is provider evidence, separate from Nines SLI.", 100*value), "Verify measurement coverage and notification delivery separately.")
				evaluation.ObservedAt = &at
			}
		}
		return ruleStage("stored", "google_slo_matches", "The current native SLO revision matches the saved definition.", "Inspect native evaluation separately."), evaluation
	}
	return ruleStage("not_deployed", "google_slo_missing", "The current native SLO revision was not found.", "Review and apply its publication plan."), googleEvaluation()
}

// Google exposes no conditional native-SLO DELETE. A reviewed withdrawal only
// confirms administrator-performed removal, never deletes a possibly edited SLO.
func (m *Manager) withdrawGoogle(ctx context.Context, s model.SLO, p Plan) (Deployment, error) {
	if err := m.authorizeWrite(ctx, s, p.Digest); err != nil {
		return Deployment{}, err
	}
	c, err := googleClient(p.Target)
	if err != nil {
		return Deployment{}, err
	}
	retained, err := c.OwnedRevisions(ctx, p.NativeGoogleCloud.Document.UserLabels["nines_owner"])
	if err != nil {
		return Deployment{}, err
	}
	if len(retained) > 0 {
		return Deployment{}, model.Invalid("Google Cloud native revisions remain in this ownership scope; remove them in Google Cloud, then refresh the plan to verify removal. Nines does not perform unconditional deletion")
	}
	stage := googleEvaluation()
	return Deployment{State: "withdrawn", Digest: p.Digest, Target: p.Target, Group: p.Group.Name, Message: "Absence of all native revisions in this ownership scope was verified. Alert policies and metric expiry are not managed by this operation.", Evaluation: &stage}, nil
}
