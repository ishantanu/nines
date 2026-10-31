// Package intenteval evaluates intent drafting and workflows using synthetic fixtures.
// It is used only by the evaluation harness, not by the production server.
package intenteval

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

//go:embed testdata/draft-cases.yaml
var suiteBytes []byte

type expectation struct {
	Status           string   `yaml:"status"`
	Outcome          string   `yaml:"outcome"`
	Target           float64  `yaml:"target"`
	WindowSeconds    int64    `yaml:"windowSeconds"`
	ThresholdSeconds *float64 `yaml:"thresholdSeconds"`
}
type Case struct {
	ID          string      `yaml:"id"`
	Text        string      `yaml:"text"`
	FixtureGoal string      `yaml:"fixtureGoal"`
	FixtureOnly bool        `yaml:"fixtureOnly"`
	Expected    expectation `yaml:"expected"`
}
type suite struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Cases      []Case `yaml:"cases"`
}
type EvaluationCheck struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Passed   bool   `json:"passed"`
	Failure  string `json:"failure,omitempty"`
}
type AcceptanceSummary struct {
	Status                string `json:"status"`
	RequiredCases         int    `json:"requiredCases"`
	ExecutedCases         int    `json:"executedCases"`
	ModelChecksPassed     int    `json:"modelChecksPassed"`
	ModelChecksFailed     int    `json:"modelChecksFailed"`
	GuardrailChecksPassed int    `json:"guardrailChecksPassed"`
	GuardrailChecksFailed int    `json:"guardrailChecksFailed"`
}

// DraftDiagnostic contains only fixed categories from synthetic evaluation.
// It never includes user text, extracted values, model responses or provider errors.
type DraftDiagnostic struct {
	ModelState         string   `json:"modelState"`
	Outcome            string   `json:"outcome,omitempty"`
	ModelClarification bool     `json:"modelClarification"`
	TargetEvidence     string   `json:"targetEvidence,omitempty"`
	WindowEvidence     string   `json:"windowEvidence,omitempty"`
	ThresholdEvidence  string   `json:"thresholdEvidence,omitempty"`
	ValidationReasons  []string `json:"validationReasons,omitempty"`
}

type trackedDrafter struct {
	service.IntentDrafter
	called bool
	goal   v1.ModelGoal
	err    error
}

func (d *trackedDrafter) Draft(ctx context.Context, text string) (v1.ModelGoal, error) {
	d.called = true
	d.goal, d.err = d.IntentDrafter.Draft(ctx, text)
	return d.goal, d.err
}
func (d *trackedDrafter) diagnostic(input string, clarifications []string) *DraftDiagnostic {
	if !d.called {
		return &DraftDiagnostic{ModelState: "not_called"}
	}
	if d.err != nil {
		return &DraftDiagnostic{ModelState: "unavailable"}
	}
	out := &DraftDiagnostic{ModelState: "returned", ModelClarification: len(d.goal.Clarifications) > 0}
	switch d.goal.Outcome {
	case "availability", "latency", "compound", "unsupported":
		out.Outcome = d.goal.Outcome
	default:
		out.Outcome = "invalid"
	}
	evidence := func(value *string) string {
		if value == nil || strings.TrimSpace(*value) == "" {
			return "missing"
		}
		if strings.Contains(input, *value) {
			return "appears_in_input"
		}
		return "not_in_input"
	}
	out.TargetEvidence = evidence(d.goal.TargetText)
	out.WindowEvidence = evidence(d.goal.WindowText)
	out.ThresholdEvidence = evidence(d.goal.ThresholdText)
	modelCount := len(d.goal.Clarifications)
	if modelCount > len(clarifications) {
		modelCount = len(clarifications)
	}
	for _, message := range clarifications[modelCount:] {
		var reason string
		switch message {
		case "State the target as an explicit percentage, for example 99.9%.":
			reason = "target_invalid"
		case "State an explicit rolling duration, for example 28 days, rather than a calendar month.":
			reason = "window_invalid"
		case "State the maximum attempt duration, for example 300 milliseconds.":
			reason = "threshold_invalid"
		case "Availability and a duration condition need a joint measurement contract; choose one outcome.", "This goal requires a separate instrumentation contract. Choose availability or latency; a joint condition cannot be inferred from two ratios.":
			reason = "unsupported_or_joint_outcome"
		default:
			reason = "other_validation"
		}
		out.ValidationReasons = append(out.ValidationReasons, reason)
	}
	return out
}

type Result struct {
	DraftDiagnostic      *DraftDiagnostic  `json:"draftDiagnostic,omitempty"`
	Checks               []EvaluationCheck `json:"checks,omitempty"`
	ModelCalls           int               `json:"modelCalls,omitempty"`
	ToolAttempts         int               `json:"toolAttempts,omitempty"`
	WorkflowError        string            `json:"workflowError,omitempty"`
	Case                 string            `json:"case"`
	Provider             string            `json:"provider"`
	Model                string            `json:"model"`
	Passed               bool              `json:"passed"`
	Status               string            `json:"status"`
	Failure              string            `json:"failure,omitempty"`
	DurationMilliseconds int64             `json:"durationMilliseconds"`
}
type Report struct {
	Acceptance     *AcceptanceSummary   `json:"acceptance,omitempty"`
	CatalogDigest  string               `json:"catalogDigest,omitempty"`
	ModelRoute     *v1.ReviewModelRoute `json:"modelRoute,omitempty"`
	Skill          *v1.ReviewSkillRef   `json:"skill,omitempty"`
	SelectedCase   string               `json:"selectedCase,omitempty"`
	Suite          string               `json:"suite"`
	APIVersion     string               `json:"apiVersion"`
	Kind           string               `json:"kind"`
	Mode           string               `json:"mode"`
	SuiteSHA256    string               `json:"suiteSHA256"`
	GeneratedAt    time.Time            `json:"generatedAt"`
	Complete       bool                 `json:"complete"`
	RuntimeVersion string               `json:"runtimeVersion,omitempty"`
	Results        []Result             `json:"results"`
	Passed         int                  `json:"passed"`
	Failed         int                  `json:"failed"`
	Skipped        []string             `json:"skipped"`
}

func Cases() ([]Case, error) {
	var s suite
	d := yaml.NewDecoder(strings.NewReader(string(suiteBytes)))
	d.KnownFields(true)
	if err := d.Decode(&s); err != nil {
		return nil, err
	}
	var extra any
	if d.Decode(&extra) != io.EOF || s.APIVersion != v1.APIVersion || s.Kind != "IntentDraftEvaluationSuite" || len(s.Cases) == 0 {
		return nil, errors.New("invalid intent evaluation suite")
	}
	seen := map[string]bool{}
	for _, c := range s.Cases {
		if c.ID == "" || seen[c.ID] || c.Text == "" || !json.Valid([]byte(c.FixtureGoal)) || (c.Expected.Status != "draft" && c.Expected.Status != "needs_clarification" && c.Expected.Status != "unavailable") {
			return nil, errors.New("invalid intent evaluation case")
		}
		seen[c.ID] = true
	}
	return s.Cases, nil
}
func NewReport(mode, runtimeVersion string) Report {
	digest := sha256.Sum256(suiteBytes)
	return Report{APIVersion: v1.APIVersion, Kind: "IntentEvaluationReport", Mode: mode, Suite: "draft", SuiteSHA256: hex.EncodeToString(digest[:]), GeneratedAt: time.Now().UTC(), RuntimeVersion: runtimeVersion, Results: []Result{}, Skipped: []string{}}
}
func (r *Report) Add(result Result) {
	r.Results = append(r.Results, result)
	if result.Passed {
		r.Passed++
	} else {
		r.Failed++
	}
}

// Evaluate runs the production drafting service. Reports exclude raw responses,
// model clarifications, endpoints and errors, which can contain private values.
func Evaluate(ctx context.Context, c Case, provider string, drafter service.IntentDrafter) Result {
	started := time.Now()
	r := Result{Case: c.ID, Provider: provider, Model: drafter.ModelID()}
	s, _, err := testtelemetry.New()
	if err != nil {
		r.Failure = "fixture_setup_failed"
		return r
	}
	s.TenantID = "evaluation"
	tracked := &trackedDrafter{IntentDrafter: drafter}
	s.IntentDrafter = tracked
	input := v1.DraftIntentInput{Text: c.Text, Metadata: v1.Metadata{Name: "evaluation-draft", Project: "test"}, Service: "api", Source: "primary", Population: map[string]string{"service": "api"}, NoDataPolicy: "unknown"}
	out, err := s.DraftIntent(ctx, input)
	r.DurationMilliseconds = time.Since(started).Milliseconds()
	if err != nil {
		r.Status = "unavailable"
		if !errors.Is(err, service.ErrIntentModel) {
			r.Failure = "unexpected_error"
			return r
		}
	} else {
		r.Status = out.Status
	}
	if ctx.Err() != nil {
		r.Failure = "cancelled"
		return r
	}
	if _, e := s.Store.Read(ctx, input.Metadata.Name); !errors.Is(e, model.ErrNotFound) {
		r.Failure = "unexpected_resource_write"
		return r
	}
	if r.Status != c.Expected.Status {
		r.DraftDiagnostic = tracked.diagnostic(c.Text, out.Clarifications)
		r.Failure = "unexpected_status"
		return r
	}
	if out.Intent != nil {
		actual := out.Intent
		if actual.Metadata != input.Metadata || actual.Spec.Service != input.Service || actual.Spec.Source != input.Source || len(actual.Spec.Population) != 1 || actual.Spec.Population["service"] != "api" || actual.Spec.NoDataPolicy != "unknown" {
			r.Failure = "scope_changed"
			return r
		}
		if r.Status != "draft" || actual.Spec.Outcome != c.Expected.Outcome || actual.Spec.Target != c.Expected.Target || actual.Spec.WindowSeconds != c.Expected.WindowSeconds {
			r.Failure = "unexpected_definition"
			return r
		}
		if (actual.Spec.ThresholdSeconds == nil) != (c.Expected.ThresholdSeconds == nil) || actual.Spec.ThresholdSeconds != nil && *actual.Spec.ThresholdSeconds != *c.Expected.ThresholdSeconds {
			r.Failure = "unexpected_threshold"
			return r
		}
	} else if r.Status == "draft" {
		r.Failure = "missing_definition"
		return r
	}
	if r.Status == "needs_clarification" && (out.Intent != nil || len(out.Clarifications) == 0) {
		r.Failure = "missing_clarification"
		return r
	}
	r.Passed = true
	return r
}
func Summary(r Report) error {
	if r.Suite == "draft" && r.SelectedCase != "" {
		return errors.New("filtered report is not full acceptance")
	}
	if !r.Complete {
		return errors.New("intent evaluation incomplete")
	}
	if len(r.Results) == 0 {
		return errors.New("intent evaluation ran no cases")
	}
	if r.Failed > 0 {
		return fmt.Errorf("intent evaluations failed: %d of %d", r.Failed, len(r.Results))
	}
	return nil
}
