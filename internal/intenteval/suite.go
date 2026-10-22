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
type Result struct {
	WorkflowError        string `json:"workflowError,omitempty"`
	Case                 string `json:"case"`
	Provider             string `json:"provider"`
	Model                string `json:"model"`
	Passed               bool   `json:"passed"`
	Status               string `json:"status"`
	Failure              string `json:"failure,omitempty"`
	DurationMilliseconds int64  `json:"durationMilliseconds"`
}
type Report struct {
	SelectedCase   string    `json:"selectedCase,omitempty"`
	Suite          string    `json:"suite"`
	APIVersion     string    `json:"apiVersion"`
	Kind           string    `json:"kind"`
	Mode           string    `json:"mode"`
	SuiteSHA256    string    `json:"suiteSHA256"`
	GeneratedAt    time.Time `json:"generatedAt"`
	Complete       bool      `json:"complete"`
	RuntimeVersion string    `json:"runtimeVersion,omitempty"`
	Results        []Result  `json:"results"`
	Passed         int       `json:"passed"`
	Failed         int       `json:"failed"`
	Skipped        []string  `json:"skipped"`
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
	s.IntentDrafter = drafter
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
