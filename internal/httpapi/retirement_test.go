package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestRetirementHTTPRequiresOwnerAndTenantBoundReview(t *testing.T) {
	a, b := service.New(store.NewMemory(), nil), service.New(store.NewMemory(), nil)
	c := model.SLO{ID: "retiring", Project: "p", Service: "api", Name: "API", Target: .99, StepSeconds: 10, WindowSeconds: 60}
	if e := a.Create(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	handler, e := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: a, Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}, {Token: "editor", Role: "editor"}, {Token: "agent", Role: "agent"}, {Token: "viewer", Role: "viewer"}}}, {ID: "beta", Name: "Beta", Service: b, Grants: []tenancy.Grant{{Token: "beta", Role: "owner"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e != nil {
		t.Fatal(e)
	}
	req := func(tenant, token, method, path, body string, want int) []byte {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set(tenancy.Header, tenant)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d want %d %s", method, path, w.Code, want, w.Body.String())
		}
		return w.Body.Bytes()
	}
	path := "/api/slos/retiring/retirement"
	var plan v1.RetirementPlan
	body := req("alpha", "agent", "GET", path+"/plan?action=archive", "", 200)
	if e = json.Unmarshal(body, &plan); e != nil {
		t.Fatal(e)
	}
	in, _ := json.Marshal(v1.RetirementInput{Action: "archive", ExpectedRevision: 1, Digest: plan.Digest})
	for _, token := range []string{"editor", "agent", "viewer"} {
		req("alpha", token, "POST", path+"/apply", string(in), 403)
	}
	req("beta", "owner", "GET", path+"/plan?action=archive", "", 403)
	req("beta", "beta", "GET", path+"/plan?action=archive", "", 404)
	req("alpha", "owner", "POST", path+"/apply", string(in), 200)
	req("alpha", "viewer", "GET", path, "", 200)
	req("alpha", "agent", "POST", path+"/retry", "{}", 403)
}
