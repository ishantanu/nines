package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

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

	at := time.Now().UTC().Truncate(time.Minute)
	if err := a.Ingest(context.Background(), "api-availability", []model.Point{{Time: at, Good: 99, Total: 100}}); err != nil {
		t.Fatal(err)
	}
	updated, err := model.ParseDefinition([]byte(examples.APIAvailability))
	if err != nil {
		t.Fatal(err)
	}
	updated.SLO.Spec.GoodQuery = "vector(95)"
	updated.SLO.Spec.TotalQuery = "vector(100)"
	updated.SLO.Spec.StepSeconds = 30
	body, err := yaml.Marshal(updated.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	planned := request("owner", "alpha", "POST", "/api/resources/plan", string(body), "application/yaml", "", 200)
	var editPlan service.ResourcePlan
	if err = json.Unmarshal(planned.Body.Bytes(), &editPlan); err != nil || !editPlan.MeasurementChanged || len(editPlan.Warnings) == 0 {
		t.Fatal(editPlan, err)
	}
	request("agent", "alpha", "POST", "/api/resources/apply", string(body), "application/yaml", `"1"`, 403)
	request("owner", "alpha", "POST", "/api/resources/apply", string(body), "application/yaml", `"1"`, 200)
	request("owner", "alpha", "POST", "/api/resources/apply", string(body), "application/yaml", `"1"`, 409)
	archived := request("viewer", "alpha", "GET", "/api/slos/api-availability/revisions/1", "", "", "", 200)
	var archivedRevision struct {
		Revision        int64
		IntervalSamples int
		ArchivedBy      struct{ Subject string }
	}
	if err = json.Unmarshal(archived.Body.Bytes(), &archivedRevision); err != nil || archivedRevision.Revision != 1 || archivedRevision.IntervalSamples != 1 || archivedRevision.ArchivedBy.Subject == "" {
		t.Fatal(archived.Body.String(), err)
	}
	request("viewer", "alpha", "GET", "/api/slos/api-availability/revisions?limit=1", "", "", "", 200)
	history := request("viewer", "alpha", "GET", "/api/slos/api-availability/history?revision=1&rangeSeconds=3600", "", "", "", 200)
	if !strings.Contains(history.Body.String(), `"configRevision":1`) {
		t.Fatal(history.Body.String())
	}
	request("owner", "beta", "GET", "/api/slos/api-availability/revisions", "", "", "", 404)
	request("owner", "beta", "GET", "/api/slos/api-availability/revisions/1", "", "", "", 404)
	request("owner", "alpha", "GET", "/api/slos/api-availability/revisions?limit=101", "", "", "", 400)
	request("owner", "alpha", "GET", "/api/slos/api-availability/revisions/0", "", "", "", 400)
	request("owner", "alpha", "GET", "/api/slos/api-availability/history?revision=-1", "", "", "", 400)
}
