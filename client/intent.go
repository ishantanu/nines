package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

// PlanIntent previews a typed intent without creating SLOs, links or rules.
func (c *Client) PlanIntent(ctx context.Context, in v1.PlanIntentInput) (v1.IntentPlan, error) {
	var out v1.IntentPlan
	b, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	err = c.scopedRequest(ctx, http.MethodPost, "/api/intents/plan", b, 1<<20, &out)
	return out, err
}
func (c *Client) DraftIntent(ctx context.Context, in v1.DraftIntentInput) (v1.IntentDraft, error) {
	var out v1.IntentDraft
	b, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	err = c.scopedRequest(ctx, http.MethodPost, "/api/intents/draft", b, 1<<20, &out)
	return out, err
}
func (c *Client) BacktestIntent(ctx context.Context, in v1.IntentBacktestInput) (v1.IntentBacktest, error) {
	var out v1.IntentBacktest
	b, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	err = c.scopedRequest(ctx, http.MethodPost, "/api/intents/backtest", b, 1<<20, &out)
	return out, err
}
func (c *Client) CheckBinding(ctx context.Context, id string, revision int64) (v1.BindingHealth, error) {
	var out v1.BindingHealth
	if !identifier.MatchString(id) || revision < 1 {
		return out, errors.New("valid binding ID and positive revision required")
	}
	err := c.scopedRequest(ctx, http.MethodGet, "/api/bindings/"+id+"/health?expectedRevision="+strconv.FormatInt(revision, 10), nil, 1<<20, &out)
	return out, err
}
