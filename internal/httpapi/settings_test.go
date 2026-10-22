package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ishantanu/nines/internal/prometheus"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestAccessPolicyAndBoundConnectionProbe(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/prometheus/api/v1/query_range" || r.Header.Get("X-Scope-OrgID") != "trusted-org" || r.Header.Get("Authorization") != "Bearer upstream-secret" {
			t.Error("source boundary changed")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("query") != "vector(1)" {
			t.Error("caller supplied query")
		}
		end, _ := strconv.ParseInt(r.Form.Get("end"), 10, 64)
		_, _ = fmt.Fprintf(w, `{"status":"success","data":{"resultType":"matrix","result":[{"values":[[%d,"1"]]}]}}`, end)
	}))
	defer upstream.Close()
	source, _ := prometheus.NewMimir(upstream.URL+"/prometheus", "trusted-org", "upstream-secret")
	svc := service.New(store.NewMemory(), source)
	svc.SourceKind = "mimir"
	h, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: svc, Source: tenancy.Source{Kind: "mimir", URL: source.BaseURL, MimirOrgID: "trusted-org", BearerTokenEnv: "UPSTREAM_TOKEN"}, SourceAuth: "bearer", AccessConfig: []tenancy.Access{{Role: "owner", TokenEnv: "OWNER_TOKEN"}}, Grants: []tenancy.Grant{{Token: "owner-secret", Role: "owner"}, {Token: "agent-secret", Role: "agent"}, {Token: "viewer-secret", Role: "viewer"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	req := func(token, method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Scope-OrgID", "forged-org")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body)
		}
		for _, secret := range []string{"owner-secret", "agent-secret", "viewer-secret", "upstream-secret"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("secret exposed")
			}
		}
		return w
	}
	var policy struct {
		Bindings    []map[string]string `json:"bindings"`
		Permissions []struct {
			ID    string          `json:"id"`
			Roles map[string]bool `json:"roles"`
		} `json:"permissions"`
	}
	_ = json.Unmarshal(req("owner-secret", "GET", "/api/access", "", 200).Body.Bytes(), &policy)
	if len(policy.Bindings) != 3 || policy.Bindings[0]["credentialRef"] != "OWNER_TOKEN" {
		t.Fatal(policy)
	}
	for _, permission := range policy.Permissions {
		if permission.ID == "apply" && (permission.Roles["agent"] || permission.Roles["viewer"] || !permission.Roles["owner"]) {
			t.Fatal("policy disagrees with authorization")
		}
	}
	var viewer struct {
		Bindings []map[string]string `json:"bindings"`
	}
	_ = json.Unmarshal(req("viewer-secret", "GET", "/api/access", "", 200).Body.Bytes(), &viewer)
	if len(viewer.Bindings) != 0 {
		t.Fatal("viewer sees grants")
	}
	req("viewer-secret", "GET", "/api/connections", "", 200)
	req("viewer-secret", "POST", "/api/connections/primary/test", "", 403)
	req("agent-secret", "POST", "/api/connections/primary/test", "", 403)
	req("owner-secret", "POST", "/api/connections/primary/test", `{"url":"http://elsewhere"}`, 400)
	if calls.Load() != 0 {
		t.Fatal("denied probe reached source")
	}
	req("owner-secret", "POST", "/api/connections/primary/test", "", 200)
	if calls.Load() != 2 {
		t.Fatal("probe did not query both expressions", calls.Load())
	}
	records, err := svc.Store.List(context.Background())
	if err != nil || len(records) != 0 {
		t.Fatal("probe stored resources or samples", err)
	}
}
