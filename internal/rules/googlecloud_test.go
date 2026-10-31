package rules

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/googlecloud"
	"github.com/ishantanu/nines/internal/model"
)

func TestGoogleCloudReviewedNativePlanAndRemovalVerification(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	saved := map[string]googlecloud.NativeDocument{}
	writes := 0
	http.DefaultTransport = dash0Transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "monitoring.googleapis.com" || r.Header.Get("Authorization") != "Bearer synthetic-publisher-key" || !strings.HasPrefix(r.URL.Path, "/v3/projects/nines-test/services/checkout/") {
			t.Fatal("native route escaped configuration")
		}
		w := httptest.NewRecorder()
		switch r.Method {
		case http.MethodGet:
			if strings.HasSuffix(r.URL.Path, "/serviceLevelObjectives") {
				rows := []googlecloud.NativeDocument{}
				for _, doc := range saved {
					rows = append(rows, doc)
				}
				if err := json.NewEncoder(w).Encode(map[string]any{"serviceLevelObjectives": rows}); err != nil {
					t.Fatal(err)
				}
				return w.Result(), nil
			}
			doc, ok := saved[strings.TrimPrefix(r.URL.Path, "/v3/")]
			if !ok {
				w.WriteHeader(404)
			} else if err := json.NewEncoder(w).Encode(doc); err != nil {
				t.Fatal(err)
			}
		case http.MethodPost:
			var doc googlecloud.NativeDocument
			if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
				t.Fatal(err)
			}
			if _, exists := saved[doc.Name]; exists {
				w.WriteHeader(409)
			} else {
				saved[doc.Name] = doc
				writes++
			}
		default:
			t.Fatal("unexpected native update/delete")
		}
		return w.Result(), nil
	})
	s := fixture()
	s.Source = "primary"
	s.WindowSeconds = 28 * 86400
	s.ProviderWindow = nil
	s.QueryLanguage = "googlecloud"
	s.GoodQuery = `metric.type = "custom.googleapis.com/nines/requests" AND resource.type = "global" AND metric.labels.result = "good"`
	s.TotalQuery = `metric.type = "custom.googleapis.com/nines/requests" AND resource.type = "global"`
	manager := &Manager{Tenant: "team-a", Targets: map[string]Target{"primary": {Kind: "googlecloud", Connection: "primary", QueryURL: googlecloud.Endpoint, GoogleProjectID: "nines-test", GoogleServiceID: "checkout", GoogleAuth: "access-token", Namespace: "nines-prod", BearerToken: "synthetic-publisher-key"}}}
	ctx := context.Background()
	plan, err := manager.PlanContext(ctx, s)
	if err != nil || plan.NativeGoogleCloud == nil {
		t.Fatal(plan, err)
	}
	if _, err = manager.Deploy(ctx, s, "unreviewed"); !errors.Is(err, model.ErrRevisionConflict) || writes != 0 {
		t.Fatal("unreviewed write", err)
	}
	manager.BeforeWrite = func() error { return model.ErrRevisionConflict }
	if _, err = manager.Deploy(ctx, s, plan.Digest); !errors.Is(err, model.ErrRevisionConflict) || writes != 0 {
		t.Fatal("guard bypass", err)
	}
	manager.BeforeWrite = nil
	result, err := manager.Deploy(ctx, s, plan.Digest)
	if err != nil || result.State != "stored" || result.Evaluation.State != "unknown" || writes != 1 {
		t.Fatal(result, err)
	}
	if _, err = manager.Deploy(ctx, s, plan.Digest); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("stale plan reused", err)
	}
	current, err := manager.PlanContext(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Withdraw(ctx, s, current.Digest); err == nil {
		t.Fatal("retained resource silently removed")
	}
	delete(saved, current.NativeGoogleCloud.Origin) // Simulates explicit administrator removal in Google Cloud.
	absent, err := manager.PlanContext(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	result, err = manager.Withdraw(ctx, s, absent.Digest)
	if err != nil || result.State != "withdrawn" || writes != 1 {
		t.Fatal(result, err)
	}
	_, evaluation := manager.Inspect(ctx, s, time.Now().UTC())
	if evaluation.State != "unknown" {
		t.Fatal("absent SLO claimed evaluation", evaluation)
	}
}
