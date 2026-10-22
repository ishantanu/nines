package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func (c *Client) Investigations(ctx context.Context, cursor string, limit int) (v1.InvestigationList, error) {
	var out v1.InvestigationList
	if limit < 1 || limit > 50 {
		return out, errors.New("limit must be 1–50")
	}
	err := c.scopedRequest(ctx, http.MethodGet, "/api/investigations?"+url.Values{"limit": {strconv.Itoa(limit)}, "cursor": {cursor}}.Encode(), nil, 1<<20, &out)
	return out, err
}
func (c *Client) Investigation(ctx context.Context, id string) (v1.Investigation, error) {
	var out v1.Investigation
	if !identifier.MatchString(id) {
		return out, errors.New("invalid investigation ID")
	}
	err := c.scopedRequest(ctx, http.MethodGet, "/api/investigations/"+id, nil, 3<<20, &out)
	return out, err
}
func (c *Client) CreateInvestigation(ctx context.Context, in v1.CreateInvestigation) (v1.Investigation, error) {
	return c.investigationMutation(ctx, "/api/investigations", in)
}
func (c *Client) AppendInvestigationMessage(ctx context.Context, id string, in v1.AppendInvestigationMessage) (v1.Investigation, error) {
	if !identifier.MatchString(id) {
		return v1.Investigation{}, errors.New("invalid investigation ID")
	}
	return c.investigationMutation(ctx, "/api/investigations/"+id+"/messages", in)
}
func (c *Client) CaptureInvestigationEvidence(ctx context.Context, id string, in v1.CaptureInvestigationEvidence) (v1.Investigation, error) {
	if !identifier.MatchString(id) {
		return v1.Investigation{}, errors.New("invalid investigation ID")
	}
	return c.investigationMutation(ctx, "/api/investigations/"+id+"/evidence", in)
}
func (c *Client) investigationMutation(ctx context.Context, path string, in any) (v1.Investigation, error) {
	var out v1.Investigation
	b, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	err = c.scopedRequest(ctx, http.MethodPost, path, b, 3<<20, &out)
	return out, err
}
