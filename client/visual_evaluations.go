package client

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func visualPath(journey string) (string, error) {
	if !identifier.MatchString(journey) {
		return "", errors.New("invalid journey ID")
	}
	return "/api/journeys/" + journey + "/visual-evaluations", nil
}
func (c *Client) VisualEvaluations(ctx context.Context, journey string) ([]v1.VisualEvaluation, error) {
	var result v1.VisualEvaluationList
	var out []v1.VisualEvaluation
	p, e := visualPath(journey)
	if e == nil {
		e = c.scopedRequest(ctx, "GET", p, nil, 512<<10, &result)
	}
	if e == nil {
		out = result.Items
		for _, r := range out {
			if r.Tenant != c.tenant || r.JourneyID != journey {
				return nil, errors.New("visual receipt scope mismatch")
			}
		}
	}
	return out, e
}
func (c *Client) AssessVisual(ctx context.Context, journey string, in v1.CreateVisualEvaluation) (v1.VisualEvaluation, error) {
	var out v1.VisualEvaluation
	p, e := visualPath(journey)
	if e != nil {
		return out, e
	}
	if !in.Consent || in.NoticeVersion != v1.VisualConsentNotice || len(in.RouteDigest) != 64 {
		return out, errors.New("explicit one-image provider-sharing consent is required")
	}
	b, e := json.Marshal(in)
	if e != nil || len(b) > 3<<20 {
		return out, errors.New("visual request exceeds bounds")
	}
	e = c.scopedRequest(ctx, "POST", p, b, 64<<10, &out)
	if e == nil && (out.ID != in.ID || out.JourneyID != journey || out.JourneyRevision != in.JourneyRevision || out.Tenant != c.tenant) {
		return out, errors.New("visual receipt identity mismatch")
	}
	return out, e
}
func (c *Client) ReviewVisual(ctx context.Context, journey, id string, in v1.ReviewVisualEvaluation) (v1.VisualEvaluation, error) {
	var out v1.VisualEvaluation
	p, e := visualPath(journey)
	if e != nil {
		return out, e
	}
	if _, e = uuid.Parse(id); e != nil {
		return out, errors.New("invalid visual evaluation UUID")
	}
	b, e := json.Marshal(in)
	if e == nil {
		e = c.scopedRequest(ctx, "POST", p+"/"+id+"/review", b, 64<<10, &out)
	}
	if e == nil && (out.ID != id || out.JourneyID != journey || out.Tenant != c.tenant) {
		e = errors.New("visual receipt identity mismatch")
	}
	return out, e
}
func (c *Client) DeleteSessionReplay(ctx context.Context, id string, version int64, confirmation string) error {
	p, e := replayPath(id)
	if e != nil {
		return e
	}
	if version < 1 || confirmation != id {
		return errors.New("provide expected version and exact recording ID confirmation")
	}
	b, _ := json.Marshal(map[string]any{"expectedVersion": version, "confirmId": confirmation})
	var out struct {
		Deleted bool `json:"deleted"`
	}
	if e = c.scopedRequest(ctx, "DELETE", p, b, 1024, &out); e != nil {
		return e
	}
	if !out.Deleted {
		return errors.New("deletion was not confirmed")
	}
	return nil
}

func (c *Client) VisualCapabilities(ctx context.Context) (v1.VisualCapabilities, error) {
	var out v1.VisualCapabilities
	e := c.scopedRequest(ctx, "GET", "/api/visual-evaluations/capabilities", nil, 16<<10, &out)
	return out, e
}
