package client

import (
	"context"
	"encoding/json"
	"errors"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

// ConvertMigration converts Sloth or Pyrra definitions on the server. It saves
// nothing: review each converted definition, then plan and apply it.
func (c *Client) ConvertMigration(ctx context.Context, req v1.MigrationRequest) (v1.MigrationResult, error) {
	var out v1.MigrationResult
	if req.Tool != "sloth" && req.Tool != "pyrra" {
		return out, errors.New("tool must be sloth or pyrra")
	}
	if req.Content == "" || len(req.Content) > 1<<20 {
		return out, errors.New("provide between 1 byte and 1 MiB of YAML")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return out, err
	}
	err = c.scopedRequest(ctx, "POST", "/api/migrations/convert", body, 8<<20, &out)
	return out, err
}
