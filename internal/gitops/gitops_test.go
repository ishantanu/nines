package gitops

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const digest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// fakeGitHub implements only the endpoints the client uses, in memory.
type fakeGitHub struct {
	t         *testing.T
	refs      map[string]string
	files     map[string]string // branch:path -> content
	pulls     []map[string]any
	deleted   []string
	failPut   bool
	auth      []string
	putBodies []map[string]any
}

func newFake(t *testing.T) (*fakeGitHub, *GitHub) {
	f := &fakeGitHub{t: t, refs: map[string]string{"heads/main": "basesha"}, files: map[string]string{}}
	srv := httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	s, err := Resolve(Config{Provider: "github", Repository: "acme/slos", TokenEnv: "GH_TOKEN"}, func(string) string { return "secret-token" })
	if err != nil {
		t.Fatal(err)
	}
	s.APIURL = srv.URL
	return f, NewGitHub(s, srv.Client().Transport)
}

func (f *fakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	path := strings.TrimPrefix(r.URL.Path, "/repos/acme/slos")
	reply := func(code int, v any) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	var in map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&in)
	}
	switch {
	case r.Method == "GET" && strings.HasPrefix(path, "/git/ref/"):
		ref := strings.TrimPrefix(path, "/git/ref/")
		if sha, ok := f.refs[ref]; ok {
			reply(200, map[string]any{"object": map[string]any{"sha": sha}})
			return
		}
		reply(404, map[string]string{"message": "Not Found"})
	case r.Method == "POST" && path == "/git/refs":
		ref := strings.TrimPrefix(in["ref"].(string), "refs/")
		if _, ok := f.refs[ref]; ok {
			reply(422, map[string]string{"message": "Reference already exists"})
			return
		}
		f.refs[ref] = in["sha"].(string)
		reply(201, map[string]any{})
	case r.Method == "DELETE" && strings.HasPrefix(path, "/git/refs/"):
		ref := strings.TrimPrefix(path, "/git/refs/")
		delete(f.refs, ref)
		f.deleted = append(f.deleted, ref)
		w.WriteHeader(204)
	case r.Method == "GET" && strings.HasPrefix(path, "/contents/"):
		key := r.URL.Query().Get("ref") + ":" + strings.TrimPrefix(path, "/contents/")
		if content, ok := f.files[key]; ok {
			reply(200, map[string]any{"sha": "filesha", "type": "file", "size": len(content), "encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(content))})
			return
		}
		reply(404, map[string]string{"message": "Not Found"})
	case r.Method == "PUT" && strings.HasPrefix(path, "/contents/"):
		if f.failPut {
			reply(403, map[string]string{"message": "secret repository detail"})
			return
		}
		f.putBodies = append(f.putBodies, in)
		raw, _ := base64.StdEncoding.DecodeString(in["content"].(string))
		f.files[in["branch"].(string)+":"+strings.TrimPrefix(path, "/contents/")] = string(raw)
		reply(201, map[string]any{})
	case r.Method == "POST" && path == "/pulls":
		pr := map[string]any{"number": len(f.pulls) + 1, "html_url": "https://github.example/acme/slos/pull/" + string(rune('0'+len(f.pulls)+1)), "state": "open", "merged_at": nil, "title": in["title"], "body": in["body"], "created_at": time.Now().UTC().Format(time.RFC3339), "head": map[string]any{"ref": in["head"]}}
		f.pulls = append(f.pulls, pr)
		reply(201, pr)
	case r.Method == "GET" && path == "/pulls":
		head := r.URL.Query().Get("head")
		out := []map[string]any{}
		for _, p := range f.pulls {
			if head == "" || "acme:"+p["head"].(map[string]any)["ref"].(string) == head {
				out = append(out, p)
			}
		}
		reply(200, out)
	default:
		reply(404, map[string]string{"message": "unexpected " + r.Method + " " + path})
	}
}

func proposal() Proposal {
	return Proposal{Tenant: "commerce", ID: "checkout", Action: "update", ExpectedRevision: 3, Digest: digest, Manifest: []byte("kind: ServiceLevelObjective\n"), Warnings: []string{"line one\nline two"}}
}

func TestOpenCreatesBranchFilePullRequestAndIsIdempotent(t *testing.T) {
	f, g := newFake(t)
	got, err := g.Open(context.Background(), proposal())
	if err != nil {
		t.Fatal(err)
	}
	branch := "nines/commerce/checkout-0123456789ab"
	if !got.Created || got.Path != "slos/checkout.slo.yaml" || got.PullRequest.Branch != branch || got.PullRequest.State != "open" {
		t.Fatalf("%+v", got)
	}
	if f.files[branch+":slos/checkout.slo.yaml"] != "kind: ServiceLevelObjective\n" {
		t.Fatal("the exact reviewed definition was not committed:", f.files)
	}
	if got.PullRequest.DefinitionDigest != digest || got.PullRequest.ExpectedRevision != 3 {
		t.Fatalf("pull request must carry the digest and expected revision: %+v", got.PullRequest)
	}
	body := f.pulls[0]["body"].(string)
	if !strings.Contains(body, "does **not** apply the SLO in Nines") || strings.Contains(body, "line one\nline two") {
		t.Fatalf("body must state that merging does not apply, and keep warnings on one line:\n%s", body)
	}
	for _, a := range f.auth {
		if a != "Bearer secret-token" {
			t.Fatal("every request is authenticated with the configured token")
		}
	}
	// A retry of the identical proposal finds the pull request instead of duplicating it.
	again, err := g.Open(context.Background(), proposal())
	if err != nil || again.Created || again.PullRequest.Number != got.PullRequest.Number || len(f.pulls) != 1 {
		t.Fatalf("%+v %v pulls=%d", again, err, len(f.pulls))
	}
}

func TestOpenRemovesItsBranchWhenTheCommitFailsAndHidesGitHubDetail(t *testing.T) {
	f, g := newFake(t)
	f.failPut = true
	_, err := g.Open(context.Background(), proposal())
	if err == nil || strings.Contains(err.Error(), "secret repository detail") {
		t.Fatalf("the failure must be reported without GitHub's message: %v", err)
	}
	if len(f.deleted) != 1 || len(f.pulls) != 0 {
		t.Fatalf("an orphan branch was left behind: deleted=%v pulls=%d", f.deleted, len(f.pulls))
	}
	if _, ok := f.refs["heads/nines/commerce/checkout-0123456789ab"]; ok {
		t.Fatal("branch still exists")
	}
}

func TestProposalsAreScopedToTheTenantAndSLOAndReportMergedState(t *testing.T) {
	f, g := newFake(t)
	merged := "2026-11-18T10:00:00Z"
	mk := func(ref, state string, mergedAt *string, n int) map[string]any {
		return map[string]any{"number": n, "html_url": "u", "state": state, "merged_at": mergedAt, "title": ref, "body": "nines-definition-digest: " + digest + "\n", "created_at": time.Date(2026, 11, 18, n, 0, 0, 0, time.UTC).Format(time.RFC3339), "head": map[string]any{"ref": ref}}
	}
	f.pulls = []map[string]any{
		mk("nines/commerce/checkout-aaaaaaaaaaaa", "closed", &merged, 1),
		mk("nines/commerce/checkout-bbbbbbbbbbbb", "open", nil, 2),
		mk("nines/commerce/checkout-cccccccccccc", "closed", nil, 3),
		mk("nines/other/checkout-dddddddddddd", "open", nil, 4),
		mk("nines/commerce/search-eeeeeeeeeeee", "open", nil, 5),
		mk("feature/checkout-x", "open", nil, 6),
	}
	got, err := g.Proposals(context.Background(), "commerce", KindSLO, "checkout")
	if err != nil {
		t.Fatal(err)
	}
	var states []string
	for _, p := range got {
		states = append(states, p.State)
	}
	if len(got) != 3 || strings.Join(states, ",") != "closed,open,merged" {
		t.Fatalf("only this tenant's proposals for this SLO, newest first, with merged distinguished from closed: %v", states)
	}
}

func TestUnsafeNamesNeverBecomeBranchesAndConfigurationIsStrict(t *testing.T) {
	_, g := newFake(t)
	for _, p := range []Proposal{
		{Tenant: "../x", ID: "a", Digest: digest},
		{Tenant: "t", ID: "a/../b", Digest: digest},
		{Tenant: "t", ID: "a", Digest: "short"},
	} {
		if _, err := g.Open(context.Background(), p); err == nil {
			t.Fatalf("accepted %+v", p)
		}
	}
	token := func(string) string { return "tok" }
	for name, c := range map[string]Config{
		"provider":      {Provider: "bitbucket", Repository: "a/b", TokenEnv: "T"},
		"repository":    {Provider: "github", Repository: "a/b/c", TokenEnv: "T"},
		"traversal":     {Provider: "github", Repository: "a/b", Path: "../x", TokenEnv: "T"},
		"absolute path": {Provider: "github", Repository: "a/b", Path: "/x", TokenEnv: "T"},
		"hidden path":   {Provider: "github", Repository: "a/b", Path: ".github", TokenEnv: "T"},
		"branch":        {Provider: "github", Repository: "a/b", BaseBranch: "a..b", TokenEnv: "T"},
		"http api":      {Provider: "github", Repository: "a/b", APIURL: "http://ghe.example", TokenEnv: "T"},
		"credentials":   {Provider: "github", Repository: "a/b", APIURL: "https://u:p@ghe.example", TokenEnv: "T"},
		"token env":     {Provider: "github", Repository: "a/b", TokenEnv: "lower"},
	} {
		if _, err := Resolve(c, token); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := Resolve(Config{Provider: "github", Repository: "a/b", TokenEnv: "T"}, func(string) string { return "" }); err == nil {
		t.Error("an empty credential was accepted")
	}
	if _, err := Resolve(Config{Provider: "github", Repository: "a/b", TokenEnv: "T"}, func(string) string { return "a\nb" }); err == nil {
		t.Error("a multi-line credential was accepted")
	}
	s, err := Resolve(Config{Provider: "github", Repository: "a/b", TokenEnv: "T"}, func(string) string { return "s3cr3t-value" })
	if err != nil || s.BaseBranch != "main" || s.Path != "slos" || s.APIURL != "https://api.github.com" {
		t.Fatalf("defaults: %+v %v", s, err)
	}
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), "s3cr3t-value") {
		t.Fatal("the credential must never serialize:", string(b))
	}
}

func TestFileReadsTheBaseBranchDefinitionAndReportsAbsence(t *testing.T) {
	f, g := newFake(t)
	f.files["main:slos/checkout.slo.yaml"] = "kind: ServiceLevelObjective\n"
	got, path, found, err := g.File(context.Background(), KindSLO, "checkout")
	if err != nil || !found || path != "slos/checkout.slo.yaml" || string(got) != "kind: ServiceLevelObjective\n" {
		t.Fatalf("%q %s %v %v", got, path, found, err)
	}
	if _, _, found, err = g.File(context.Background(), KindSLO, "missing"); err != nil || found {
		t.Fatalf("an absent file is reported, not an error: %v %v", found, err)
	}
	if _, _, _, err = g.File(context.Background(), KindSLO, "../etc"); err == nil {
		t.Fatal("an unsafe identifier reached the provider")
	}
}
