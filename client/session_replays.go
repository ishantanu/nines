package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func replayPath(id string) (string, error) {
	if !identifier.MatchString(id) {
		return "", errors.New("invalid session replay ID")
	}
	return "/api/session-replays/" + id, nil
}
func (c *Client) SessionReplays(ctx context.Context, kind, id string) (v1.SessionReplayList, error) {
	var out v1.SessionReplayList
	path := "/api/session-replays"
	if kind != "" || id != "" {
		if !identifier.MatchString(id) {
			return out, errors.New("provide a valid linked resource ID")
		}
		path += "?resourceKind=" + url.QueryEscape(kind) + "&resourceId=" + url.QueryEscape(id)
	}
	err := c.scopedRequest(ctx, "GET", path, nil, 512<<10, &out)
	return out, err
}
func (c *Client) SessionReplay(ctx context.Context, id string) (v1.SessionReplay, error) {
	var out v1.SessionReplay
	p, e := replayPath(id)
	if e == nil {
		e = c.scopedRequest(ctx, "GET", p, nil, 64<<10, &out)
	}
	if e == nil && out.ID != id {
		e = errors.New("unexpected replay identity")
	}
	return out, e
}
func (c *Client) ReplayEvents(ctx context.Context, id string) (v1.ReplayEvents, error) {
	var out v1.ReplayEvents
	p, e := replayPath(id)
	if e == nil {
		e = c.scopedRequest(ctx, "GET", p+"/events", nil, 3<<20, &out)
	}
	if e == nil && out.ID != id {
		e = errors.New("unexpected replay identity")
	}
	return out, e
}
func (c *Client) CreateSessionReplay(ctx context.Context, in v1.CreateSessionReplay) (v1.SessionReplay, error) {
	var out v1.SessionReplay
	b, e := json.Marshal(in)
	if e == nil && len(b) > 3<<20 {
		return out, errors.New("recording exceeds request bounds")
	}
	if e == nil {
		e = c.scopedRequest(ctx, "POST", "/api/session-replays", b, 64<<10, &out)
	}
	return out, e
}
