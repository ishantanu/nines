package githubreview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var ErrConflict = errors.New("review or pull request changed; prepare and review a fresh plan")
var ErrUncertain = errors.New("GitHub publication may have succeeded; retry the identical reviewed plan to inspect for an existing comment")

type Receipt struct {
	APIVersion   string `json:"apiVersion"`
	State        string `json:"state"`
	CommentID    int64  `json:"commentId"`
	URL          string `json:"url"`
	Head         string `json:"head"`
	ReportDigest string `json:"reportDigest"`
}
type Publisher struct {
	token  string
	origin string
	http   *http.Client
}

func New(token string, transport http.RoundTripper) (*Publisher, error) {
	if token == "" || len(token) > 8192 || strings.ContainsFunc(token, unicode.IsControl) || strings.TrimSpace(token) != token {
		return nil, errors.New("set a single-line NINES_GITHUB_TOKEN")
	}
	return &Publisher{token: token, origin: "https://api.github.com", http: &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (g *Publisher) request(ctx context.Context, method, path string, in, out any) error {
	var payload []byte
	if in != nil {
		var err error
		payload, err = json.Marshal(in)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, g.origin+path, bytes.NewReader(payload))
	if err != nil {
		return errors.New("invalid GitHub request")
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := g.http.Do(req)
	if err != nil {
		if method == http.MethodPost {
			return ErrUncertain
		}
		return errors.New("GitHub read unavailable")
	}
	defer func() { _ = response.Body.Close() }()
	want := http.StatusOK
	if method == http.MethodPost {
		want = http.StatusCreated
	}
	if response.StatusCode != want {
		if method == http.MethodPost && response.StatusCode >= 500 {
			return ErrUncertain
		}
		return fmt.Errorf("GitHub API returned HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil || len(raw) > 8<<20 || json.Unmarshal(raw, out) != nil {
		if method == http.MethodPost {
			return ErrUncertain
		}
		return errors.New("invalid GitHub response")
	}
	return nil
}
func (g *Publisher) checkPull(ctx context.Context, p Plan) error {
	var pull struct {
		Number int
		State  string
		Head   struct{ SHA string }
		Base   struct {
			Repo struct {
				FullName string `json:"full_name"`
			}
		}
	}
	if err := g.request(ctx, http.MethodGet, "/repos/"+p.Repository+"/pulls/"+strconv.Itoa(p.PullRequest), nil, &pull); err != nil {
		return err
	}
	if pull.Number != p.PullRequest || pull.State != "open" || pull.Head.SHA != p.Head || !strings.EqualFold(pull.Base.Repo.FullName, p.Repository) {
		return ErrConflict
	}
	return nil
}

type comment struct {
	ID   int64
	Body string
}

func receipt(p Plan, id int64, state string) Receipt {
	return Receipt{APIVersion: p.APIVersion, State: state, CommentID: id, URL: "https://github.com/" + p.Repository + "/pull/" + strconv.Itoa(p.PullRequest) + "#issuecomment-" + strconv.FormatInt(id, 10), Head: p.Head, ReportDigest: p.ReportDigest}
}

// Publish creates an immutable comment; it never edits comments or records approval.
// fresh must recheck the Nines snapshot before the external write.
// GitHub has no atomic head precondition for comments: every report labels its SHA.
func (g *Publisher) Publish(ctx context.Context, p Plan, fresh func(context.Context) error) (Receipt, error) {
	if err := p.Validate(); err != nil {
		return Receipt{}, err
	}
	if fresh == nil {
		return Receipt{}, errors.New("a fresh Nines revision check is required")
	}
	if ContainsSecret(p.Markdown, g.token) {
		return Receipt{}, errors.New("review contains the GitHub credential")
	}
	if err := g.checkPull(ctx, p); err != nil {
		return Receipt{}, err
	}
	endpoint := "/repos/" + p.Repository + "/issues/" + strconv.Itoa(p.PullRequest) + "/comments"
	// Bound pagination; never claim absence if the comment search was incomplete.
	for page := 1; page <= 10; page++ {
		var comments []comment
		if err := g.request(ctx, http.MethodGet, endpoint+"?per_page=100&page="+strconv.Itoa(page), nil, &comments); err != nil {
			return Receipt{}, err
		}
		if comments == nil {
			return Receipt{}, errors.New("invalid GitHub comment listing")
		}
		for _, item := range comments {
			if strings.HasPrefix(item.Body, marker(p)+"\n") {
				if item.ID < 1 || item.Body != p.Markdown {
					return Receipt{}, ErrConflict
				}
				if err := fresh(ctx); err != nil {
					return Receipt{}, err
				}
				if err := g.checkPull(ctx, p); err != nil {
					return Receipt{}, err
				}
				return receipt(p, item.ID, "already_published"), nil
			}
		}
		if len(comments) < 100 {
			break
		}
		if page == 10 {
			return Receipt{}, errors.New("GitHub comment history exceeds the bounded search; publication stopped")
		}
	}
	if err := fresh(ctx); err != nil {
		return Receipt{}, err
	}
	if err := g.checkPull(ctx, p); err != nil {
		return Receipt{}, err
	}
	var created comment
	if err := g.request(ctx, http.MethodPost, endpoint, map[string]string{"body": p.Markdown}, &created); err != nil {
		return Receipt{}, err
	}
	if created.ID < 1 || created.Body != p.Markdown {
		return Receipt{}, ErrUncertain
	}
	return receipt(p, created.ID, "published"), nil
}
