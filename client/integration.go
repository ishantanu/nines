package client

import (
	"context"
	"errors"

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
