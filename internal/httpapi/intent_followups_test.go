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

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

type apiDraftModel struct{}

func (apiDraftModel) ModelID() string { return "api-test-model" }
func (apiDraftModel) Draft(context.Context, string) (v1.ModelGoal, error) {
	a, b := "99.9%", "28 days"
	return v1.ModelGoal{Outcome: "availability", TargetText: &a, WindowText: &b, Clarifications: []string{}}, nil
}
func TestIntentFollowupsTenantRolesAndStrictRequests(t *testing.T) {
	s, _, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	s.IntentDrafter = apiDraftModel{}
	h, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: s, Grants: []tenancy.Grant{{Token: "view", Role: "viewer"}, {Token: "agent", Role: "agent"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, token, tenant, body string, want int) []byte {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Nines-Tenant", tenant)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w.Body.Bytes()
	}
	draft := `{"text":"99.9% successful over 28 days","metadata":{"name":"api-draft","project":"test"},"service":"api","source":"primary","population":{"service":"api"},"noDataPolicy":"unknown"}`
	request("POST", "/api/intents/draft", "view", "alpha", draft, 403)
	raw := request("POST", "/api/intents/draft", "agent", "alpha", draft, 200)
	var out v1.IntentDraft
	if json.Unmarshal(raw, &out) != nil || out.Tenant != "alpha" || out.Intent == nil || out.Status != "draft" {
		t.Fatal(string(raw))
	}
	request("POST", "/api/intents/draft", "agent", "beta", draft, 403)
	request("POST", "/api/intents/draft", "agent", "alpha", strings.TrimSuffix(draft, "}")+`,"endpoint":"http://other"}`, 400)
	request("GET", "/api/intents/capabilities", "view", "alpha", "", 200)
	request("GET", "/api/bindings/requests/health?expectedRevision=1", "view", "alpha", "", 200)
	request("GET", "/api/bindings/requests/health?expectedRevision=2", "view", "alpha", "", 409)
	request("GET", "/api/bindings/requests/health?expectedRevision=1&orgID=other", "view", "alpha", "", 400)
	now := time.Now().UTC().Truncate(time.Minute).Add(-5 * time.Minute)
	plan := v1.PlanIntentInput{Intent: *out.Intent, Binding: &v1.IntentSelection{Name: "requests", ExpectedRevision: 1}}
	input := v1.IntentBacktestInput{Plan: plan, From: now.Add(-time.Hour), To: now, ResolutionSeconds: 300}
	b, _ := json.Marshal(input)
	raw = request("POST", "/api/intents/backtest", "view", "alpha", string(b), 200)
	var result v1.IntentBacktest
	if json.Unmarshal(raw, &result) != nil || len(result.Points) != 12 || result.Tenant != "alpha" {
		t.Fatal(string(raw))
	}
	request("POST", "/api/intents/backtest?source=other", "view", "alpha", string(b), 400)
	request("POST", "/api/intents/backtest", "view", "alpha", strings.TrimSuffix(string(b), "}")+`,"query":"vector(1)"}`, 400)
}
