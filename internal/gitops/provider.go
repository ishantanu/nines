package gitops

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

// ErrConflict means the repository changed in a way the reviewed plan did not
// anticipate.
var ErrConflict = errors.New("the repository changed; prepare and review a fresh proposal")

// ErrUncertain means a write may have reached the provider. Retrying the
// identical proposal is safe: it finds and returns the existing branch or
// request.
var ErrUncertain = errors.New("the Git provider may have completed the proposal; retry the identical reviewed proposal to inspect it")

// Kinds of definition that can be proposed.
const (
	KindSLO     = "slo"
	KindJourney = "journey"
)

// PullRequest and Receipt are the public wire types, shared with the HTTP API
// and the Go client. A GitLab merge request is reported as a pull request.
type (
	PullRequest = v1.GitPullRequest
	Receipt     = v1.GitReceipt
)

// Proposal is one reviewed definition to propose. Digest and ExpectedRevision
// come from the server's own plan, never from the caller.
type Proposal struct {
	Tenant           string
	Kind             string // KindSLO (default) or KindJourney
	ID               string
	Action           string
	ExpectedRevision int64
	Digest           string
	Manifest         []byte
	Warnings         []string
}

// Provider is the Git hosting adapter. Implementations only write a branch, a
// file and a pull or merge request: merging and deploying stay with the
// repository's own review and pipeline.
type Provider interface {
	Info() Info
	Open(ctx context.Context, p Proposal) (Receipt, error)
	Proposals(ctx context.Context, tenant, kind, id string) ([]PullRequest, error)
	File(ctx context.Context, kind, id string) (content []byte, path string, found bool, err error)
}

// New builds the provider for validated settings.
func New(s *Settings, transport http.RoundTripper) (Provider, error) {
	switch s.Provider {
	case "github":
		return NewGitHub(s, transport), nil
	case "gitlab":
		return NewGitLab(s, transport), nil
	}
	return nil, errors.New("unsupported Git provider")
}

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func kindOrDefault(k string) (string, error) {
	switch k {
	case "", KindSLO:
		return KindSLO, nil
	case KindJourney:
		return KindJourney, nil
	}
	return "", errors.New("unsupported definition kind")
}

// branchPrefix scopes proposals to one tenant, kind and definition. Journeys use
// a nested "journeys/" segment, which a safe name can never contain, so an SLO
// and a journey with the same identifier cannot be confused.
func branchPrefix(tenant, kind, id string) (string, error) {
	kind, err := kindOrDefault(kind)
	if err != nil {
		return "", err
	}
	if !safeName.MatchString(tenant) || !safeName.MatchString(id) {
		return "", errors.New("tenant or identifier is not safe to use in a branch name")
	}
	if kind == KindJourney {
		return "nines/" + tenant + "/journeys/" + id + "-", nil
	}
	return "nines/" + tenant + "/" + id + "-", nil
}

// Branch names the proposal branch for a definition digest.
func Branch(tenant, kind, id, digest string) (string, error) {
	prefix, err := branchPrefix(tenant, kind, id)
	if err != nil || !digestPattern.MatchString(digest) {
		return "", errors.New("tenant, identifier or digest is not safe to use in a branch name")
	}
	return prefix + digest[:12], nil
}

func filePath(dir, kind, id string) string {
	if kind == KindJourney {
		return dir + "/" + id + ".journey.yaml"
	}
	return dir + "/" + id + ".slo.yaml"
}

func prBody(p Proposal, kind, path, base, noun string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Nines proposes the reviewed `%s` definition for %s `%s` (%s).\n\n", p.Action, noun, p.ID, p.Tenant)
	fmt.Fprintf(&b, "- File: `%s`\n- Base branch: `%s`\n- Expected live revision: %d\n- Definition digest: `%s`\n\n", path, base, p.ExpectedRevision, p.Digest)
	if len(p.Warnings) > 0 {
		b.WriteString("Plan warnings:\n\n")
		for _, w := range p.Warnings {
			fmt.Fprintf(&b, "- %s\n", strings.ReplaceAll(strings.ReplaceAll(w, "\r", " "), "\n", " "))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Merging this request does **not** apply the %s in Nines. Your pipeline (the Kubernetes operator, `nines apply` in CI or another deploy step) must deploy it, and Nines will report the definition as applied only once the live definition matches this digest.\n\n", noun)
	b.WriteString("<!-- nines -->\n")
	fmt.Fprintf(&b, "nines-tenant: %s\nnines-kind: %s\nnines-id: %s\nnines-definition-digest: %s\nnines-expected-revision: %d\n", p.Tenant, kind, p.ID, p.Digest, p.ExpectedRevision)
	return b.String()
}

var (
	markerDigest   = regexp.MustCompile(`(?m)^nines-definition-digest: ([a-f0-9]{64})$`)
	markerRevision = regexp.MustCompile(`(?m)^nines-expected-revision: ([0-9]{1,18})$`)
)

func noun(kind string) string {
	if kind == KindJourney {
		return "journey"
	}
	return "SLO"
}
