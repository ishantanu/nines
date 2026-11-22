package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/features"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

const betaManifest = `apiVersion: nines.dev/v1beta1
kind: ServiceLevelObjective
metadata:
  name: checkout
  project: storefront
spec:
  displayName: Checkout
  service: checkout
  target: 0.999
  window: 7d
  queries:
    good: sum(increase(checkout_success_total[1m]))
    total: sum(increase(checkout_total[1m]))
`

func TestManifestsExportV1beta1AndPlansReportDefaults(t *testing.T) {
	h, err := NewMulti([]Workspace{{Features: features.All(), ID: "alpha", Name: "Alpha", Service: service.New(store.NewMemory(), nil), Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body, media string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer owner")
		r.Header.Set("Content-Type", media)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	plan := call("POST", "/api/resources/plan", betaManifest, "application/yaml")
	var p struct{ Notes []string }
	if plan.Code != 200 || json.Unmarshal(plan.Body.Bytes(), &p) != nil {
		t.Fatal(plan.Code, plan.Body.String())
	}
	if !strings.Contains(strings.Join(p.Notes, "\n"), "policies default to the standard burn-rate alerts") {
		t.Fatal("plan does not report the default alerts", p.Notes)
	}
	apply := httptest.NewRequest("POST", "/api/resources/apply", strings.NewReader(betaManifest))
	apply.Header.Set("Authorization", "Bearer owner")
	apply.Header.Set("Content-Type", "application/yaml")
	apply.Header.Set("If-Match", `"0"`)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, apply)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}

	exported := call("GET", "/api/slos/checkout/manifest", "", "")
	text := exported.Body.String()
	if exported.Code != 200 || !strings.Contains(text, "apiVersion: nines.dev/v1beta1") || !strings.Contains(text, "window: 7d") || strings.Contains(text, "Seconds") {
		t.Fatalf("manifest is not v1beta1:\n%s", text)
	}
	if d, err := model.ParseDefinition([]byte(text)); err != nil || d.SLO.Spec.WindowSeconds != 7*86400 || len(d.SLO.Spec.Policies) != 3 {
		t.Fatal("exported manifest does not read back to the stored SLO", err)
	}
	legacy := call("GET", "/api/slos/checkout/manifest?apiVersion=nines.dev/v1alpha1", "", "")
	if !strings.Contains(legacy.Body.String(), "apiVersion: nines.dev/v1alpha1") || !strings.Contains(legacy.Body.String(), "windowSeconds: 604800") {
		t.Fatalf("v1alpha1 export:\n%s", legacy.Body.String())
	}
	if bad := call("GET", "/api/slos/checkout/manifest?apiVersion=nines.dev/v2", "", ""); bad.Code != 400 {
		t.Fatal(bad.Code)
	}

	// Planning the v1alpha1 form says it is deprecated.
	old := call("POST", "/api/resources/plan", legacy.Body.String(), "application/yaml")
	if !strings.Contains(old.Body.String(), "nines.dev/v1alpha1 is deprecated") {
		t.Fatal(old.Body.String())
	}
}
