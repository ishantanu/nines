package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ishantanu/nines/examples"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestYAMLResourcesTenantAuthorizationAndRoundTrip(t *testing.T) {
	a, b := service.New(store.NewMemory(), nil), service.New(store.NewMemory(), nil)
	h, err := NewMulti([]Workspace{
		{ID: "alpha", Name: "Alpha", Service: a, Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}, {Token: "viewer", Role: "viewer"}, {Token: "agent", Role: "agent"}}},
		{ID: "beta", Name: "Beta", Service: b, Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}}},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	request := func(token, tenant, method, path, body, media, match string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set(tenancy.Header, tenant)
		r.Header.Set("Content-Type", media)
		if match != "" {
			r.Header.Set("If-Match", match)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body)
		}
		return w
	}
	for _, body := range []string{examples.APIAvailability, examples.Checkout} {
		request("viewer", "alpha", "POST", "/api/resources/validate", body, "application/yaml", "", 200)
		request("agent", "alpha", "POST", "/api/resources/apply", body, "application/yaml", `"0"`, 403)
	}
	request("owner", "alpha", "POST", "/api/resources/plan", examples.Checkout, "application/yaml", "", 400) // dangling refs
	request("owner", "alpha", "POST", "/api/resources/apply", examples.APIAvailability, "application/json", `"0"`, 400)
	for _, match := range []string{"", "0", `W/"0"`, `"00"`} {
		request("owner", "alpha", "POST", "/api/resources/apply", examples.APIAvailability, "application/yaml", match, 400)
	}
	request("viewer", "alpha", "POST", "/api/resources/plan", examples.APIAvailability, "application/yaml", "", 200)
	w := request("owner", "alpha", "POST", "/api/resources/apply", examples.APIAvailability, "application/yaml", `"0"`, 200)
	if w.Header().Get("ETag") != `"1"` {
		t.Fatal("missing revision ETag")
	}
	request("owner", "alpha", "POST", "/api/resources/apply", examples.APIAvailability, "application/yaml", `"0"`, 409)
	request("owner", "alpha", "POST", "/api/resources/apply", examples.APIAvailability, "application/yaml", `"1"`, 200)
	request("owner", "beta", "GET", "/api/slos/api-availability/manifest", "", "", "", 404)
	// Checkout also refers to background-jobs. Create it through the same YAML path.
	for _, id := range []string{"background-jobs", "checkout-success"} {
		bg := strings.Replace(examples.APIAvailability, "name: api-availability", "name: "+id, 1)
		request("owner", "alpha", "POST", "/api/resources/apply", bg, "application/yaml", `"0"`, 200)
	}
	request("viewer", "alpha", "POST", "/api/resources/plan", examples.Checkout, "application/yaml", "", 200)
	request("owner", "alpha", "POST", "/api/resources/apply", examples.Checkout, "application/yaml", `"0"`, 200)
	for _, path := range []string{"/api/slos/api-availability/manifest", "/api/journeys/complete-checkout/manifest"} {
		exported := request("viewer", "alpha", "GET", path, "", "", "", 200)
		if exported.Header().Get("Content-Type") != "application/yaml" {
			t.Fatal("export not YAML")
		}
		if _, err := model.ParseDefinition(exported.Body.Bytes()); err != nil {
			t.Fatal("unparseable export", err)
		}
		planned := request("viewer", "alpha", "POST", "/api/resources/plan", exported.Body.String(), "application/yaml", "", 200)
		var result service.ResourcePlan
		if err := json.Unmarshal(planned.Body.Bytes(), &result); err != nil || result.Action != "unchanged" || result.ExpectedRevision != 1 {
			t.Fatal("round trip drift", result, err)
		}
	}
	request("viewer", "beta", "POST", "/api/resources/validate", examples.APIAvailability, "application/yaml", "", 403)
}
