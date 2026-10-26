package intentflow

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	rm "github.com/ishantanu/nines/internal/reviewmodel"
	"github.com/ishantanu/nines/internal/slotemplates"
	"github.com/ishantanu/nines/internal/tenancy"
)

func templateMeasurement() *v1.MeasurementSpec {
	return &v1.MeasurementSpec{Type: "availability", Metric: "http_requests_total", Labels: map[string]string{"service": "api"}, SuccessLabels: map[string]string{"status": "200"}, ScrapeIntervalSeconds: 10, ObservationPoint: "Completed API requests, after retries"}
}
func templateCall(t *testing.T, s *Service, id string, values any) rm.Response {
	t.Helper()
	template, err := s.Templates.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	return response("render_slo_template", map[string]any{"id": id, "expectedTemplateDigest": template.Digest, "values": values})
}
func extensionRegistry(t *testing.T) *slotemplates.Registry {
	t.Helper()
	data, err := os.ReadFile("../../examples/extensions/example-agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(t.TempDir(), "extensions.lock.yaml")
	if _, err = slotemplates.Install(lock, data, slotemplates.SHA256(data), false); err != nil {
		t.Fatal(err)
	}
	registry, err := slotemplates.Load(lock)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func TestTemplateWorkflowHumanReviewAndDurableProvenance(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, dsn := fixture(t, pg)
			in := input()
			in.Scope.Journey = nil
			in.Goal.Spec.Journey = nil
			in.Goal.Spec.Policies = []v1.BurnRatePolicy{{Name: "user-policy", Severity: "warning", ShortWindowSeconds: 60, LongWindowSeconds: 300, BurnRate: 6, MinCoverage: 1}}
			v, err := s.Create(ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			s.Model = &scriptedModel{route: "templates", turn: func(_ context.Context, request rm.Request) (rm.Response, error) {
				last := request.Messages[len(request.Messages)-1]
				switch last.Name {
				case "":
					return response("list_slo_templates", map[string]string{"category": "request-serving"}), nil
				case "list_slo_templates":
					return templateCall(t, s, "request-availability", map[string]any{"measurement": templateMeasurement()}), nil
				case "render_slo_template":
					return response("find_measurements", map[string]any{}), nil
				default:
					if strings.Contains(string(last.Result), `"yaml":`) {
						t.Fatal("executable YAML sent to model")
					}
					return response("ask_user", map[string]string{"question": "Confirm the population, completion observation point, retries and status=200 success criterion."}), nil
				}
			}}
			v, err = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "template-draft", ExpectedVersion: v.Version, Mode: "model", ModelConsent: true})
			if err != nil {
				t.Fatal(err)
			}
			work(t, s, ctx, 8)
			v, err = s.Read(ctx, v.ID)
			if err != nil || v.State != "needs_selection" || v.Template == nil || v.Selection != nil || v.Plan == nil || v.Plan.Compiled == nil {
				t.Fatal(v, err)
			}
			if v.Goal.Spec.Target != in.Goal.Spec.Target || v.Goal.Spec.WindowSeconds != in.Goal.Spec.WindowSeconds || hash(v.Goal.Spec.Policies) != hash(in.Goal.Spec.Policies) || v.Goal.Spec.Lifecycle != v1.LifecycleExperimental || !sameScope(in.Scope, *v.Goal) {
				t.Fatal("recipe changed commitments", v.Goal)
			}
			if v.Template.IntentDigest != hash(*v.Goal) || v.Template.TemplateID != "request-availability" || v.Template.CatalogDigest != v.Runs[0].TemplateCatalogDigest || v.Runs[0].ModelCalls != 4 {
				t.Fatal("lost provenance", v.Template)
			}
			if _, err = s.Platform.Store.Read(ctx, in.Scope.Metadata.Name); !errors.Is(err, model.ErrNotFound) {
				t.Fatal("preparation wrote SLO", err)
			}
			if pg {
				s.Repository.Close()
				s.Repository, err = NewPostgres(ctx, dsn)
				if err != nil {
					t.Fatal(err)
				}
				defer s.Repository.Close()
				restored, e := s.Read(ctx, v.ID)
				if e != nil || hash(restored.Template) != hash(v.Template) {
					t.Fatal("restart lost template", e)
				}
			}
			confirmation := v1.ConfirmIntentBinding{ExpectedVersion: v.Version, IntentDigest: v.Plan.IntentDigest, MeasurementDigest: v.Plan.Compiled.MeasurementDigest, Confirm: true}
			agent := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "alpha", Role: "agent", Subject: "agent", Kind: "api_token"})
			if _, err = s.Select(agent, v.ID, confirmation); !errors.Is(err, ErrForbidden) {
				t.Fatal("agent confirmed recipe", err)
			}
			v, err = s.Select(ctx, v.ID, confirmation)
			if err != nil {
				t.Fatal(err)
			}
			v, err = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "template-compile", ExpectedVersion: v.Version, Mode: "guided"})
			if err != nil {
				t.Fatal(err)
			}
			work(t, s, ctx, 2)
			v, err = s.Read(ctx, v.ID)
			if err != nil || v.State != "ready" {
				t.Fatal(v.State, err)
			}
			v, err = s.PrepareChangeSet(ctx, v.ID, v.Version)
			if err != nil || v.ChangeSet.Recommendation.Template == nil || hash(v.ChangeSet.Recommendation.Template) != hash(v.Template) {
				t.Fatal("proposal omitted template", err)
			}
			audit, err := s.Audit(ctx, v.ID)
			if err != nil || audit.Integrity != "verified" {
				t.Fatal(err, audit)
			}
			found := false
			for _, event := range audit.Events {
				if event.Action == "template.selected" {
					found = event.EvidenceDigest == hash(v.Template)
				}
			}
			if !found {
				t.Fatal("template choice missing from audit journal")
			}
			v, err = s.Append(ctx, v.ID, v1.AppendIntentConversation{ID: "manual-edit", ExpectedVersion: v.Version, Text: "Review a manual measurement", Goal: v.Goal})
			if err != nil || v.Template != nil || v.Selection != nil || v.Plan != nil {
				t.Fatal("edit retained template authority", err)
			}
		})
	}
}

func TestTemplateSelectionRejectsScopeChangesAndUnsafeArguments(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	s.Model = &scriptedModel{route: "fixture"}
	v, err := s.Create(ctx, input())
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "draft", ExpectedVersion: v.Version, Mode: "model", ModelConsent: true})
	if err != nil {
		t.Fatal(err)
	}
	base, err := s.Repository.Read(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	template, _ := s.Templates.Get("request-availability")
	for name, values := range map[string]any{
		"tenant scope":        map[string]any{"source": "other", "measurement": templateMeasurement()},
		"target":              map[string]any{"target": 0.95, "measurement": templateMeasurement()},
		"window":              map[string]any{"windowSeconds": 60, "measurement": templateMeasurement()},
		"missing measurement": map[string]any{},
	} {
		t.Run(name, func(t *testing.T) {
			r := clone(base)
			_, err := s.execute(ctx, &r, templateCall(t, s, template.ID, values).Calls[0])
			if err == nil || r.Session.Template != nil {
				t.Fatal("unsafe values accepted")
			}
		})
	}
	for _, field := range []string{"labels", "type"} {
		r := clone(base)
		measurement := templateMeasurement()
		if field == "labels" {
			measurement.Labels["environment"] = "other"
		} else {
			measurement.Type = "latency"
			threshold := 0.3
			measurement.ThresholdSeconds = &threshold
			measurement.SuccessLabels = nil
		}
		if _, err = s.execute(ctx, &r, templateCall(t, s, template.ID, map[string]any{"measurement": measurement}).Calls[0]); err == nil {
			t.Fatal("incompatible measurement accepted", field)
		}
	}
	for _, raw := range []string{`null`, `{"measurement":{"type":"availability","type":"latency"}}`, `{"inputs":{"population":{"service":"api","service":"other"}}}`} {
		if _, err = parseTemplateArguments(json.RawMessage(raw)); err == nil {
			t.Fatal("ambiguous arguments accepted", raw)
		}
	}
	r := clone(base)
	r.Session.Selection = &v1.IntentBindingConfirmation{}
	if _, err = s.execute(ctx, &r, templateCall(t, s, template.ID, map[string]any{"measurement": templateMeasurement()}).Calls[0]); err == nil {
		t.Fatal("overwrote confirmed measurement")
	}
	r = clone(base)
	r.Session.Goal.Spec.Measurement = templateMeasurement()
	if _, err = s.execute(ctx, &r, templateCall(t, s, template.ID, map[string]any{"measurement": templateMeasurement()}).Calls[0]); err == nil {
		t.Fatal("overwrote supplied measurement")
	}
	call := templateCall(t, s, template.ID, map[string]any{"measurement": templateMeasurement()}).Calls[0]
	call.Arguments = json.RawMessage(strings.Replace(string(call.Arguments), template.Digest, strings.Repeat("0", 64), 1))
	r = clone(base)
	if _, err = s.execute(ctx, &r, call); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("stale template accepted", err)
	}
}

func TestTenantExtensionsAndCatalogChangesFenceModelRuns(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	s.Templates = extensionRegistry(t)
	s.Model = &scriptedModel{route: "fixture", turn: func(context.Context, rm.Request) (rm.Response, error) {
		return response("list_slo_templates", map[string]any{}), nil
	}}
	in := input()
	in.Scope.Journey = nil
	in.Goal.Spec.Journey = nil
	in.Scope.Population = map[string]string{"service": "api", "rubric": "support-v1", "evaluator": "rules-v1"}
	in.Goal.Spec.Population = in.Scope.Population
	v, err := s.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "template", ExpectedVersion: v.Version, Mode: "model", ModelConsent: true})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Repository.Read(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]any{"inputs": map[string]any{"metric": "agent_evaluations_total", "population": in.Scope.Population}}
	if _, err = s.execute(ctx, &r, templateCall(t, s, "example-agent--evaluation-pass", values).Calls[0]); err != nil {
		t.Fatal(err)
	}
	if r.Session.Template.Extension == nil || r.Session.Goal.Spec.Target != .999 || r.Session.Goal.Spec.WindowSeconds != 3600 || len(r.Session.Goal.Spec.Policies) != 0 || r.Session.Goal.Spec.Quality != nil {
		t.Fatal("extension preset replaced reviewed goal", r.Session.Goal)
	}
	other, _, _ := fixture(t, false)
	other.Model = s.Model
	if _, err = other.Templates.Get("example-agent--evaluation-pass"); !errors.Is(err, model.ErrNotFound) {
		t.Fatal("private extension visible in another tenant", err)
	}
	s.Templates = nil
	work(t, s, ctx, 1)
	v, err = s.Read(ctx, v.ID)
	if err != nil || v.State != "failed" || v.Runs[0].ErrorCode != "template_catalog_changed" {
		t.Fatal(v, err)
	}
}

func TestTemplateDiscoveryPagesAreBoundedAndComplete(t *testing.T) {
	s, _, _ := fixture(t, false)
	s.Templates = extensionRegistry(t)
	after := ""
	seen := map[string]bool{}
	for page := 0; page < 10; page++ {
		raw, _ := json.Marshal(map[string]string{"after": after})
		result, err := s.discoverTemplates(raw)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(result)
		if len(body) > 11<<10 || len(result.Templates) > 4 {
			t.Fatal("unbounded discovery", len(body))
		}
		for _, item := range result.Templates {
			if seen[item.ID] {
				t.Fatal("repeated recipe")
			}
			seen[item.ID] = true
		}
		if result.NextAfter == "" {
			break
		}
		after = result.NextAfter
	}
	if len(seen) != 16 {
		t.Fatal("incomplete catalog", len(seen))
	}
	if _, err := s.discoverTemplates(json.RawMessage(`{"category":"agent","after":"request-availability"}`)); err == nil {
		t.Fatal("cross-category cursor accepted")
	}
}

func TestCatalogChangeDuringModelCallDoesNotPublishPendingTool(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	registry := extensionRegistry(t)
	s.Model = &scriptedModel{route: "fixture", turn: func(context.Context, rm.Request) (rm.Response, error) {
		s.Templates = registry
		return response("list_slo_templates", map[string]any{}), nil
	}}
	v, err := s.Create(ctx, input())
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "catalog-change", ExpectedVersion: v.Version, Mode: "model", ModelConsent: true})
	if err != nil {
		t.Fatal(err)
	}
	work(t, s, ctx, 1)
	r, err := s.Repository.Read(ctx, v.ID)
	if err != nil || r.Pending != nil || len(r.Session.Runs[0].Steps) != 0 || r.Session.Runs[0].ErrorCode != "template_catalog_changed" {
		t.Fatal("stale model output published", r.Session.State, err)
	}
}

func TestOversizedTemplateSchemaRequiresManualInputs(t *testing.T) {
	s, _, _ := fixture(t, false)
	template, _ := s.Templates.Get("request-availability")
	template.Requirements = []string{strings.Repeat("x", 7<<10)}
	candidate := candidateForTemplate(template)
	if !candidate.RequiresManualInputs || candidate.Inputs != nil || candidate.ExampleMeasurement != nil || candidate.Requirements != nil {
		t.Fatal("oversized recipe silently truncated")
	}
}

func TestTemplateDraftPreservesExplicitCommittedLifecycleInItsWarnings(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	s.Model = &scriptedModel{route: "fixture"}
	in := input()
	in.Goal.Spec.Lifecycle = v1.LifecycleCommitted
	v, err := s.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "draft", ExpectedVersion: v.Version, Mode: "model", ModelConsent: true})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Repository.Read(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.execute(ctx, &r, templateCall(t, s, "request-availability", map[string]any{"measurement": templateMeasurement()}).Calls[0])
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	if r.Session.Goal.Spec.Lifecycle != v1.LifecycleCommitted || strings.Contains(string(raw), "experimental") {
		t.Fatal("misleading lifecycle warning", string(raw))
	}
}
