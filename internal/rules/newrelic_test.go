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

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/newrelic"
)

func TestNewRelicReviewedNativePlanAndRemovalVerification(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })

	const entity = "MTIzNDU2fFdPUktMT0FEfHRlc3Q="
	saved := []map[string]any{}
	writes := 0
	http.DefaultTransport = dash0Transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.eu.newrelic.com" || r.Header.Get("API-Key") != "synthetic-user" {
			t.Fatal("native route escaped")
		}
		var req struct {
			Query     string                     `json:"query"`
			Variables map[string]json.RawMessage `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		var data any
		if strings.Contains(req.Query, "mutation NinesNativeSLOCreate") {
			var doc newrelic.NativeDocument
			if err := json.Unmarshal(req.Variables["indicator"], &doc); err != nil {
				t.Fatal(err)
			}
			row := map[string]any{"id": "test-sli", "guid": "c3ludGhldGljLXNsaQ==", "entityGuid": entity, "name": doc.Name, "description": doc.Description, "objectives": doc.Objectives, "events": map[string]any{"account": map[string]any{"id": 123456}, "goodEvents": doc.Events.GoodEvents, "validEvents": doc.Events.ValidEvents}, "resultQueries": map[string]any{"goodEvents": map[string]any{"nrql": "FROM Metric SELECT sum(good)"}, "validEvents": map[string]any{"nrql": "FROM Metric SELECT sum(valid)"}, "indicator": map[string]any{"nrql": "FROM Metric SELECT sum(good)/sum(valid)"}}}
			saved = append(saved, row)
			writes++
			data = map[string]any{"serviceLevelCreate": row}
		} else {
			data = map[string]any{"actor": map[string]any{"entity": map[string]any{"guid": entity, "accountId": 123456, "serviceLevel": map[string]any{"indicators": saved}}}}
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"data": data}); err != nil {
			t.Fatal(err)
		}
		return w.Result(), nil
	})
	s := fixture()
	s.Source = "primary"
	s.WindowSeconds = 7 * 86400
	s.ProviderWindow = nil
	s.QueryLanguage = "nrql"
	s.GoodQuery = "SELECT count(*) FROM NinesRequest WHERE success = true"
	s.TotalQuery = "SELECT count(*) FROM NinesRequest"
	s.StepSeconds = 60
	manager := &Manager{Tenant: "team-a", Targets: map[string]Target{"primary": {Kind: "newrelic", Connection: "primary", QueryURL: "https://api.eu.newrelic.com/graphql", Namespace: "prod", NewRelicAccountID: "123456", NewRelicEntityGUID: entity, NewRelicUserKey: "synthetic-user"}}}
	ctx := context.Background()
	plan, err := manager.PlanContext(ctx, s)
	if err != nil || plan.NativeNewRelic == nil {
		t.Fatal(plan, err)
	}
	if strings.Contains(plan.YAML, "e+") {
		t.Fatal("native YAML renders numeric identifiers in scientific notation", plan.YAML)
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
	saved = []map[string]any{} // Simulates explicit administrator removal in Datadog.
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
