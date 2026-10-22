package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func (c *Client) ReviewCapabilities(ctx context.Context) (v1.ReviewCapabilities, error) {
	var out v1.ReviewCapabilities
	err := c.scopedRequest(ctx, "GET", "/api/investigations/capabilities", nil, 64<<10, &out)
	return out, err
}

// WatchInvestigationRun follows one bounded SSE connection. false requests a
// reconnect using the last delivered run version; true indicates terminal state.
func (c *Client) WatchInvestigationRun(ctx context.Context, review, id string, after int64, receive func(v1.InvestigationRun) error) (bool, error) {
	path, err := runPath(review, id)
	if err != nil {
		return false, err
	}
	if id == "" || after < 0 || receive == nil {
		return false, errors.New("provide a run ID, valid version and receiver")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", c.origin+path+"/events", nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("X-Nines-Tenant", c.tenant)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Last-Event-ID", strconv.FormatInt(after, 10))
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, errors.New("review stream request failed")
	}
	defer func() {
		_ = res.Body.Close()
	}()
	if res.StatusCode != 200 {
		return false, &APIError{res.StatusCode}
	}
	if strings.Split(res.Header.Get("Content-Type"), ";")[0] != "text/event-stream" {
		return false, errors.New("invalid review stream type")
	}
	scanner := bufio.NewScanner(io.LimitReader(res.Body, (8<<20)+1))
	scanner.Buffer(make([]byte, 4096), 256<<10)
	event := ""
	data := ""
	total := 0
	for scanner.Scan() {
		line := scanner.Text()
		total += len(line) + 1
		if total > 8<<20 {
			return false, errors.New("review stream exceeds bounds")
		}
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(line[6:])
		} else if strings.HasPrefix(line, "data:") {
			data += strings.TrimSpace(line[5:]) + "\n"
		} else if line == "" {
			switch event {
			case "run":
				var run v1.InvestigationRun
				if json.Unmarshal([]byte(data), &run) != nil || run.APIVersion != v1.APIVersion || run.Tenant != c.tenant || run.ID != id || run.InvestigationID != review || run.Version < 1 {
					return false, errors.New("invalid scoped review event")
				}
				if run.Version > after {
					if err = receive(run); err != nil {
						return false, err
					}
					after = run.Version
				}
			case "done":
				var value struct{ Reason string }
				if json.Unmarshal([]byte(data), &value) != nil {
					return false, errors.New("invalid stream completion")
				}
				return value.Reason == "terminal", nil
			case "access_revoked":
				return false, &APIError{403}
			case "unavailable":
				return false, errors.New("review progress unavailable")
			}
			event = ""
			data = ""
		}
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	return false, errors.New("review stream ended before completion")
}
