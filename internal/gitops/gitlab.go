package gitops

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// GitLab implements Provider with the GitLab REST v4 API. A merge request is
// reported as a pull request. It never merges.
type GitLab struct {
	settings Settings
	http     *http.Client
}

func NewGitLab(s *Settings, transport http.RoundTripper) *GitLab {
	return &GitLab{settings: *s, http: &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (g *GitLab) Info() Info { return g.settings.Info() }

func (g *GitLab) project() string { return "/projects/" + url.PathEscape(g.settings.Repository) }

func (g *GitLab) do(ctx context.Context, method, path string, in, out any, want ...int) (int, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.settings.APIURL+path, body)
	if err != nil {
		return 0, errors.New("invalid GitLab request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("PRIVATE-TOKEN", g.settings.Token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return send(g.http, req, "GitLab", out, want)
}

type glMR struct {
	IID          int       `json:"iid"`
	WebURL       string    `json:"web_url"`
	State        string    `json:"state"` // opened, closed, merged or locked
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	CreatedAt    time.Time `json:"created_at"`
	SourceBranch string    `json:"source_branch"`
}

func (m glMR) convert() PullRequest {
	state := "closed"
	switch m.State {
	case "opened", "locked":
		state = "open"
	case "merged":
		state = "merged"
	}
	return convert(m.IID, m.WebURL, state, m.Title, m.Description, m.SourceBranch, m.CreatedAt)
}

func (g *GitLab) existing(ctx context.Context, branch string) (*PullRequest, error) {
	var mrs []glMR
	q := url.Values{"source_branch": {branch}, "state": {"all"}, "per_page": {"5"}}
	if _, err := g.do(ctx, http.MethodGet, g.project()+"/merge_requests?"+q.Encode(), nil, &mrs); err != nil {
		return nil, err
	}
	for _, m := range mrs {
		if m.SourceBranch == branch {
			c := m.convert()
			return &c, nil
		}
	}
	return nil, nil
}

func (g *GitLab) Open(ctx context.Context, p Proposal) (Receipt, error) {
	kind, err := kindOrDefault(p.Kind)
	if err != nil {
		return Receipt{}, err
	}
	branch, err := Branch(p.Tenant, kind, p.ID, p.Digest)
	if err != nil {
		return Receipt{}, err
	}
	if len(p.Manifest) == 0 || len(p.Manifest) > 1<<20 {
		return Receipt{}, errors.New("the definition is empty or too large to propose")
	}
	path := filePath(g.settings.Path, kind, p.ID)
	receipt := Receipt{APIVersion: "nines.dev/v1alpha1", Path: path}
	if mr, err := g.existing(ctx, branch); err != nil {
		return receipt, err
	} else if mr != nil {
		receipt.PullRequest = *mr
		return receipt, nil
	}
	// The base branch must exist; this also fails fast on a wrong project.
	if _, err := g.do(ctx, http.MethodGet, g.project()+"/repository/branches/"+url.PathEscape(g.settings.BaseBranch), nil, &struct{}{}); err != nil {
		return receipt, err
	}
	q := url.Values{"branch": {branch}, "ref": {g.settings.BaseBranch}}
	if _, err := g.do(ctx, http.MethodPost, g.project()+"/repository/branches?"+q.Encode(), nil, nil, http.StatusCreated, http.StatusBadRequest); err != nil {
		return receipt, err
	}
	cleanup := func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_, _ = g.do(c, http.MethodDelete, g.project()+"/repository/branches/"+url.PathEscape(branch), nil, nil, http.StatusNoContent)
	}
	action := "create"
	status, err := g.do(ctx, http.MethodGet, g.project()+"/repository/files/"+url.PathEscape(path)+"?ref="+url.QueryEscape(branch), nil, &struct{}{}, http.StatusOK, http.StatusNotFound)
	if err != nil {
		cleanup()
		return receipt, err
	}
	if status == http.StatusOK {
		action = "update"
	}
	commit := map[string]any{
		"branch":         branch,
		"commit_message": fmt.Sprintf("Propose %s %s (%s)\n\nDefinition digest: %s\nExpected revision: %d", noun(kind), p.ID, p.Action, p.Digest, p.ExpectedRevision),
		"actions":        []map[string]string{{"action": action, "file_path": path, "content": base64.StdEncoding.EncodeToString(p.Manifest), "encoding": "base64"}},
	}
	if _, err = g.do(ctx, http.MethodPost, g.project()+"/repository/commits", commit, nil, http.StatusCreated); err != nil {
		if !errors.Is(err, ErrUncertain) {
			cleanup()
		}
		return receipt, err
	}
	var mr glMR
	title := fmt.Sprintf("%s %s: %s %s", strings.ToUpper(noun(kind)[:1])+noun(kind)[1:], p.Action, p.ID, p.Digest[:8])
	if _, err = g.do(ctx, http.MethodPost, g.project()+"/merge_requests", map[string]any{"source_branch": branch, "target_branch": g.settings.BaseBranch, "title": title, "description": prBody(p, kind, path, g.settings.BaseBranch, noun(kind)), "remove_source_branch": false}, &mr, http.StatusCreated); err != nil {
		if !errors.Is(err, ErrUncertain) {
			cleanup()
		}
		return receipt, err
	}
	receipt.Created = true
	receipt.PullRequest = mr.convert()
	return receipt, nil
}

func (g *GitLab) Proposals(ctx context.Context, tenant, kind, id string) ([]PullRequest, error) {
	prefix, err := branchPrefix(tenant, kind, id)
	if err != nil {
		return nil, err
	}
	var mrs []glMR
	q := url.Values{"state": {"all"}, "order_by": {"created_at"}, "sort": {"desc"}, "per_page": {"100"}}
	if _, err := g.do(ctx, http.MethodGet, g.project()+"/merge_requests?"+q.Encode(), nil, &mrs); err != nil {
		return nil, err
	}
	out := []PullRequest{}
	for _, m := range mrs {
		if strings.HasPrefix(m.SourceBranch, prefix) {
			out = append(out, m.convert())
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > 10 {
		out = out[:10]
	}
	return out, nil
}

func (g *GitLab) File(ctx context.Context, kind, id string) (content []byte, path string, found bool, err error) {
	kind, err = kindOrDefault(kind)
	if err != nil || !safeName.MatchString(id) {
		return nil, "", false, errors.New("identifier is not safe to use in a path")
	}
	path = filePath(g.settings.Path, kind, id)
	var file struct {
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
		Size     int    `json:"size"`
	}
	status, err := g.do(ctx, http.MethodGet, g.project()+"/repository/files/"+url.PathEscape(path)+"?ref="+url.QueryEscape(g.settings.BaseBranch), nil, &file, http.StatusOK, http.StatusNotFound)
	if err != nil {
		return nil, path, false, err
	}
	if status == http.StatusNotFound {
		return nil, path, false, nil
	}
	if file.Encoding != "base64" || file.Size > 1<<20 {
		return nil, path, true, errors.New("the repository file is not a readable definition")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(file.Content, "\n", ""))
	if err != nil || len(raw) > 1<<20 {
		return nil, path, true, errors.New("the repository file is not a readable definition")
	}
	return raw, path, true, nil
}
