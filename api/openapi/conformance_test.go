package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ishantanu/nines/internal/features"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/intentflow"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/slotemplates"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/workflow"
)

// Real responses from the HTTP handlers must match the contract. This catches
// what type-level checks cannot, such as custom JSON encoders.
func TestResponsesConformToTheContract(t *testing.T) {
	raw, err := os.ReadFile("v1alpha1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err = yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)

	ctx := context.Background()
	svc := service.New(store.NewMemory(), nil)
	now := time.Now().UTC().Truncate(time.Minute)
	svc.Now = func() time.Time { return now }
	policy := model.Policy{Name: "fast-burn", Severity: "critical", ShortWindowSeconds: 300, LongWindowSeconds: 3600, BurnRate: 14.4, MinCoverage: 1}
	if err = svc.Create(ctx, model.SLO{ID: "checkout", Project: "shop", Name: "Checkout", Service: "api", Target: 0.99, WindowSeconds: 3600, StepSeconds: 60, Policies: []model.Policy{policy}}); err != nil {
		t.Fatal(err)
	}
	var points []model.Point
	for m := 1; m <= 60; m++ {
		points = append(points, model.Point{Time: now.Add(time.Duration(m-60) * time.Minute), Good: 990, Total: 1000})
	}
	if err = svc.Ingest(ctx, "checkout", points); err != nil {
		t.Fatal(err)
	}
	for _, manifest := range []string{
		"apiVersion: nines.dev/v1beta1\nkind: CriticalUserJourney\nmetadata: {name: purchase, project: shop}\nspec:\n  displayName: Purchase\n  description: Buy.\n  owner: team\n  criticality: high\n  slos: [{id: checkout, required: true}]\n  steps: [{id: pay, name: Pay, actor: user, sloRefs: [checkout]}]\n",
		"apiVersion: nines.dev/v1beta1\nkind: ServiceDependencyMap\nmetadata: {name: shop, project: shop}\nspec:\n  displayName: Shop\n  services: [{name: api, displayName: API, sloRefs: [checkout]}, {name: db, displayName: DB, sloRefs: []}]\n  dependencies: [{caller: api, callee: db, critical: true}]\n",
	} {
		d, err := model.ParseDefinition([]byte(manifest))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = svc.ApplyDefinition(ctx, d, 0); err != nil {
			t.Fatal(err)
		}
	}
	// The tenant as cmd/server wires it, so templates, skills and audit are
	// mounted too.
	handler, err := httpapi.NewMulti([]httpapi.Workspace{{
		ID: "default", Name: "Default workspace", Service: svc, Features: features.All(),
		Grants:              []tenancy.Grant{{Token: "owner", Role: "owner"}},
		Templates:           slotemplates.Default(),
		Workflows:           workflow.New(svc, workflow.NewMemory()),
		IntentConversations: intentflow.New(svc, intentflow.NewMemory()),
	}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	for route, want := range map[string]map[string]any{
		"/api/slos":                                            {"type": "array", "items": ref("SLOView")},
		"/api/slos/checkout":                                   ref("SLOView"),
		"/api/slos/checkout/status":                            ref("Snapshot"),
		"/api/slos/checkout/quality":                           ref("MeasurementQuality"),
		"/api/slos/checkout/budget":                            ref("ErrorBudgetDecision"),
		"/api/slos/checkout/samples":                           {"type": "array", "items": ref("Point")},
		"/api/slos/checkout/history":                           ref("History"),
		"/api/slos/checkout/revisions":                         ref("SLORevisionList"),
		"/api/slos/checkout/journeys":                          {"type": "array", "items": ref("JourneyRevision")},
		"/api/slos/checkout/context":                           ref("SLOContext"),
		"/api/slos/checkout/retirement/plan?action=archive":    ref("RetirementPlan"),
		"/api/access":                                          ref("AccessView"),
		"/api/journeys":                                        {"type": "array", "items": ref("JourneyView")},
		"/api/journeys/purchase":                               ref("JourneyView"),
		"/api/journeys/purchase/history":                       {"type": "array", "items": ref("JourneyRevision")},
		"/api/journeys/purchase/runs":                          {"type": "array", "items": ref("JourneyRun")},
		"/api/journeys/purchase/context":                       ref("JourneyContext"),
		"/api/dependency-maps/shop":                            ref("TopologyResource"),
		"/api/dependency-maps/shop/view":                       ref("DependencyView"),
		"/api/dependency-maps/shop/composition":                ref("DependencyComposition"),
		"/api/workspace":                                       ref("Workspace"),
		"/api/slo-templates":                                   ref("SLOTemplateCatalog"),
		"/api/slo-templates/agent-task-completion":             ref("SLOTemplateDetail"),
		"POST /api/slo-templates/agent-task-completion/render": ref("SLOTemplateDraft"),
		"/api/resources/search":                                ref("ResourceSearch"),
		"/api/topology/resources":                              ref("TopologyCatalog"),
		"/api/skills":                                          {"type": "array", "items": ref("Skill")},
		"/api/skills/reliability-investigation":                ref("Skill"),
		"/api/events":                                          {"type": "array", "items": ref("AlertDelivery")},
		"/api/audit":                                           ref("AuditPage"),
		"/api/audit/archive":                                   ref("AuditArchiveStatus"),
	} {
		method, path, post := "GET", route, strings.HasPrefix(route, "POST ")
		var payload io.Reader
		if post {
			method, path, payload = "POST", strings.TrimPrefix(route, "POST "), strings.NewReader(`{"project":"shop"}`)
		}
		req, _ := http.NewRequest(method, server.URL+path, payload)
		req.Header.Set("Authorization", "Bearer owner")
		if post {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != 200 {
			t.Errorf("%s: HTTP %d: %s", path, res.StatusCode, body)
			continue
		}
		var value any
		if err = json.Unmarshal(body, &value); err != nil {
			t.Fatal(path, err)
		}
		if err = conforms(value, want, schemas, "$"); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}

func ref(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

// conforms checks the JSON Schema subset the contract uses: $ref, type,
// oneOf, properties, required, additionalProperties, items and enum.
func conforms(v any, s map[string]any, schemas map[string]any, at string) error {
	if r, ok := s["$ref"].(string); ok {
		target, ok := schemas[strings.TrimPrefix(r, "#/components/schemas/")].(map[string]any)
		if !ok {
			return fmt.Errorf("%s: unresolved %s", at, r)
		}
		return conforms(v, target, schemas, at)
	}
	if options, ok := s["oneOf"].([]any); ok {
		for _, o := range options {
			if conforms(v, o.(map[string]any), schemas, at) == nil {
				return nil
			}
		}
		return fmt.Errorf("%s: matches no oneOf option", at)
	}
	if enum, ok := s["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			found = found || fmt.Sprint(e) == fmt.Sprint(v)
		}
		if !found {
			return fmt.Errorf("%s: %v not in enum", at, v)
		}
	}
	typ, _ := s["type"].(string)
	if types, ok := s["type"].([]any); ok {
		// A type list, such as [array, null]: null is allowed, otherwise
		// check the first type.
		if v == nil {
			for _, t := range types {
				if t == "null" {
					return nil
				}
			}
		}
		typ = fmt.Sprint(types[0])
	}
	switch typ {
	case "":
		return nil
	case "null":
		if v != nil {
			return fmt.Errorf("%s: want null", at)
		}
	case "string":
		if _, ok := v.(string); !ok {
			return fmt.Errorf("%s: want string, got %T", at, v)
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("%s: want boolean, got %T", at, v)
		}
	case "number", "integer":
		n, ok := v.(float64)
		if !ok {
			return fmt.Errorf("%s: want %s, got %T", at, typ, v)
		}
		if typ == "integer" && n != float64(int64(n)) {
			return fmt.Errorf("%s: want integer, got %v", at, n)
		}
	case "array":
		items, ok := v.([]any)
		if !ok {
			return fmt.Errorf("%s: want array, got %T", at, v)
		}
		if item, ok := s["items"].(map[string]any); ok {
			for i, x := range items {
				if err := conforms(x, item, schemas, fmt.Sprintf("%s[%d]", at, i)); err != nil {
					return err
				}
			}
		}
	case "object":
		obj, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: want object, got %T", at, v)
		}
		props, _ := s["properties"].(map[string]any)
		for _, r := range asStrings(s["required"]) {
			if _, ok := obj[r]; !ok {
				return fmt.Errorf("%s: missing required %s", at, r)
			}
		}
		for k, x := range obj {
			if p, ok := props[k].(map[string]any); ok {
				if err := conforms(x, p, schemas, at+"."+k); err != nil {
					return err
				}
				continue
			}
			switch extra := s["additionalProperties"].(type) {
			case bool:
				if !extra {
					return fmt.Errorf("%s: unexpected property %s", at, k)
				}
			case map[string]any:
				if err := conforms(x, extra, schemas, at+"."+k); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func asStrings(v any) []string {
	out := []string{}
	if list, ok := v.([]any); ok {
		for _, x := range list {
			out = append(out, fmt.Sprint(x))
		}
	}
	return out
}
