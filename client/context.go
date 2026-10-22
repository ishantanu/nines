package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func (c *Client) SearchResources(ctx context.Context, in v1.SearchOptions) (v1.ResourceSearch, error) {
	var out v1.ResourceSearch
	if in.Kind != "" && in.Kind != v1.SLOKind && in.Kind != v1.JourneyKind || in.Limit < 0 || in.Limit > 100 || len(in.Project) > 200 || len(in.Query) > 200 || len(in.Cursor) > 2048 || in.LinkedSLO != "" && !identifier.MatchString(in.LinkedSLO) {
		return out, errors.New("invalid search parameters")
	}
	q := url.Values{}
	for k, v := range map[string]string{"kind": in.Kind, "project": in.Project, "query": in.Query, "cursor": in.Cursor, "linkedSLO": in.LinkedSLO} {
		if v != "" {
			q.Set(k, v)
		}
	}
	if in.Limit > 0 {
		q.Set("limit", strconv.Itoa(in.Limit))
	}
	err := c.contextRead(ctx, "/api/resources/search?"+q.Encode(), &out)
	return out, err
}
func contextURL(id string, in v1.ContextOptions, journey bool) (string, error) {
	if !identifier.MatchString(id) || in.ExpectedRevision < 0 || len(in.Cursor) > 2048 || in.Limit < 0 || in.Limit > 10 || !journey && (in.Cursor != "" || in.Limit != 0) {
		return "", errors.New("invalid context parameters")
	}
	q := url.Values{}
	if in.ExpectedRevision > 0 {
		q.Set("expectedRevision", strconv.FormatInt(in.ExpectedRevision, 10))
	}
	if !in.At.IsZero() {
		q.Set("at", in.At.UTC().Format(time.RFC3339Nano))
	}
	if in.Limit > 0 {
		q.Set("limit", strconv.Itoa(in.Limit))
	}
	if in.Cursor != "" {
		q.Set("cursor", in.Cursor)
	}
	prefix := "/api/slos/"
	if journey {
		prefix = "/api/journeys/"
	}
	path := prefix + id + "/context"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return path, nil
}
func (c *Client) SLOContext(ctx context.Context, id string, in v1.ContextOptions) (v1.SLOContext, error) {
	var out v1.SLOContext
	path, err := contextURL(id, in, false)
	if err == nil {
		err = c.contextRead(ctx, path, &out)
	}
	return out, err
}
func (c *Client) JourneyContext(ctx context.Context, id string, in v1.ContextOptions) (v1.JourneyContext, error) {
	var out v1.JourneyContext
	path, err := contextURL(id, in, true)
	if err == nil {
		err = c.contextRead(ctx, path, &out)
	}
	return out, err
}
func (c *Client) contextRead(ctx context.Context, path string, out any) error {
	return c.scopedRequest(ctx, http.MethodGet, path, nil, 256<<10, out)
}
func (c *Client) scopedRequest(ctx context.Context, method, path string, body []byte, limit int64, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.origin+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Nines-Tenant", c.tenant)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("nines context request failed")
	}
	defer func() {
		_ = res.Body.Close()
	}()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return &APIError{res.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil || int64(len(data)) > limit {
		return errors.New("scoped response unreadable or exceeds bounds")
	}
	var scope struct {
		Tenant     string `json:"tenant"`
		APIVersion string `json:"apiVersion"`
	}
	if json.Unmarshal(data, &scope) != nil || scope.Tenant != c.tenant || scope.APIVersion != v1.APIVersion {
		return errors.New("invalid scoped context response")
	}
	return json.Unmarshal(data, out)
}
