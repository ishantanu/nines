package client

import (
	"context"
	"errors"
	"regexp"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

var skillVersion = regexp.MustCompile(`^[0-9]{1,6}\.[0-9]{1,6}\.[0-9]{1,6}$`)

func (c *Client) ReviewSkills(ctx context.Context) (v1.ReviewSkillList, error) {
	var out v1.ReviewSkillList
	err := c.scopedRequest(ctx, "GET", "/api/review-skills", nil, 256<<10, &out)
	return out, err
}
func (c *Client) ReviewSkill(ctx context.Context, id, version string) (v1.ReviewSkillDocument, error) {
	var out v1.ReviewSkillDocument
	if !identifier.MatchString(id) || !skillVersion.MatchString(version) {
		return out, errors.New("provide a skill ID and exact semantic version")
	}
	err := c.scopedRequest(ctx, "GET", "/api/review-skills/"+id+"/versions/"+version, nil, 128<<10, &out)
	if err == nil && (out.Item.Metadata.Name != id || out.Item.Metadata.Version != version) {
		return out, errors.New("unexpected skill identity")
	}
	return out, err
}
