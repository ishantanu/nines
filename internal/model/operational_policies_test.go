package model_test

import (
	"encoding/json"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/examples"
	"github.com/ishantanu/nines/internal/model"
)

func TestOperationalPoliciesRoundTripAndValidate(t *testing.T) {
	d, err := model.ParseSLODefinition([]byte(examples.APIAvailability))
	if err != nil {
		t.Fatal(err)
	}
	old, _ := json.Marshal(d)
	if strings.Contains(string(old), "errorBudgetPolicy") || strings.Contains(string(old), `"quality"`) {
		t.Fatal("optional fields changed old manifests")
	}
	d.Spec.Quality = &v1.MeasurementQualityPolicy{MinEvents: 100, MaxSampleAgeSeconds: 120, CohortLabels: []string{"region", "tier"}}
	d.Spec.ErrorBudgetPolicy = &v1.ErrorBudgetPolicy{Owner: "oncall", RunbookURL: "https://example.test/runbook", ReviewBelow: .25}
	raw, _ := json.Marshal(d)
	parsed, err := model.ParseSLODefinition(raw)
	if err != nil || model.DefinitionDigest(model.DefinitionFromSLO(parsed.SLO())) != model.DefinitionDigest(d) {
		t.Fatal(parsed, err)
	}
	for _, label := range []string{"region) or vector(1)", "__name__", "le"} {
		d.Spec.Quality.CohortLabels = []string{label}
		if d.Validate() == nil {
			t.Fatal("invalid cohort label accepted", label)
		}
	}
	d.Spec.Quality.CohortLabels = []string{"region", "region"}
	if d.Validate() == nil {
		t.Fatal("duplicate cohort accepted")
	}
	d.Spec.Quality.CohortLabels = nil
	d.Spec.Quality.MaxSampleAgeSeconds = 1
	if d.Validate() == nil {
		t.Fatal("age below measurement interval accepted")
	}
	d.Spec.Quality.MaxSampleAgeSeconds = 120
	for _, url := range []string{"javascript:alert(1)", "https://user:secret@example.test/runbook", "file:///etc/passwd"} {
		d.Spec.ErrorBudgetPolicy.RunbookURL = url
		if d.Validate() == nil {
			t.Fatal("unsafe runbook accepted", url)
		}
	}
	d.Spec.ErrorBudgetPolicy.RunbookURL = "https://example.test/runbook"
	d.Spec.ErrorBudgetPolicy.OnExhausted = "proceed"
	if d.Validate() == nil {
		t.Fatal("exhaustion bypass accepted")
	}
}
