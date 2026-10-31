package client_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ishantanu/nines/client"
)

func TestNativeReadersRequireScopeAndEscapeVendorIDs(t *testing.T) {
	tenant := "alpha"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.Header.Get("X-Nines-Tenant") != "alpha" || r.Header.Get("Authorization") != "Bearer read" {
			t.Fatal("identity or read-only fence")
		}
		if r.URL.Path == "/api/connections/nr/native-slo-draft" && r.URL.Query().Get("nativeID") != "id?account=other" {
			t.Fatal("vendor ID not escaped")
		}
		_, _ = fmt.Fprintf(w, `{"apiVersion":"nines.dev/v1alpha1","tenant":%q,"items":[],"points":[]}`, tenant)
	}))
	defer server.Close()
	c, e := client.New(client.Config{URL: server.URL, Tenant: "alpha", Token: "read"})
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	from := time.Unix(1800000000, 0).UTC()
	if _, e = c.NativeSLOResults(ctx, "api", from, from.Add(time.Hour)); e != nil {
		t.Fatal(e)
	}
	if _, e = c.NativeSLOCatalog(ctx, "nr"); e != nil {
		t.Fatal(e)
	}
	if _, e = c.NativeSLODraft(ctx, "nr", "id?account=other", "imported", "prod", "api"); e != nil {
		t.Fatal(e)
	}
	tenant = "beta"
	if _, e = c.NativeSLOCatalog(ctx, "nr"); e == nil {
		t.Fatal("foreign response accepted")
	}
	before := calls
	if _, e = c.NativeSLOCatalog(ctx, "../beta"); e == nil {
		t.Fatal("path traversal")
	}
	if _, e = c.NativeSLOResults(ctx, "api", from, from.Add(32*24*time.Hour)); e == nil {
		t.Fatal("unbounded range")
	}
	if calls != before {
		t.Fatal("invalid input reached server")
	}
}
