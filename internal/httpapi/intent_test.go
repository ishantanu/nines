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
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestIntentAPIReadOnlyScopeAndBounds(t *testing.T) {
	s, _, err := testtelemetry.New()
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: s, Grants: []tenancy.Grant{{Token: "view", Role: "viewer"}, {Token: "agent", Role: "agent"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	in := v1.PlanIntentInput{Intent: v1.SLOIntent{APIVersion: v1.APIVersion, Kind: v1.IntentKind, Metadata: v1.Metadata{Name: "intent-api", Project: "test"}, Spec: v1.IntentSpec{Description: "Successful API attempts", Service: "api", Source: "primary", Outcome: "availability", Population: map[string]string{"service": "api"}, Target: .999, WindowSeconds: 3600, NoDataPolicy: "unknown"}}, Binding: &v1.IntentSelection{Name: "requests", ExpectedRevision: 1}}
	b, _ := json.Marshal(in)
	request := func(token, tenant, path, body string, want int) []byte {
		t.Helper()
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Nines-Tenant", tenant)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%d want %d: %s", w.Code, want, w.Body.String())
		}
		return w.Body.Bytes()
	}
	for _, token := range []string{"view", "agent"} {
		raw := request(token, "alpha", "/api/intents/plan", string(b), 200)
		var out v1.IntentPlan
		if json.Unmarshal(raw, &out) != nil || out.Tenant != "alpha" || out.Status != "ready" {
			t.Fatal(string(raw))
		}
		request(token, "alpha", "/api/resources/apply", out.Compiled.YAML, 403)
	}
	request("view", "beta", "/api/intents/plan", string(b), 403)
	request("view", "alpha", "/api/intents/plan?tenant=beta", string(b), 400)
	request("view", "alpha", "/api/intents/plan", strings.TrimSuffix(string(b), "}")+`,"orgID":"beta"}`, 400)
	request("view", "alpha", "/api/intents/plan", string(b)+" {}", 400)
	request("view", "alpha", "/api/intents/plan", strings.Repeat(" ", 32769)+string(b), 400)
	if _, err = s.Store.Read(context.Background(), "intent-api"); err == nil {
		t.Fatal("planning wrote resource")
	}
}
