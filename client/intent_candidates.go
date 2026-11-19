package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

// DiscoverIntentCandidates performs explicit bounded provider reads, never writes.
func (c *Client) DiscoverIntentCandidates(ctx context.Context, in v1.IntentCandidateInput) (v1.IntentCandidateDiscovery, error) {
	var out v1.IntentCandidateDiscovery
	body, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	if len(body) > 32<<10 {
		return out, errors.New("candidate input exceeds 32 KiB")
	}
	err = c.scopedRequest(ctx, http.MethodPost, "/api/intents/candidates", body, 256<<10, &out)
	return out, err
}
