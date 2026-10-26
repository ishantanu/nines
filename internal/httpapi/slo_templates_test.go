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
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestSLOTemplatesAreTenantAuthorizedReadOnlyDrafts(t *testing.T) {
	s := service.New(store.NewMemory(), nil)
	h, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: s, Grants: []tenancy.Grant{{Token: "viewer", Role: "viewer"}, {Token: "agent", Role: "agent"}}}, {ID: "beta", Name: "Beta", Service: service.New(store.NewMemory(), nil), Grants: []tenancy.Grant{{Token: "beta", Role: "viewer"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body, tenant, token string, want int) []byte {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set(tenancy.Header, tenant)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	for _, token := range []string{"viewer", "agent"} {
		data := request("GET", "/api/slo-templates?category=agent", "", "alpha", token, 200)
		var catalog v1.SLOTemplateCatalog
		if json.Unmarshal(data, &catalog) != nil || len(catalog.Templates) != 4 {
			t.Fatal(string(data))
		}
		request("GET", "/api/slo-templates/inference-ttft", "", "alpha", token, 200)
		data = request("POST", "/api/slo-templates/agent-task-completion/render", `{"name":"support-agent"}`, "alpha", token, 200)
		var draft v1.SLOTemplateDraft
		if json.Unmarshal(data, &draft) != nil || draft.TelemetryVerified || draft.Manifest.Metadata.Name != "support-agent" || draft.Manifest.Spec.Lifecycle != v1.LifecycleExperimental {
			t.Fatal(string(data))
		}
		request("GET", "/api/slo-templates", "", "beta", token, 403)
		request("POST", "/api/slo-templates/agent-task-completion/render", `{}`, "beta", token, 403)
		request("POST", "/api/slo-templates/agent-task-completion/render", `{"target":1}`, "alpha", token, 400)
		request("POST", "/api/slo-templates/agent-task-completion/render", `{"endpoint":"https://other"}`, "alpha", token, 400)
		request("POST", "/api/slo-templates/agent-task-completion/render", `{} {}`, "alpha", token, 400)
		request("GET", "/api/slo-templates?category=unknown", "", "alpha", token, 400)
		request("GET", "/api/slo-templates?tenant=beta", "", "alpha", token, 400)
		request("GET", "/api/slo-templates/not-a-template", "", "alpha", token, 404)
	}
	if rows, err := s.Store.List(context.Background()); err != nil || len(rows) != 0 {
		t.Fatal("render persisted resource", rows, err)
	}
}
