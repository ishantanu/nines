package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/intentflow"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/slotemplates"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestExtensionAPIRequiresTenantMembershipTypedInputsAndExactPin(t *testing.T) {
	data, err := os.ReadFile("../../examples/extensions/example-agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(t.TempDir(), "extensions.lock.yaml")
	if _, err = slotemplates.Install(lock, data, slotemplates.SHA256(data), false); err != nil {
		t.Fatal(err)
	}
	registry, err := slotemplates.Load(lock)
	if err != nil {
		t.Fatal(err)
	}
	s := service.New(store.NewMemory(), nil)
	handler, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: s, Templates: registry, Grants: []tenancy.Grant{{Token: "agent", Role: "agent"}, {Token: "viewer", Role: "viewer"}}}, {ID: "beta", Name: "Beta", Service: service.New(store.NewMemory(), nil), Grants: []tenancy.Grant{{Token: "beta", Role: "viewer"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, tenant, token, body string, want int) []byte {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set(tenancy.Header, tenant)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	template, _ := registry.Get("example-agent--evaluation-pass")
	values := v1.TemplateValues{ExpectedTemplateDigest: template.Digest, Inputs: map[string]any{"metric": "support_evaluations_total", "service": "support-agent", "population": map[string]string{"service": "support-agent", "rubric": "support-v1", "evaluator": "rules-v1"}}}
	body, _ := json.Marshal(values)
	for _, role := range []string{"agent", "viewer"} {
		raw := request("GET", "/api/slo-templates?category=agent", "alpha", role, "", 200)
		var catalog v1.SLOTemplateCatalog
		if json.Unmarshal(raw, &catalog) != nil || len(catalog.Extensions) != 1 {
			t.Fatal(string(raw))
		}
		raw = request("POST", "/api/slo-templates/"+template.ID+"/render", "alpha", role, string(body), 200)
		var draft v1.SLOTemplateDraft
		if json.Unmarshal(raw, &draft) != nil || draft.Extension == nil || draft.TelemetryVerified {
			t.Fatal(string(raw))
		}
		request("POST", "/api/slo-templates/"+template.ID+"/render", "alpha", role, strings.Replace(string(body), template.Digest, strings.Repeat("0", 64), 1), 409)
		request("POST", "/api/slo-templates/"+template.ID+"/render", "alpha", role, `{"inputs":{"metric":"one","metric":"two"}}`, 400)
		request("POST", "/api/slo-templates/"+template.ID+"/render", "alpha", role, `{"expectedTemplateDigest":"`+template.Digest+`","inputs":{"endpoint":"https://other.invalid"}}`, 400)
		request("GET", "/api/slo-templates", "beta", role, "", 403)
	}
	request("GET", "/api/slo-templates/"+template.ID, "beta", "beta", "", 404)
	if rows, err := s.Store.List(context.Background()); err != nil || len(rows) != 0 {
		t.Fatal("extension rendering mutated store", err)
	}
}

func TestExtensionCatalogIsSharedByAPIAndTenantIntentWorker(t *testing.T) {
	data, err := os.ReadFile("../../examples/extensions/example-agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(t.TempDir(), "extensions.lock.yaml")
	if _, err = slotemplates.Install(lock, data, slotemplates.SHA256(data), false); err != nil {
		t.Fatal(err)
	}
	registry, err := slotemplates.Load(lock)
	if err != nil {
		t.Fatal(err)
	}
	makeWorkspace := func(id, token string, templates *slotemplates.Registry) Workspace {
		platform := service.New(store.NewMemory(), nil)
		return Workspace{ID: id, Name: id, Service: platform, Templates: templates, IntentConversations: intentflow.New(platform, intentflow.NewMemory()), Grants: []tenancy.Grant{{Token: token, Role: "viewer"}}}
	}
	workspaces := []Workspace{makeWorkspace("alpha", "alpha", registry), makeWorkspace("beta", "beta", nil)}
	handler, err := NewMulti(workspaces, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	for _, workspace := range workspaces {
		w := invRequest(handler, "GET", "/api/intent-conversations/capabilities", "", workspace.ID, workspace.ID)
		var capabilities struct {
			TemplateCatalogDigest string `json:"templateCatalogDigest"`
			TemplateSelection     bool   `json:"templateSelection"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &capabilities) != nil {
			t.Fatal(w.Code, w.Body.String())
		}
		catalog, err := workspace.Templates.List("")
		if err != nil || !capabilities.TemplateSelection || capabilities.TemplateCatalogDigest != catalog.Digest {
			t.Fatal("worker catalog differs from API catalog", workspace.ID, err)
		}
	}
	if _, err = workspaces[1].IntentConversations.Templates.Get("example-agent--evaluation-pass"); err == nil {
		t.Fatal("private recipe crossed tenant boundary")
	}
}
