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

	"github.com/ishantanu/nines/internal/datadog"
	"github.com/ishantanu/nines/internal/model"
)

func TestDatadogReviewedNativePlanAndRemovalVerification(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	saved := map[string]datadog.NativeDocument{}
	writes := 0
	http.DefaultTransport = dash0Transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.uk1.datadoghq.com" || r.Header.Get("DD-API-KEY") != "synthetic-api" || r.Header.Get("DD-APPLICATION-KEY") != "synthetic-app" {
			t.Fatal("native route escaped")
		}
		w := httptest.NewRecorder()
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/metrics/"):
			if err := json.NewEncoder(w).Encode(map[string]any{"type": "count"}); err != nil {
				t.Fatal(err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/slo":
			rows := []datadog.NativeResource{}
			for id, d := range saved {
				rows = append(rows, datadog.NativeResource{NativeDocument: d, ID: id})
			}
			if err := json.NewEncoder(w).Encode(map[string]any{"data": rows}); err != nil {
				t.Fatal(err)
			}
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/slo/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/slo/")
			if err := json.NewEncoder(w).Encode(map[string]any{"data": datadog.NativeResource{NativeDocument: saved[id], ID: id}}); err != nil {
				t.Fatal(err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/slo":
			var d datadog.NativeDocument
			if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
				t.Fatal(err)
			}
			id := "0123456789abcdef0123456789abcdef"
			saved[id] = d
			writes++
			if err := json.NewEncoder(w).Encode(map[string]any{"data": []datadog.NativeResource{{NativeDocument: d, ID: id}}}); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatal("unexpected native update/delete")
		}
		return w.Result(), nil
	})
	s := fixture()
	s.Source = "primary"
	s.WindowSeconds = 7 * 86400
	s.ProviderWindow = nil
	s.QueryLanguage = "datadog"
	s.GoodQuery = "sum:api.good{env:prod}.as_count().rollup(sum,60)"
	s.TotalQuery = "sum:api.total{env:prod}.as_count().rollup(sum,60)"
	s.StepSeconds = 60
	manager := &Manager{Tenant: "team-a", Targets: map[string]Target{"primary": {Kind: "datadog", Connection: "primary", QueryURL: "https://api.uk1.datadoghq.com", Namespace: "prod", DatadogAPIKey: "synthetic-api", DatadogApplicationKey: "synthetic-app"}}}
	ctx := context.Background()
	plan, err := manager.PlanContext(ctx, s)
	if err != nil || plan.NativeDatadog == nil {
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
	delete(saved, current.NativeDatadog.NativeID) // Simulates explicit administrator removal in Datadog.
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
