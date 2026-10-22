package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func runPath(review, id string) (string, error) {
	if !identifier.MatchString(review) || (id != "" && !identifier.MatchString(id)) {
		return "", errors.New("invalid review or run ID")
	}
	path := "/api/investigations/" + review + "/runs"
	if id != "" {
		path += "/" + id
	}
	return path, nil
}
func (c *Client) InvestigationRuns(ctx context.Context, review string) (v1.InvestigationRunList, error) {
	var out v1.InvestigationRunList
	path, err := runPath(review, "")
	if err == nil {
		err = c.scopedRequest(ctx, http.MethodGet, path, nil, 3<<20, &out)
	}
	return out, err
}
func (c *Client) InvestigationRun(ctx context.Context, review, id string) (v1.InvestigationRun, error) {
	var out v1.InvestigationRun
	if id == "" {
		return out, errors.New("run ID is required")
	}
	path, err := runPath(review, id)
	if err == nil {
		err = c.scopedRequest(ctx, http.MethodGet, path, nil, 256<<10, &out)
	}
	return out, err
}
func (c *Client) CreateInvestigationRun(ctx context.Context, review string, in v1.CreateInvestigationRun) (v1.InvestigationRun, error) {
	path, err := runPath(review, "")
	if err != nil {
		return v1.InvestigationRun{}, err
	}
	return c.runMutation(ctx, path, in)
}
func (c *Client) CancelInvestigationRun(ctx context.Context, review, id string, in v1.CancelInvestigationRun) (v1.InvestigationRun, error) {
	if id == "" {
		return v1.InvestigationRun{}, errors.New("run ID is required")
	}
	path, err := runPath(review, id)
	if err != nil {
		return v1.InvestigationRun{}, err
	}
	return c.runMutation(ctx, path+"/cancel", in)
}
func (c *Client) runMutation(ctx context.Context, path string, in any) (v1.InvestigationRun, error) {
	var out v1.InvestigationRun
	b, err := json.Marshal(in)
	if err == nil {
		err = c.scopedRequest(ctx, http.MethodPost, path, b, 256<<10, &out)
	}
	return out, err
}
