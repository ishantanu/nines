package client

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func (c *Client) MeasurementQuality(ctx context.Context, id string, expected int64, cohorts bool) (v1.MeasurementQuality, error) {
	var out v1.MeasurementQuality
	if !identifier.MatchString(id) || expected < 0 {
		return out, fmt.Errorf("invalid quality request")
	}
	q := url.Values{}
	if expected > 0 {
		q.Set("expectedRevision", strconv.FormatInt(expected, 10))
	}
	if cohorts {
		q.Set("cohorts", "true")
	}
	path := "/api/slos/" + id + "/quality"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	err := c.contextRead(ctx, path, &out)
	if err == nil && (out.APIVersion != v1.APIVersion || out.Tenant != c.tenant || out.SLOID != id || out.Revision < 1 || (expected > 0 && out.Revision != expected)) {
		return v1.MeasurementQuality{}, fmt.Errorf("quality response scope or revision changed")
	}
	return out, err
}

func (c *Client) ErrorBudgetDecision(ctx context.Context, id string, expected int64) (v1.ErrorBudgetDecision, error) {
	var out v1.ErrorBudgetDecision
	if !identifier.MatchString(id) || expected < 0 {
		return out, fmt.Errorf("invalid budget decision request")
	}
	path := "/api/slos/" + id + "/budget"
	if expected > 0 {
		path += "?expectedRevision=" + strconv.FormatInt(expected, 10)
	}
	err := c.contextRead(ctx, path, &out)
	if err == nil && (out.APIVersion != v1.APIVersion || out.Tenant != c.tenant || out.SLOID != id || out.Revision < 1 || (expected > 0 && out.Revision != expected)) {
		return v1.ErrorBudgetDecision{}, fmt.Errorf("budget response scope or revision changed")
	}
	return out, err
}
