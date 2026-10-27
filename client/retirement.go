package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func (c *Client) PlanRetirement(ctx context.Context, id, action string) (v1.RetirementPlan, error) {
	var out v1.RetirementPlan
	if !identifier.MatchString(id) || (action != "archive" && action != "delete") {
		return out, errors.New("provide a valid SLO and archive or delete action")
	}
	e := c.contextRead(ctx, "/api/slos/"+id+"/retirement/plan?action="+url.QueryEscape(action), &out)
	return out, e
}
func (c *Client) ApplyRetirement(ctx context.Context, id string, in v1.RetirementInput) (v1.RetirementReceipt, error) {
	var out v1.RetirementReceipt
	if !identifier.MatchString(id) || in.ExpectedRevision < 1 || in.Digest == "" || (in.Action != "archive" && in.Action != "delete") {
		return out, errors.New("provide a valid SLO and reviewed retirement revision/digest")
	}
	if in.Action == "delete" && in.ConfirmName != id {
		return out, errors.New("confirmName must match the SLO being permanently deleted")
	}
	body, e := json.Marshal(in)
	if e != nil {
		return out, e
	}
	e = c.scopedRequest(ctx, http.MethodPost, "/api/slos/"+id+"/retirement/apply", body, 1<<20, &out)
	return out, e
}
func (c *Client) Retirement(ctx context.Context, id string) (v1.RetirementReceipt, error) {
	var out v1.RetirementReceipt
	if !identifier.MatchString(id) {
		return out, errors.New("invalid SLO identifier")
	}
	e := c.contextRead(ctx, "/api/slos/"+id+"/retirement", &out)
	return out, e
}
func (c *Client) RetryRetirement(ctx context.Context, id string) (v1.RetirementReceipt, error) {
	var out v1.RetirementReceipt
	if !identifier.MatchString(id) {
		return out, errors.New("invalid SLO identifier")
	}
	e := c.scopedRequest(ctx, http.MethodPost, "/api/slos/"+id+"/retirement/retry", []byte(`{}`), 1<<20, &out)
	return out, e
}
