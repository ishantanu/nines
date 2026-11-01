// Package githubreview publishes explicit, immutable review snapshots to GitHub.
// GitHub comments never grant Nines approval or application authority.
package githubreview

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

var identifier = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
var repoPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
var hash = regexp.MustCompile(`^[a-f0-9]{64}$`)
var commit = regexp.MustCompile(`^([a-f0-9]{40}|[a-f0-9]{64})$`)

type Target struct {
	Repository  string `json:"repository"`
	PullRequest int    `json:"pullRequest"`
	Head        string `json:"head"`
}
type Plan struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Target
	Tenant          string `json:"tenant"`
	Conversation    string `json:"conversation"`
	Version         int64  `json:"version"`
	ChangeSetDigest string `json:"changeSetDigest"`
	ReportDigest    string `json:"reportDigest"`
	Markdown        string `json:"markdown"`
}

func (t Target) Validate() error {
	parts := strings.Split(t.Repository, "/")
	if len(parts) != 2 || len(parts[0]) > 39 || len(parts[1]) > 100 || t.PullRequest < 1 || !commit.MatchString(t.Head) {
		return errors.New("provide owner/repository, a positive pull request number and a full lowercase commit SHA")
	}
	for _, p := range parts {
		if p == "." || p == ".." || !repoPart.MatchString(p) {
			return errors.New("invalid GitHub repository")
		}
	}
	return nil
}
func marker(p Plan) string {
	b, _ := json.Marshal(struct {
		Target                       Target
		Tenant, Conversation, Digest string
		Version                      int64
	}{p.Target, p.Tenant, p.Conversation, p.ChangeSetDigest, p.Version})
	sum := sha256.Sum256(b)
	return "<!-- nines-review:v1:" + hex.EncodeToString(sum[:]) + " -->"
}
func digest(p Plan) string {
	p.ReportDigest = ""
	b, _ := json.Marshal(p)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func (p Plan) Validate() error {
	if err := p.Target.Validate(); err != nil {
		return err
	}
	if p.APIVersion != v1.APIVersion || p.Kind != "GitHubReviewPlan" || !identifier.MatchString(p.Tenant) || !identifier.MatchString(p.Conversation) || p.Version < 1 || !hash.MatchString(p.ChangeSetDigest) || !hash.MatchString(p.ReportDigest) || digest(p) != p.ReportDigest || !strings.HasPrefix(p.Markdown, marker(p)+"\n") || len(p.Markdown) > 60000 {
		return errors.New("invalid or modified GitHub review plan")
	}
	return nil
}

// Prepare excludes the export timestamp so retries of the same revision are stable.
// Every user/model-provided field is inside a JSON fence of sufficient length.
func Prepare(a v1.IntentReviewArtifact, target Target) (Plan, error) {
	p := Plan{APIVersion: v1.APIVersion, Kind: "GitHubReviewPlan", Target: target, Tenant: a.Tenant, Conversation: a.Conversation, Version: a.Version, ChangeSetDigest: a.ChangeSet.Digest}
	if err := target.Validate(); err != nil {
		return Plan{}, err
	}
	if a.APIVersion != v1.APIVersion || a.AuditIntegrity != "verified" || a.Version < 1 || a.Status.Version != a.Version || a.Status.Tenant != a.Tenant || a.Status.Conversation != a.Conversation || a.Status.ChangeSetDigest != a.ChangeSet.Digest || !hash.MatchString(a.ChangeSet.Digest) || len(a.ChangeSet.Changes) == 0 || a.ChangeSet.Recommendation == nil || strings.TrimSpace(a.ChangeSet.Recommendation.Summary) == "" || a.AuditThrough < 1 || len(a.Audit) == 0 || a.Audit[len(a.Audit)-1].Sequence != a.AuditThrough {
		return Plan{}, errors.New("review requires a consistent, prepared and audit-verified artifact")
	}
	var body bytes.Buffer
	fmt.Fprintf(&body, "%s\n## Nines SLO change review\n\nNines snapshot associated with PR commit `%s`. Conversation version `%d`.\n\nThis comment is review evidence. It does not verify that PR files implement the proposed definitions. Recheck the current Nines version and digest before approval or application. GitHub review or merge does not approve or apply an SLO.\n\n", marker(p), target.Head, a.Version)
	block := func(title string, value any) error {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		fence := "```"
		for bytes.Contains(data, []byte(fence)) {
			fence += "`"
		}
		fmt.Fprintf(&body, "### %s\n\n%sjson\n%s\n%s\n\n", title, fence, data, fence)
		return nil
	}
	if err := block("Workflow and review state", a.Status); err != nil {
		return Plan{}, err
	}
	if err := block("Recommendation and evidence", a.ChangeSet.Recommendation); err != nil {
		return Plan{}, err
	}
	if err := block("Exact proposed changes", a.ChangeSet.Changes); err != nil {
		return Plan{}, err
	}
	// Retain all remaining governance fields, including rollback, safety and outcome.
	raw, err := json.Marshal(a.ChangeSet)
	if err != nil {
		return Plan{}, err
	}
	var governance map[string]any
	if err = json.Unmarshal(raw, &governance); err != nil {
		return Plan{}, err
	}
	delete(governance, "recommendation")
	delete(governance, "changes")
	if err = block("Approval, safety and rollback", governance); err != nil {
		return Plan{}, err
	}
	if err = block("Verified audit", a.Audit); err != nil {
		return Plan{}, err
	}
	if err = block("Limitations", a.Limitations); err != nil {
		return Plan{}, err
	}
	p.Markdown = body.String()
	p.ReportDigest = digest(p)
	if err = p.Validate(); err != nil {
		return Plan{}, errors.New("review cannot be published: verify identifiers or use an access-controlled artifact for reports over 60000 bytes")
	}
	return p, nil
}

// ContainsSecret rejects both literal and JSON-escaped copies of a credential.
func ContainsSecret(body, secret string) bool {
	if secret == "" {
		return false
	}
	if strings.Contains(body, secret) {
		return true
	}
	encoded, _ := json.Marshal(secret)
	return len(encoded) > 2 && strings.Contains(body, string(encoded[1:len(encoded)-1]))
}
