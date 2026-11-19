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
	"strconv"
	"strings"
	"time"
)

type GitHub struct {
	settings Settings
	http     *http.Client
}

func NewGitHub(s *Settings, transport http.RoundTripper) *GitHub {
	return &GitHub{settings: *s, http: &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (g *GitHub) Info() Info { return g.settings.Info() }

// do sends one bounded request. The error text is generic on purpose: provider
// bodies can echo repository detail.
func (g *GitHub) do(ctx context.Context, method, path string, in, out any, want ...int) (int, error) {
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
		return 0, errors.New("invalid GitHub request")
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+g.settings.Token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return send(g.http, req, "GitHub", out, want)
}

// send executes a request and decodes a bounded JSON response. Writes that
// might have succeeded report ErrUncertain.
func send(c *http.Client, req *http.Request, name string, out any, want []int) (int, error) {
	method := req.Method
	resp, err := c.Do(req)
	if err != nil {
		if method != http.MethodGet {
			return 0, ErrUncertain
		}
		return 0, fmt.Errorf("%s read unavailable", name)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 {
		if method != http.MethodGet {
			return resp.StatusCode, ErrUncertain
		}
		return resp.StatusCode, fmt.Errorf("invalid %s response", name)
	}
	ok := len(want) == 0 && resp.StatusCode == http.StatusOK
	for _, w := range want {
		ok = ok || resp.StatusCode == w
	}
	if !ok {
		if method != http.MethodGet && resp.StatusCode >= 500 {
			return resp.StatusCode, ErrUncertain
		}
		return resp.StatusCode, fmt.Errorf("%s API returned HTTP %d", name, resp.StatusCode)
	}
	if out != nil && len(raw) > 0 && json.Unmarshal(raw, out) != nil {
		if method != http.MethodGet {
			return resp.StatusCode, ErrUncertain
		}
		return resp.StatusCode, fmt.Errorf("invalid %s response", name)
	}
	return resp.StatusCode, nil
}

func (g *GitHub) repo() string { return "/repos/" + g.settings.Repository }

type ghPull struct {
	Number    int       `json:"number"`
	HTMLURL   string    `json:"html_url"`
	State     string    `json:"state"`
	MergedAt  *string   `json:"merged_at"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	Head      struct {
		Ref string `json:"ref"`
	} `json:"head"`
}

func convert(number int, link, state, title, body, branch string, created time.Time) PullRequest {
	out := PullRequest{Number: number, URL: link, Branch: branch, State: state, Title: title, CreatedAt: created}
	if m := markerDigest.FindStringSubmatch(body); m != nil {
		out.DefinitionDigest = m[1]
	}
	if m := markerRevision.FindStringSubmatch(body); m != nil {
		out.ExpectedRevision, _ = strconv.ParseInt(m[1], 10, 64)
	}
	return out
}

func (p ghPull) convert() PullRequest {
	state := p.State
	if p.State == "closed" && p.MergedAt != nil {
		state = "merged"
	}
	return convert(p.Number, p.HTMLURL, state, p.Title, p.Body, p.Head.Ref, p.CreatedAt)
}

func owner(repository string) string { return strings.SplitN(repository, "/", 2)[0] }

// existing returns the pull request for a branch, if GitHub has one.
func (g *GitHub) existing(ctx context.Context, branch string) (*PullRequest, error) {
	var pulls []ghPull
	q := url.Values{"head": {owner(g.settings.Repository) + ":" + branch}, "state": {"all"}, "per_page": {"5"}}
	if _, err := g.do(ctx, http.MethodGet, g.repo()+"/pulls?"+q.Encode(), nil, &pulls); err != nil {
		return nil, err
	}
	for _, p := range pulls {
		if p.Head.Ref == branch {
			c := p.convert()
			return &c, nil
		}
	}
	return nil, nil
}

// Open creates the branch, file and pull request for a proposal. It is
// idempotent for an identical proposal: the branch name derives from the
// definition digest, so a retry returns the existing pull request.
func (g *GitHub) Open(ctx context.Context, p Proposal) (Receipt, error) {
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
	if pr, err := g.existing(ctx, branch); err != nil {
		return receipt, err
	} else if pr != nil {
		receipt.PullRequest = *pr
		return receipt, nil
	}
	var base struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if _, err := g.do(ctx, http.MethodGet, g.repo()+"/git/ref/heads/"+g.settings.BaseBranch, nil, &base); err != nil || base.Object.SHA == "" {
		if err == nil {
			err = errors.New("invalid GitHub response")
		}
		return receipt, err
	}
	ref := "refs/heads/" + branch
	if _, err := g.do(ctx, http.MethodPost, g.repo()+"/git/refs", map[string]string{"ref": ref, "sha": base.Object.SHA}, nil, http.StatusCreated, http.StatusUnprocessableEntity); err != nil {
		return receipt, err
	}
	// The branch exists from here on. Undo it if the proposal cannot complete,
	// so a failure never leaves an orphan branch behind.
	cleanup := func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_, _ = g.do(c, http.MethodDelete, g.repo()+"/git/"+ref, nil, nil, http.StatusNoContent)
	}
	var current struct {
		SHA string `json:"sha"`
	}
	status, err := g.do(ctx, http.MethodGet, g.repo()+"/contents/"+path+"?ref="+url.QueryEscape(branch), nil, &current, http.StatusOK, http.StatusNotFound)
	if err != nil {
		cleanup()
		return receipt, err
	}
	put := map[string]any{
		"message": fmt.Sprintf("Propose %s %s (%s)\n\nDefinition digest: %s\nExpected revision: %d", noun(kind), p.ID, p.Action, p.Digest, p.ExpectedRevision),
		"content": base64.StdEncoding.EncodeToString(p.Manifest),
		"branch":  branch,
	}
	if status == http.StatusOK && current.SHA != "" {
		put["sha"] = current.SHA
	}
	if _, err = g.do(ctx, http.MethodPut, g.repo()+"/contents/"+path, put, nil, http.StatusOK, http.StatusCreated); err != nil {
		if !errors.Is(err, ErrUncertain) {
			cleanup()
		}
		return receipt, err
	}
	var pull ghPull
	title := fmt.Sprintf("%s %s: %s %s", strings.ToUpper(noun(kind)[:1])+noun(kind)[1:], p.Action, p.ID, p.Digest[:8])
	if _, err = g.do(ctx, http.MethodPost, g.repo()+"/pulls", map[string]string{"title": title, "head": branch, "base": g.settings.BaseBranch, "body": prBody(p, kind, path, g.settings.BaseBranch, noun(kind))}, &pull, http.StatusCreated); err != nil {
		if !errors.Is(err, ErrUncertain) {
			cleanup()
		}
		return receipt, err
	}
	receipt.Created = true
	receipt.PullRequest = pull.convert()
	return receipt, nil
}

// Proposals lists recent pull requests that Nines opened for one definition in
// one tenant. It reads GitHub only; nothing is stored in Nines.
func (g *GitHub) Proposals(ctx context.Context, tenant, kind, id string) ([]PullRequest, error) {
	prefix, err := branchPrefix(tenant, kind, id)
	if err != nil {
		return nil, err
	}
	var pulls []ghPull
	q := url.Values{"state": {"all"}, "sort": {"created"}, "direction": {"desc"}, "per_page": {"100"}}
	if _, err := g.do(ctx, http.MethodGet, g.repo()+"/pulls?"+q.Encode(), nil, &pulls); err != nil {
		return nil, err
	}
	out := []PullRequest{}
	for _, p := range pulls {
		if strings.HasPrefix(p.Head.Ref, prefix) {
			out = append(out, p.convert())
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > 10 {
		out = out[:10]
	}
	return out, nil
}

// File reads the definition file from the base branch. found is false when the
// repository has no such file.
func (g *GitHub) File(ctx context.Context, kind, id string) (content []byte, path string, found bool, err error) {
	kind, err = kindOrDefault(kind)
	if err != nil || !safeName.MatchString(id) {
		return nil, "", false, errors.New("identifier is not safe to use in a path")
	}
	path = filePath(g.settings.Path, kind, id)
	var file struct {
		Type     string `json:"type"`
		Size     int    `json:"size"`
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}
	status, err := g.do(ctx, http.MethodGet, g.repo()+"/contents/"+path+"?ref="+url.QueryEscape(g.settings.BaseBranch), nil, &file, http.StatusOK, http.StatusNotFound)
	if err != nil {
		return nil, path, false, err
	}
	if status == http.StatusNotFound {
		return nil, path, false, nil
	}
	if file.Type != "file" || file.Encoding != "base64" || file.Size > 1<<20 {
		return nil, path, true, errors.New("the repository file is not a readable definition")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(file.Content, "\n", ""))
	if err != nil || len(raw) > 1<<20 {
		return nil, path, true, errors.New("the repository file is not a readable definition")
	}
	return raw, path, true, nil
}
