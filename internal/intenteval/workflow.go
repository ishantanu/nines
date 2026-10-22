package intenteval

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/intentflow"
	"github.com/ishantanu/nines/internal/intentmodel"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

//go:embed testdata/workflow-cases.yaml
var workflowBytes []byte

type workflowCase struct {
	ID                  string `yaml:"id"`
	DraftCase           string `yaml:"draftCase"`
	Telemetry           string `yaml:"telemetry"`
	ExpectedState       string `yaml:"expectedState"`
	ExpectedCode        string `yaml:"expectedCode"`
	ExpectedUnknown     int    `yaml:"expectedUnknown"`
	ExpectedNoTraffic   int    `yaml:"expectedNoTraffic"`
	Apply               bool   `yaml:"apply"`
	LostResponse        bool   `yaml:"lostResponse"`
	UnmatchedPopulation bool   `yaml:"unmatchedPopulation"`
	JourneyConflict     bool   `yaml:"journeyConflict"`
	Interference        string `yaml:"interference"`
	Attack              bool   `yaml:"attack"`
	FixtureOnly         bool   `yaml:"fixtureOnly"`
}

func workflowCases(only ...string) ([]workflowCase, error) {
	var suite struct {
		APIVersion string         `yaml:"apiVersion"`
		Kind       string         `yaml:"kind"`
		Cases      []workflowCase `yaml:"cases"`
	}
	d := yaml.NewDecoder(strings.NewReader(string(workflowBytes)))
	d.KnownFields(true)
	if err := d.Decode(&suite); err != nil {
		return nil, errors.New("invalid workflow evaluation suite")
	}
	if d.Decode(new(any)) != io.EOF || suite.APIVersion != v1.APIVersion || suite.Kind != "IntentWorkflowEvaluationSuite" || len(suite.Cases) == 0 {
		return nil, errors.New("invalid workflow evaluation suite")
	}
	seen := map[string]bool{}
	for _, c := range suite.Cases {
		if c.ID == "" || seen[c.ID] || c.DraftCase == "" || c.ExpectedState == "" {
			return nil, errors.New("invalid workflow evaluation case")
		}
		seen[c.ID] = true
	}
	if len(only) > 0 && only[0] != "" {
		for _, c := range suite.Cases {
			if c.ID == only[0] {
				return []workflowCase{c}, nil
			}
		}
		return nil, errors.New("unknown workflow evaluation case")
	}
	return suite.Cases, nil
}

type workflowTelemetry struct {
	*testtelemetry.Source
	mode string
}

func (s *workflowTelemetry) Discover(ctx context.Context, metric string, labels map[string]string, at time.Time) (v1.TelemetryDiscovery, error) {
	if s.mode == "missing" {
		return v1.TelemetryDiscovery{Metric: metric, From: at.Add(-time.Minute), To: at}, nil
	}
	return s.Source.Discover(ctx, metric, labels, at)
}
func (s *workflowTelemetry) CollectWindowRange(_ context.Context, slo model.SLO, from, to time.Time, resolution int64) ([]model.WindowObservation, error) {
	values := []model.WindowObservation{}
	for at := from.Add(time.Duration(resolution) * time.Second); !at.After(to); at = at.Add(time.Duration(resolution) * time.Second) {
		for _, window := range slo.Windows() {
			good, total, coverage := 999.0, 1000.0, 1.0
			if s.mode == "sparse" {
				coverage = .5
			}
			if s.mode == "no_traffic" {
				good, total = 0, 0
			}
			values = append(values, model.WindowObservation{Time: at, WindowSeconds: window, Revision: model.ConfigRevision(slo), Good: good, Total: total, Coverage: coverage})
		}
	}
	return values, nil
}

// EvaluateWorkflow runs the actual bounded model/tool worker, explicit human
// selection, deterministic backtest and optional reviewed SLO/CUJ transaction.
// The provider receives synthetic scope only; no real server is contacted.
func EvaluateWorkflow(ctx context.Context, c workflowCase, draft Case, provider string, client *intentmodel.Client) Result {
	start := time.Now()
	result := Result{Case: c.ID, Provider: provider, Model: client.ModelID()}
	fail := func(code string) Result {
		result.Failure = code
		result.DurationMilliseconds = time.Since(start).Milliseconds()
		return result
	}
	platform, source, err := testtelemetry.New()
	if err != nil {
		return fail("fixture_setup_failed")
	}
	platform.TenantID = "evaluation"
	telemetry := &workflowTelemetry{Source: source, mode: c.Telemetry}
	platform.Collector = telemetry
	platform.IntentDrafter = client
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	platform.Now = func() time.Time { return now }
	latency := testtelemetry.Binding()
	latency.Metadata.Name = "latency"
	latency.Spec.Measurement = "latency"
	latency.Spec.Metric = "http_duration_seconds"
	latency.Spec.SuccessLabels = nil
	threshold := .3
	latency.Spec.ThresholdSeconds = &threshold
	if _, err := platform.ApplyDefinition(ctx, model.Definition{Binding: &latency}, 0); err != nil {
		return fail("fixture_setup_failed")
	}
	flow := intentflow.New(platform, intentflow.NewMemory())
	defer flow.Repository.Close()
	flow.Model = client
	flow.Authorize = func(context.Context, v1.InvestigationActor) error { return nil }
	owner := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "evaluation", Role: "owner", Subject: "fixture-owner", Kind: "user"})
	input := v1.CreateIntentConversation{ID: "workflow-evaluation", Title: "Synthetic intent evaluation", Text: draft.Text, Scope: v1.IntentAuthoringScope{Metadata: v1.Metadata{Name: "evaluation-slo", Project: "test"}, Service: "api", Source: "primary", Population: map[string]string{"service": "api"}, NoDataPolicy: "unknown", Journey: &v1.IntentJourney{Name: "checkout", Step: "request", ExpectedRevision: 1, Required: true}}}
	if c.UnmatchedPopulation {
		input.Scope.Population = map[string]string{"service": "unmatched"}
	}
	if c.JourneyConflict {
		input.Scope.Metadata.Name = "api"
	}
	baseline, err := platform.Store.List(ctx)
	if err != nil {
		return fail("fixture_setup_failed")
	}
	before := model.DefinitionDigest(baseline)
	value, err := flow.Create(owner, input)
	if err != nil {
		return fail("conversation_create_failed")
	}
	value, err = flow.Start(owner, value.ID, v1.CreateIntentFlowRun{ID: "discover", ExpectedVersion: value.Version, Mode: "model", ModelConsent: true})
	if err != nil {
		return fail("conversation_start_failed")
	}
	value, err = settleWorkflow(owner, flow, value.ID)
	if err != nil {
		return fail("worker_execution_failed")
	}
	if value.State == "needs_selection" {
		// This is the harness's explicit human role, never a model-selected binding.
		binding := "requests"
		if draft.ID == "latency" {
			binding = "latency"
		}
		value, err = flow.Select(owner, value.ID, v1.ConfirmIntentBinding{ExpectedVersion: value.Version, IntentDigest: value.Plan.IntentDigest, Binding: v1.IntentSelection{Name: binding, ExpectedRevision: 1}, Confirm: true})
		if err != nil {
			return fail("human_selection_failed")
		}
		value, err = flow.Start(owner, value.ID, v1.CreateIntentFlowRun{ID: "prepare", ExpectedVersion: value.Version, Mode: "model", ModelConsent: true, Backtest: &v1.IntentFlowBacktestWindow{From: now.Add(-12 * time.Minute), To: now.Add(-2 * time.Minute), ResolutionSeconds: 300}})
		if err != nil {
			return fail("conversation_start_failed")
		}
		value, err = settleWorkflow(owner, flow, value.ID)
		if err != nil {
			return fail("worker_execution_failed")
		}
	}
	result.Status = value.State
	if len(value.Runs) > 0 {
		result.WorkflowError = value.Runs[len(value.Runs)-1].ErrorCode
	}
	if ctx.Err() != nil {
		return fail("cancelled")
	}
	if value.State != c.ExpectedState {
		return fail("unexpected_workflow_state")
	}
	if len(value.Runs) == 0 || c.ExpectedCode != "" && value.Runs[len(value.Runs)-1].ErrorCode != c.ExpectedCode {
		return fail("unexpected_workflow_error")
	}
	public, _ := json.Marshal(value)
	if strings.Contains(string(public), "fixture-evaluation-key") || strings.Contains(string(public), "native-private-call") {
		return fail("private_state_exposed")
	}
	current, err := platform.Store.List(ctx)
	if err != nil || model.DefinitionDigest(current) != before {
		return fail("preparation_wrote_resources")
	}
	if model.DefinitionDigest(value.Scope) != model.DefinitionDigest(input.Scope) {
		return fail("scope_changed")
	}
	if value.State == "ready" {
		p, b := value.Plan, value.Backtest
		if p == nil || p.Compiled == nil || value.Selection == nil || b == nil || b.IntentDigest != p.IntentDigest || b.ManifestDigest != p.Compiled.Digest || b.SourceDigest != p.SourceDigest || b.From != now.Add(-12*time.Minute) || b.To != now.Add(-2*time.Minute) {
			return fail("missing_definition_or_time_provenance")
		}
		if b.UnknownCutoffs != c.ExpectedUnknown || b.NoTrafficCutoffs != c.ExpectedNoTraffic {
			return fail("unexpected_traffic_classification")
		}
		if p.Compiled.Definition.Spec.ObservationPoint == "" || p.Compiled.Evidence.Fingerprint == "" || p.Compiled.Evidence.Source != "primary" || len(p.Warnings) == 0 {
			return fail("missing_measurement_evidence")
		}
		if c.Apply || c.Interference != "" {
			if err := reviewAndApply(owner, flow, value, c); err != nil {
				return fail(err.Error())
			}
		}
	}
	result.Passed = true
	result.DurationMilliseconds = time.Since(start).Milliseconds()
	return result
}
func settleWorkflow(ctx context.Context, flow *intentflow.Service, id string) (v1.IntentConversation, error) {
	for n := 0; n < 20; n++ {
		value, err := flow.Read(ctx, id)
		if err != nil {
			return value, err
		}
		if value.State != "queued" && value.State != "running" {
			return value, nil
		}
		found, err := flow.WorkOne(ctx)
		if err != nil {
			return value, err
		}
		if !found {
			return value, errors.New("worker not available")
		}
	}
	return v1.IntentConversation{}, errors.New("worker evaluation limit")
}
func reviewAndApply(ctx context.Context, flow *intentflow.Service, value v1.IntentConversation, c workflowCase) error {
	failed := func(code string) error { return errors.New(code) }
	value, err := flow.PrepareChangeSet(ctx, value.ID, value.Version)
	if err != nil {
		return failed("change_set_prepare_failed")
	}
	agent := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "evaluation", Role: "agent", Subject: "fixture-agent", Kind: "api_token"})
	request := v1.ReviewIntentChangeSet{ExpectedVersion: value.Version, Digest: value.ChangeSet.Digest, Confirm: true}
	if _, err := flow.ApproveChangeSet(agent, value.ID, request); !errors.Is(err, intentflow.ErrForbidden) {
		return failed("agent_approved_change_set")
	}
	if _, err := flow.ApplyChangeSet(ctx, value.ID, request); err == nil {
		return failed("unapproved_apply_succeeded")
	}
	value, err = flow.ApproveChangeSet(ctx, value.ID, request)
	if err != nil {
		return failed("change_set_approval_failed")
	}
	request.ExpectedVersion = value.Version
	if _, err := flow.ApplyChangeSet(agent, value.ID, request); !errors.Is(err, intentflow.ErrForbidden) {
		return failed("agent_applied_change_set")
	}
	if c.Interference != "" {
		if c.Interference == "binding" {
			b := testtelemetry.Binding()
			b.Spec.DisplayName = "Changed contract"
			if _, err := flow.Platform.ApplyDefinition(ctx, model.Definition{Binding: &b}, 1); err != nil {
				return failed("fixture_setup_failed")
			}
		}
		if c.Interference == "source" {
			flow.Platform.Collector.(*workflowTelemetry).Route = "changed-mimir-organization"
		}
		if _, err := flow.ApplyChangeSet(ctx, value.ID, request); !errors.Is(err, intentflow.ErrConflict) && !errors.Is(err, model.ErrRevisionConflict) {
			return failed("stale_approval_applied")
		}
		if _, err := flow.Platform.Store.Read(ctx, value.Scope.Metadata.Name); !errors.Is(err, model.ErrNotFound) {
			return failed("stale_approval_wrote_resource")
		}
		return nil
	}
	if c.LostResponse {
		mux := http.NewServeMux()
		intentflow.Mount(mux, flow)
		var dropped atomic.Bool
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, r.WithContext(ctx))
			if recorder.Code == 200 && !dropped.Swap(true) {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					_ = conn.Close()
				}
				return
			}
			for key, values := range recorder.Header() {
				for _, v := range values {
					w.Header().Add(key, v)
				}
			}
			w.WriteHeader(recorder.Code)
			_, _ = w.Write(recorder.Body.Bytes())
		}))
		defer server.Close()
		body, _ := json.Marshal(request)
		response, err := http.Post(server.URL+"/api/intent-conversations/"+value.ID+"/change-set/apply", "application/json", strings.NewReader(string(body)))
		if response != nil {
			_ = response.Body.Close()
		}
		if err == nil || !dropped.Load() {
			return failed("lost_response_not_simulated")
		}
		value, err = flow.Read(ctx, value.ID)
		if err != nil || value.ChangeSet.State != "applied" {
			return failed("ambiguous_apply_readback_failed")
		}
	} else {
		value, err = flow.ApplyChangeSet(ctx, value.ID, request)
		if err != nil {
			return failed("reviewed_apply_failed")
		}
	}
	if value.ChangeSet == nil || value.ChangeSet.State != "applied" || len(value.ChangeSet.Applied) != 2 || value.ChangeSet.AppliedAt == nil {
		return failed("missing_apply_receipt")
	}
	retry, err := flow.ApplyChangeSet(ctx, value.ID, request)
	if err != nil || model.DefinitionDigest(retry.ChangeSet) != model.DefinitionDigest(value.ChangeSet) {
		return failed("apply_retry_changed_receipt")
	}
	verification, err := flow.VerifyChangeSet(ctx, value.ID)
	if err != nil || verification.SourceState != "matches" || len(verification.Resources) != 2 {
		return failed("apply_verification_failed")
	}
	for _, resource := range verification.Resources {
		if resource.State != "matches" || resource.Health != "unknown" {
			return failed("apply_claimed_unobserved_health")
		}
	}
	row, err := flow.Platform.Store.Read(ctx, value.Scope.Metadata.Name)
	if err != nil || model.ConfigRevision(row.SLO) != 1 {
		return failed("apply_duplicated_slo")
	}
	history, err := flow.Platform.Journeys.JourneyHistory(ctx, "checkout")
	if err != nil || len(history) != 2 {
		return failed("apply_duplicated_cuj")
	}
	return nil
}
