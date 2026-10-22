package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"regexp"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type reviewSkillInput struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

func addReviewSkills(s *mcp.Server, a *apiClient) {
	register(s, a, "list_review_skills", "Read server-owned executable review skill versions, digests, tool allowlists and input/output contracts. This does not execute a run.", true, func(ctx context.Context, _ emptyInput) (any, error) {
		return a.request(ctx, http.MethodGet, "/api/review-skills", nil)
	})
	register(s, a, "get_review_skill", "Read one exact executable review skill version in the configured tenant. Guidance skill resources remain separate.", true, func(ctx context.Context, in reviewSkillInput) (any, error) {
		if err := validID(in.ID); err != nil {
			return nil, err
		}
		if !regexp.MustCompile(`^[0-9]{1,6}\.[0-9]{1,6}\.[0-9]{1,6}$`).MatchString(in.Version) {
			return nil, errors.New("provide an exact semantic version")
		}
		return a.request(ctx, http.MethodGet, "/api/review-skills/"+in.ID+"/versions/"+in.Version, nil)
	})
}
