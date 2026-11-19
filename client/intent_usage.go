package client

import (
	"context"
	"errors"
	"strconv"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

// IntentUsage reads aggregate intent workflow progress for the client's tenant.
// A zero days value uses the server default window.
func (c *Client) IntentUsage(ctx context.Context, days int) (v1.IntentUsage, error) {
	var out v1.IntentUsage
	if days < 0 || days > 365 {
		return out, errors.New("days must be between 1 and 365")
	}
	path := "/api/intent-conversations/usage"
	if days > 0 {
		path += "?days=" + strconv.Itoa(days)
	}
	e := c.scopedRequest(ctx, "GET", path, nil, 64<<10, &out)
	return out, e
}
