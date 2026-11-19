package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

// The Git delivery wire types are shared with the server.
type (
	GitInfo           = v1.GitInfo
	GitPullRequest    = v1.GitPullRequest
	GitProposal       = v1.GitProposalResult
	GitProposalStatus = v1.GitProposalStatus
	GitProposals      = v1.GitProposalList
	GitDrift          = v1.GitDrift
)

var planDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (c *Client) GitInfo(ctx context.Context) (GitInfo, error) {
	var out GitInfo
	err := c.contextRead(ctx, "/api/gitops", &out)
	return out, err
}

func gitResource(kind string) (string, error) {
	switch kind {
	case "", "slo":
		return "slos", nil
	case "journey":
		return "journeys", nil
	}
	return "", errors.New("kind must be slo or journey")
}

// GitProposals lists proposals for an SLO.
func (c *Client) GitProposals(ctx context.Context, id string) (GitProposals, error) {
	return c.GitProposalsFor(ctx, "slo", id)
}

// GitProposalsFor lists proposals for an SLO ("slo") or a journey ("journey").
func (c *Client) GitProposalsFor(ctx context.Context, kind, id string) (GitProposals, error) {
	var out GitProposals
	resource, err := gitResource(kind)
	if err != nil || !identifier.MatchString(id) {
		return out, errors.New("invalid kind or identifier")
	}
	err = c.contextRead(ctx, "/api/"+resource+"/"+id+"/gitops/proposals", &out)
	return out, err
}

// GitDriftFor compares the repository file with the live definition by digest.
func (c *Client) GitDriftFor(ctx context.Context, kind, id string) (GitDrift, error) {
	var out GitDrift
	resource, err := gitResource(kind)
	if err != nil || !identifier.MatchString(id) {
		return out, errors.New("invalid kind or identifier")
	}
	err = c.contextRead(ctx, "/api/"+resource+"/"+id+"/gitops/drift", &out)
	return out, err
}

// ProposeYAML opens a pull request for a reviewed SLO definition. expected and
// digest come from the plan the caller reviewed; the server refuses a mismatch.
func (c *Client) ProposeYAML(ctx context.Context, body []byte, expected int64, digest string) (GitProposal, error) {
	var out GitProposal
	if len(body) == 0 || len(body) > 1<<20 || expected < 0 || !planDigest.MatchString(digest) {
		return out, errors.New("provide one SLO YAML document, a nonnegative expected revision and the plan digest")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.origin+"/api/gitops/proposals", bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/yaml")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Nines-Tenant", c.tenant)
	req.Header.Set("If-Match", fmt.Sprintf("\"%d\"", expected))
	req.Header.Set("X-Nines-Plan-Digest", digest)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		return out, errors.New("nines request failed")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		return out, &APIError{res.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 || json.Unmarshal(data, &out) != nil {
		return GitProposal{}, errors.New("invalid Nines response")
	}
	return out, nil
}
