package client

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

// RecordIntentEvidence reads provider evidence and saves a version-fenced receipt.
// The default timeout is 85 seconds; explicit HTTP timeouts remain effective.
func (c *Client) RecordIntentEvidence(ctx context.Context, id string, in v1.RecordIntentEvidence) (v1.IntentConversation, error) {
	p, err := conversationPath(id)
	if err != nil {
		return v1.IntentConversation{}, err
	}
	b, err := json.Marshal(in)
	if err != nil {
		return v1.IntentConversation{}, err
	}
	if len(b) > 4096 {
		return v1.IntentConversation{}, errors.New("evidence request exceeds 4 KiB")
	}
	requestClient := *c
	if !c.customTimeout {
		h := *c.http
		h.Timeout = 85 * time.Second
		requestClient.http = &h
	}
	return requestClient.conversationWrite(ctx, p+"/evidence", in)
}
