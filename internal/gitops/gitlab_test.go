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

// fakeGitLab implements the GitLab v4 endpoints the client uses, in memory.
type fakeGitLab struct {
	branches   map[string]bool
	files      map[string]string // branch:path -> content
	mrs        []map[string]any
	deleted    []string
	failCommit bool
	tokens     []string
}

func newFakeGitLab(t *testing.T) (*fakeGitLab, Provider) {
	f := &fakeGitLab{branches: map[string]bool{"main": true}, files: map[string]string{}}
	srv := httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	s, err := Resolve(Config{Provider: "gitlab", Repository: "platform/team/slos", TokenEnv: "GL_TOKEN", Path: "slos/commerce"}, func(string) string { return "glpat-secret" })
	if err != nil {
		t.Fatal(err)
	}
	s.APIURL = srv.URL
	p, err := New(s, srv.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	return f, p
}

func (f *fakeGitLab) serve(w http.ResponseWriter, r *http.Request) {
	f.tokens = append(f.tokens, r.Header.Get("PRIVATE-TOKEN"))
	reply := func(code int, v any) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	// The nested project path must arrive percent-encoded as one segment.
	path := r.URL.EscapedPath()
	const project = "/projects/platform%2Fteam%2Fslos"
	if !strings.HasPrefix(path, project) {
		reply(404, map[string]string{"message": "wrong project path " + path})
		return
	}
	path = strings.TrimPrefix(path, project)
	var in map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&in)
	}
	q := r.URL.Query()
	switch {
	case r.Method == "GET" && strings.HasPrefix(path, "/repository/branches/"):
		if f.branches[strings.TrimPrefix(path, "/repository/branches/")] {
			reply(200, map[string]any{"name": "ok"})
			return
		}
		reply(404, map[string]string{"message": "404 Branch Not Found"})
	case r.Method == "POST" && path == "/repository/branches":
		if f.branches[q.Get("branch")] {
			reply(400, map[string]string{"message": "Branch already exists"})
			return
		}
		f.branches[q.Get("branch")] = true
		reply(201, map[string]any{})
	case r.Method == "DELETE" && strings.HasPrefix(path, "/repository/branches/"):
		name := strings.TrimPrefix(path, "/repository/branches/")
		delete(f.branches, strings.ReplaceAll(name, "%2F", "/"))
		f.deleted = append(f.deleted, name)
		w.WriteHeader(204)
	case r.Method == "GET" && strings.HasPrefix(path, "/repository/files/"):
		file := strings.ReplaceAll(strings.TrimPrefix(path, "/repository/files/"), "%2F", "/")
		if content, ok := f.files[q.Get("ref")+":"+file]; ok {
			reply(200, map[string]any{"encoding": "base64", "size": len(content), "content": base64.StdEncoding.EncodeToString([]byte(content))})
			return
		}
		reply(404, map[string]string{"message": "404 File Not Found"})
	case r.Method == "POST" && path == "/repository/commits":
		if f.failCommit {
			reply(403, map[string]string{"message": "secret project detail"})
			return
		}
		branch := in["branch"].(string)
		a := in["actions"].([]any)[0].(map[string]any)
		raw, _ := base64.StdEncoding.DecodeString(a["content"].(string))
		f.files[branch+":"+a["file_path"].(string)] = string(raw)
		reply(201, map[string]any{})
	case r.Method == "POST" && path == "/merge_requests":
		mr := map[string]any{"iid": len(f.mrs) + 1, "web_url": "https://gitlab.example/mr", "state": "opened", "title": in["title"], "description": in["description"], "created_at": time.Now().UTC().Format(time.RFC3339), "source_branch": in["source_branch"]}
		f.mrs = append(f.mrs, mr)
		reply(201, mr)
	case r.Method == "GET" && path == "/merge_requests":
		out := []map[string]any{}
		for _, m := range f.mrs {
			if sb := q.Get("source_branch"); sb == "" || m["source_branch"] == sb {
				out = append(out, m)
			}
		}
		reply(200, out)
	default:
		reply(404, map[string]string{"message": "unexpected " + r.Method + " " + path})
	}
}

func TestGitLabOpensAMergeRequestInANestedProjectAndIsIdempotent(t *testing.T) {
	f, g := newFakeGitLab(t)
	p := proposal()
	got, err := g.Open(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	branch := "nines/commerce/checkout-0123456789ab"
	if !got.Created || got.Path != "slos/commerce/checkout.slo.yaml" || got.PullRequest.State != "open" || got.PullRequest.Branch != branch {
		t.Fatalf("%+v", got)
	}
	if f.files[branch+":slos/commerce/checkout.slo.yaml"] != "kind: ServiceLevelObjective\n" {
		t.Fatalf("the exact reviewed definition was not committed: %v", f.files)
	}
	if got.PullRequest.DefinitionDigest != digest || got.PullRequest.ExpectedRevision != 3 {
		t.Fatalf("the merge request must carry digest and revision: %+v", got.PullRequest)
	}
	for _, tok := range f.tokens {
		if tok != "glpat-secret" {
			t.Fatal("every request carries the configured token")
		}
	}
	again, err := g.Open(context.Background(), p)
	if err != nil || again.Created || len(f.mrs) != 1 {
		t.Fatalf("%+v %v mrs=%d", again, err, len(f.mrs))
	}
}

func TestGitLabRemovesItsBranchOnFailureAndMapsStates(t *testing.T) {
	f, g := newFakeGitLab(t)
	f.failCommit = true
	if _, err := g.Open(context.Background(), proposal()); err == nil || strings.Contains(err.Error(), "secret project detail") {
		t.Fatalf("the failure must not echo provider detail: %v", err)
	}
	if len(f.deleted) != 1 || len(f.mrs) != 0 {
		t.Fatalf("orphan branch: deleted=%v mrs=%d", f.deleted, len(f.mrs))
	}
	mk := func(branch, state string, n int) map[string]any {
		return map[string]any{"iid": n, "web_url": "u", "state": state, "title": branch, "description": "nines-definition-digest: " + digest + "\n", "created_at": time.Date(2026, 11, 18, n, 0, 0, 0, time.UTC).Format(time.RFC3339), "source_branch": branch}
	}
	f.mrs = []map[string]any{mk("nines/commerce/checkout-aaaaaaaaaaaa", "merged", 1), mk("nines/commerce/checkout-bbbbbbbbbbbb", "opened", 2), mk("nines/commerce/checkout-cccccccccccc", "closed", 3), mk("nines/commerce/journeys/checkout-dddddddddddd", "opened", 4), mk("nines/other/checkout-eeeeeeeeeeee", "opened", 5)}
	got, err := g.Proposals(context.Background(), "commerce", KindSLO, "checkout")
	if err != nil {
		t.Fatal(err)
	}
	var states []string
	for _, pr := range got {
		states = append(states, pr.State)
	}
	if strings.Join(states, ",") != "closed,open,merged" {
		t.Fatalf("an SLO's list must exclude journeys with the same id and other tenants: %v", states)
	}
	journeys, _ := g.Proposals(context.Background(), "commerce", KindJourney, "checkout")
	if len(journeys) != 1 || journeys[0].Number != 4 {
		t.Fatalf("%+v", journeys)
	}
}

func TestGitLabReadsTheBaseBranchFile(t *testing.T) {
	f, g := newFakeGitLab(t)
	f.files["main:slos/commerce/checkout.slo.yaml"] = "kind: ServiceLevelObjective\n"
	content, path, found, err := g.File(context.Background(), KindSLO, "checkout")
	if err != nil || !found || path != "slos/commerce/checkout.slo.yaml" || string(content) != "kind: ServiceLevelObjective\n" {
		t.Fatalf("%q %s %v %v", content, path, found, err)
	}
	if _, _, found, err = g.File(context.Background(), KindJourney, "checkout"); err != nil || found {
		t.Fatalf("%v %v", found, err)
	}
}

func TestJourneyProposalsUseTheirOwnFileAndBranchNamespace(t *testing.T) {
	f, g := newFake(t)
	p := proposal()
	p.Kind = KindJourney
	got, err := g.Open(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "slos/checkout.journey.yaml" || got.PullRequest.Branch != "nines/commerce/journeys/checkout-0123456789ab" {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(f.pulls[0]["body"].(string), "does **not** apply the journey in Nines") {
		t.Fatal(f.pulls[0]["body"])
	}
	// The same identifier as an SLO is a different branch and file.
	p.Kind = KindSLO
	slo, err := g.Open(context.Background(), p)
	if err != nil || slo.PullRequest.Number == got.PullRequest.Number || slo.Path != "slos/checkout.slo.yaml" {
		t.Fatalf("%+v %v", slo, err)
	}
	if _, err = g.Open(context.Background(), Proposal{Tenant: "commerce", Kind: "binding", ID: "x", Digest: digest, Manifest: []byte("x")}); err == nil {
		t.Fatal("an unsupported kind was accepted")
	}
}

func TestGitLabConfigurationDefaultsAndStrictness(t *testing.T) {
	token := func(string) string { return "t" }
	s, err := Resolve(Config{Provider: "gitlab", Repository: "group/sub/project", TokenEnv: "T"}, token)
	if err != nil || s.APIURL != "https://gitlab.com/api/v4" {
		t.Fatalf("%+v %v", s, err)
	}
	for name, c := range map[string]Config{
		"single segment": {Provider: "gitlab", Repository: "project", TokenEnv: "T"},
		"traversal":      {Provider: "gitlab", Repository: "a/../b", TokenEnv: "T"},
		"http":           {Provider: "gitlab", Repository: "a/b", APIURL: "http://gitlab.example/api/v4", TokenEnv: "T"},
	} {
		if _, err := Resolve(c, token); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
