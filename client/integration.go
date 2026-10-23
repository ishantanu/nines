package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

// Integration reads live collection and external rule evaluation evidence.
func (c *Client) Integration(ctx context.Context, id string) (v1.IntegrationStatus, error) {
	var out v1.IntegrationStatus
	if !identifier.MatchString(id) {
		return out, errors.New("invalid SLO identifier")
	}
	err := c.contextRead(ctx, "/api/slos/"+id+"/integration", &out)
	return out, err
}

// SetRuleAutomation pauses/resumes reconciliation of an existing saved SLO.
// Only a tenant owner can change this control. It never edits the SLO.
func (c *Client) SetRuleAutomation(ctx context.Context, id string, revision int64, paused bool) (v1.RuleAutomationResult, error) {
	var out v1.RuleAutomationResult
	if !identifier.MatchString(id) || revision < 1 {
		return out, errors.New("provide a valid SLO and positive expected revision")
	}
	body, _ := json.Marshal(struct {
		ExpectedRevision int64 `json:"expectedRevision"`
		Paused           bool  `json:"paused"`
	}{revision, paused})
	e := c.scopedRequest(ctx, http.MethodPost, "/api/slos/"+id+"/rules/automation", body, 1<<20, &out)
	return out, e
}
