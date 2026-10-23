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
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestAutomaticRulesControlsRequireOwnerExactRevisionAndExplicitAction(t *testing.T) {
	workspaces := []Workspace{}
	for _, id := range []string{"alpha", "beta"} {
		s := service.New(store.NewMemory(), nil)
		if e := s.Create(context.Background(), model.SLO{ID: "api", Project: "test", Name: "API", Service: "api", Target: .99, WindowSeconds: 3600, StepSeconds: 60, GoodQuery: "vector(99)", TotalQuery: "vector(100)"}); e != nil {
			t.Fatal(e)
		}
		m := &rules.Manager{Tenant: id, Targets: map[string]rules.Target{"primary": {Connection: "primary", Kind: "prometheus", QueryURL: "http://127.0.0.1:1", Directory: t.TempDir(), Namespace: "nines", Reload: true, AutoDeploy: true}}}
		grants := []tenancy.Grant{}
		for _, role := range []string{"owner", "editor", "agent", "viewer"} {
			grants = append(grants, tenancy.Grant{Token: id + "-" + role, Role: role})
		}
		workspaces = append(workspaces, Workspace{ID: id, Name: id, Service: s, Rules: m, Grants: grants})
	}
	h, e := NewMulti(workspaces, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e != nil {
		t.Fatal(e)
	}
	request := func(token, tenant, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("POST", "/api/slos/api/rules/automation", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set(tenancy.Header, tenant)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", token, tenant, w.Code, w.Body.String())
		}
		return w
	}
	body := `{"expectedRevision":1,"paused":true}`
	for _, role := range []string{"agent", "editor", "viewer"} {
		request("alpha-"+role, "alpha", body, 403)
	}
	request("alpha-owner", "beta", body, 403)
	request("alpha-owner", "alpha", `{"expectedRevision":1}`, 400)
	request("alpha-owner", "alpha", `{"expectedRevision":2,"paused":true}`, 409)
	request("alpha-owner", "alpha", `{"expectedRevision":1,"paused":true,"tenant":"beta"}`, 400)
	w := request("alpha-owner", "alpha", body, 200)
	var result v1.RuleAutomationResult
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Tenant != "alpha" || !result.Automation.Paused {
		t.Fatal(w.Body.String())
	}
	record, _ := workspaces[0].Service.Store.Read(context.Background(), "api")
	if model.ConfigRevision(record.SLO) != 1 || record.RuleAutomation == nil || !record.RuleAutomation.Paused {
		t.Fatal(record)
	}
	request("alpha-owner", "alpha", `{"expectedRevision":1,"paused":false}`, 200)
	record, _ = workspaces[0].Service.Store.Read(context.Background(), "api")
	if record.RuleAutomation != nil {
		t.Fatal("resume did not queue fresh reconciliation")
	}
	other, _ := workspaces[1].Service.Store.Read(context.Background(), "api")
	if other.RuleAutomation != nil {
		t.Fatal("cross-tenant status mutation")
	}
}
