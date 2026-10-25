package operator

import (
	"encoding/json"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
)

func TestOperatorLifecycleRoundTripAndRulePromotion(t *testing.T) {
	d := definition()
	d.Spec.Lifecycle = v1.LifecycleAspirational
	d.Spec.Policy.MinEvents = 100
	d.Spec.Quality = &v1.MeasurementQualityPolicy{MinEvents: 100, CohortLabels: []string{"region"}}
	d.Spec.ErrorBudgetPolicy = &v1.ErrorBudgetPolicy{Owner: "oncall", RunbookURL: "https://example.test/runbook", ReviewBelow: .25}
	resource, err := FromDefinition(model.Definition{SLO: &d}, "team-commerce")
	if err != nil {
		t.Fatal(err)
	}
	native, err := Native(resource)
	if err != nil || native.SLO.Spec.Lifecycle != d.Spec.Lifecycle || native.SLO.Spec.Policy.MinEvents != 100 || native.SLO.Spec.Quality.MinEvents != 100 || native.SLO.Spec.ErrorBudgetPolicy.Owner != "oncall" {
		t.Fatal(native, err)
	}
	for _, lifecycle := range []v1.SLOLifecycle{v1.LifecycleAspirational, v1.LifecycleCommitted} {
		d.Spec.Lifecycle = lifecycle
		rendered, err := PrometheusRule(d, "commerce", "team-commerce", "owner-uid", nil, 2)
		if err != nil {
			t.Fatal(err)
		}
		var file rules.File
		if err = json.Unmarshal(rendered.Spec, &file); err != nil {
			t.Fatal(err)
		}
		alerts := 0
		for _, group := range file.Groups {
			for _, rule := range group.Rules {
				if rule.Alert != "" {
					alerts++
				}
			}
		}
		if (alerts == 1) != (lifecycle == v1.LifecycleCommitted) {
			t.Fatal("operator alert lifecycle mismatch", lifecycle, alerts)
		}
	}
}
