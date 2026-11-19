package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestGitProposalSendsTheReviewedRevisionAndDigestAndReadsStatus(t *testing.T) {
	digest := strings.Repeat("a", 64)
	var seen http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/gitops/proposals":
			seen = r.Header.Clone()
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"receipt":{"created":true,"path":"slos/x.slo.yaml","pullRequest":{"number":7,"url":"https://github.com/a/b/pull/7","state":"open"}},"definitionDigest":"` + digest + `","expectedRevision":2,"limitations":["not applied"]}`))
		case "/api/slos/x/gitops/proposals":
			_, _ = w.Write([]byte(`{"apiVersion":"nines.dev/v1alpha1","tenant":"alpha","configured":true,"liveDigest":"` + digest + `","proposals":[{"number":1,"state":"merged","applied":true}]}`))
		case "/api/gitops":
			_, _ = w.Write([]byte(`{"apiVersion":"nines.dev/v1alpha1","tenant":"alpha","configured":true,"repository":"a/b"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c, err := New(Config{URL: srv.URL, Tenant: "alpha", Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.ProposeYAML(context.Background(), []byte("kind: ServiceLevelObjective\n"), 2, digest)
	if err != nil || !got.Receipt.Created || got.Receipt.PullRequest.Number != 7 || got.ExpectedRevision != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	if seen.Get("If-Match") != `"2"` || seen.Get("X-Nines-Plan-Digest") != digest || seen.Get("Content-Type") != "application/yaml" {
		t.Fatalf("%v", seen)
	}
	for _, bad := range []struct {
		body     string
		expected int64
		digest   string
	}{{"", 0, digest}, {"x", -1, digest}, {"x", 0, "short"}} {
		if _, err := c.ProposeYAML(context.Background(), []byte(bad.body), bad.expected, bad.digest); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	info, err := c.GitInfo(context.Background())
	if err != nil || !info.Configured || info.Repository != "a/b" {
		t.Fatalf("%+v %v", info, err)
	}
	list, err := c.GitProposals(context.Background(), "x")
	if err != nil || len(list.Proposals) != 1 || !list.Proposals[0].Applied {
		t.Fatalf("%+v %v", list, err)
	}
	if _, err := c.GitProposals(context.Background(), "../x"); err == nil {
		t.Fatal("accepted an unsafe identifier")
	}
}

func TestGitDriftAndJourneyReadsUseTheirOwnRoutes(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte(`{"apiVersion":"nines.dev/v1alpha1","tenant":"alpha","state":"drifted","path":"slos/x.journey.yaml","proposals":[]}`))
	}))
	defer srv.Close()
	c, err := New(Config{URL: srv.URL, Tenant: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	d, err := c.GitDriftFor(context.Background(), "journey", "x")
	if err != nil || d.State != "drifted" {
		t.Fatalf("%+v %v", d, err)
	}
	if _, err = c.GitProposalsFor(context.Background(), "journey", "x"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(paths, ",") != "/api/journeys/x/gitops/drift,/api/journeys/x/gitops/proposals" {
		t.Fatalf("%v", paths)
	}
	for _, kind := range []string{"binding", "slos/../x"} {
		if _, err = c.GitDriftFor(context.Background(), kind, "x"); err == nil {
			t.Fatalf("accepted kind %q", kind)
		}
	}
}

func TestConvertMigrationValidatesBeforeSendingAndChecksScope(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 4096)
		n, _ := r.Body.Read(b)
		body = string(b[:n])
		_, _ = w.Write([]byte(`{"apiVersion":"nines.dev/v1alpha1","tenant":"alpha","tool":"sloth","definitions":[{"name":"x","origin":"o","yaml":"y","notes":[]}],"skipped":[],"notes":[],"limitations":[]}`))
	}))
	defer srv.Close()
	c, err := New(Config{URL: srv.URL, Tenant: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.ConvertMigration(context.Background(), v1.MigrationRequest{Tool: "sloth", Content: "version: prometheus/v1"})
	if err != nil || len(got.Definitions) != 1 || !strings.Contains(body, `"tool":"sloth"`) {
		t.Fatalf("%+v %v %s", got, err, body)
	}
	for _, bad := range []v1.MigrationRequest{{Tool: "datadog", Content: "x"}, {Tool: "sloth"}, {Tool: "pyrra", Content: strings.Repeat("a", 1<<20+1)}} {
		if _, err := c.ConvertMigration(context.Background(), bad); err == nil {
			t.Fatalf("accepted %q", bad.Tool)
		}
	}
	other, _ := New(Config{URL: srv.URL, Tenant: "beta"})
	if _, err := other.ConvertMigration(context.Background(), v1.MigrationRequest{Tool: "sloth", Content: "x"}); err == nil {
		t.Fatal("a response for another tenant must be refused")
	}
}
