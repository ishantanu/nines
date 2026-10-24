package client

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func (c *Client) Audit(ctx context.Context, q v1.AuditQuery) (v1.AuditPage, error) {
	var out v1.AuditPage
	values := url.Values{}
	for key, value := range map[string]string{"conversation": q.Conversation, "actor": q.Actor, "resource": q.Resource, "action": q.Action, "outcome": q.Outcome, "cursor": q.Cursor} {
		if value != "" {
			values.Set(key, value)
		}
	}
	if q.Limit != 0 {
		values.Set("limit", strconv.Itoa(q.Limit))
	}
	if !q.From.IsZero() {
		values.Set("from", q.From.Format(time.RFC3339Nano))
	}
	if !q.To.IsZero() {
		values.Set("to", q.To.Format(time.RFC3339Nano))
	}
	err := c.scopedRequest(ctx, "GET", "/api/audit?"+values.Encode(), nil, 3<<20, &out)
	if err == nil {
		for _, item := range out.Items {
			if item.Tenant != c.tenant {
				return out, errors.New("unexpected audit item tenant")
			}
		}
	}
	return out, err
}
func (c *Client) AuditRecord(ctx context.Context, id string) (v1.AuditRecord, error) {
	var out v1.AuditRecord
	if !identifier.MatchString(id) {
		return out, errors.New("invalid audit record ID")
	}
	err := c.scopedRequest(ctx, "GET", "/api/audit/"+id, nil, 3<<20, &out)
	if err == nil && out.ID != id {
		err = errors.New("unexpected audit record identity")
	}
	return out, err
}

func (c *Client) AuditArchiveStatus(ctx context.Context, id string) (v1.AuditArchiveStatus, error) {
	var out v1.AuditArchiveStatus
	path := "/api/audit/archive"
	if id != "" {
		if !identifier.MatchString(id) {
			return out, errors.New("invalid audit record ID")
		}
		path += "?record=" + url.QueryEscape(id)
	}
	err := c.scopedRequest(ctx, "GET", path, nil, 32<<10, &out)
	if err == nil && out.RecordID != id {
		err = errors.New("unexpected archive status record")
	}
	return out, err
}
