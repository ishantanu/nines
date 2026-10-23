package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestInlineCompilationRolesTenantAndStrictResource(t *testing.T) {
	s := service.New(store.NewMemory(), &testtelemetry.Source{})
	other := service.New(store.NewMemory(), &testtelemetry.Source{Fail: true})
	h, e := NewMulti([]Workspace{
		{ID: "alpha", Name: "Alpha", Service: s, Grants: []tenancy.Grant{{Token: "viewer", Role: "viewer"}, {Token: "agent", Role: "agent"}}},
		{ID: "beta", Name: "Beta", Service: other, Grants: []tenancy.Grant{{Token: "beta", Role: "viewer"}}},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e != nil {
		t.Fatal(e)
	}
	manifest := v1.NewSLO("inline-api", "test", v1.SLOSpec{DisplayName: "API", Service: "api", Source: "primary", Target: .999, WindowSeconds: 3600, StepSeconds: 60, Measurement: &v1.MeasurementSpec{Type: "availability", Metric: "http_requests_total", Labels: map[string]string{"service": "api"}, SuccessLabels: map[string]string{"status": "200"}, ScrapeIntervalSeconds: 10, ObservationPoint: "API completions"}})
	request := func(tenant, token, typ string, body []byte, want int) []byte {
		t.Helper()
		r := httptest.NewRequest("POST", "/api/slos/compile", strings.NewReader(string(body)))
		r.Header.Set("Content-Type", typ)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set(tenancy.Header, tenant)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", tenant, token, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	for _, typ := range []string{"application/yaml", "application/json"} {
		body, _ := yaml.Marshal(manifest)
		if typ == "application/json" {
			body, _ = json.Marshal(manifest)
		}
		for _, role := range []string{"viewer", "agent"} {
			raw := request("alpha", role, typ, body, 200)
			var compiled v1.CompiledMeasurement
			if json.Unmarshal(raw, &compiled) != nil || compiled.Tenant != "alpha" || compiled.MeasurementDigest == "" || compiled.Manifest.Spec.MeasurementBinding != nil {
				t.Fatal(string(raw))
			}
		}
		request("beta", "viewer", typ, body, 403)
	}
	body, _ := yaml.Marshal(manifest)
	request("alpha", "viewer", "text/plain", body, 400)
	request("alpha", "viewer", "application/yaml", append(body, []byte("  unknownField: true\n")...), 400)
	request("alpha", "viewer", "application/yaml", append(body, []byte("---\n{}\n")...), 400)
	if rows, e := s.Store.List(context.Background()); e != nil || len(rows) != 0 {
		t.Fatal("compile created SLO", rows, e)
	}
	if rows, e := s.Store.(store.TopologyStore).ListTopology(context.Background(), v1.BindingKind, "", 100); e != nil || len(rows) != 0 {
		t.Fatal("compile created binding", rows, e)
	}
}
