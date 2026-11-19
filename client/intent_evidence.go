package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

// PrepareIntentEvidence performs explicit provider reads. Its default timeout is
// 80 seconds; caller contexts and explicitly configured HTTP timeouts still apply.
func (c *Client) PrepareIntentEvidence(ctx context.Context, in v1.IntentEvidenceInput) (v1.IntentEvidence, error) {
	var out v1.IntentEvidence
	body, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	if len(body) > 32<<10 {
		return out, errors.New("evidence request exceeds 32 KiB")
	}
	requestClient := *c
	if !c.customTimeout {
		h := *c.http
		h.Timeout = 80 * time.Second
		requestClient.http = &h
	}
	err = requestClient.scopedRequest(ctx, http.MethodPost, "/api/intents/evidence", body, 256<<10, &out)
	return out, err
}
