package model_test

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/examples"
	"github.com/ishantanu/nines/internal/model"
)

func TestLifecycleAndTrafficGuardDefinitionContract(t *testing.T) {
	d, err := model.ParseSLODefinition([]byte(examples.APIAvailability))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(d)
	if strings.Contains(string(b), "lifecycle") || strings.Contains(string(b), "minEvents") {
		t.Fatal("new defaults changed canonical documents", string(b))
	}
	if !d.SLO().AlertsEnabled() {
		t.Fatal("omitted lifecycle must retain committed behavior")
	}
	for _, lifecycle := range []v1.SLOLifecycle{v1.LifecycleExperimental, v1.LifecycleAspirational, v1.LifecycleCommitted, v1.LifecycleDeprecated} {
		d.Spec.Lifecycle = lifecycle
		d.Spec.Policies[0].MinEvents = 100
		if err = d.Validate(); err != nil {
			t.Fatal(err)
		}
		b, err = yaml.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := model.ParseSLODefinition(b)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(parsed, d) || !reflect.DeepEqual(model.DefinitionFromSLO(parsed.SLO()), d) {
			t.Fatal("lifecycle or traffic guard lost on round trip")
		}
		if parsed.SLO().AlertsEnabled() != (lifecycle == v1.LifecycleCommitted) {
			t.Fatal(lifecycle)
		}
	}
	d.Spec.Lifecycle = "active"
	if d.Validate() == nil {
		t.Fatal("unknown lifecycle accepted")
	}
}

func TestBurnPoliciesRejectUnreachableThresholdsAndInvalidTrafficCounts(t *testing.T) {
	d, err := model.ParseSLODefinition([]byte(examples.APIAvailability))
	if err != nil {
		t.Fatal(err)
	}
	d.Spec.Target = .9
	d.Spec.Policies = d.Spec.Policies[:1]
	d.Spec.Policies[0].BurnRate = 10 // 100% bad traffic, tolerate floating point rounding.
	if err = d.Validate(); err != nil {
		t.Fatal(err)
	}
	d.Spec.Policies[0].BurnRate = 10.01
	if err = d.Validate(); err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Fatal(err)
	}
	d.Spec.Policies[0].BurnRate = 2
	for _, count := range []float64{-1, math.NaN(), math.Inf(1), 1e19} {
		d.Spec.Policies[0].MinEvents = count
		if d.Validate() == nil {
			t.Fatalf("invalid count %g accepted", count)
		}
	}
	d.Spec.Policies[0].MinEvents = 100.5 // Provider increases may be extrapolated fractional counts.
	if err = d.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestOfflineIntentValidationIncludesAlertSafety(t *testing.T) {
	goal := v1.IntentSpec{Description: "API requests succeed", Service: "api", Source: "primary", Outcome: "availability", Population: map[string]string{"service": "api"}, Target: .9, WindowSeconds: 3600, NoDataPolicy: "unknown", Policy: &v1.BurnRatePolicy{ShortWindowSeconds: 60, LongWindowSeconds: 300, BurnRate: 10, MinCoverage: 1}}
	if err := model.ValidateIntentSpec(goal); err != nil {
		t.Fatal("legacy intent must validate before interval selection", err)
	}
	goal.Policy.BurnRate = 14.4
	if err := model.ValidateIntentSpec(goal); err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Fatal("offline intent accepted unreachable threshold", err)
	}
	goal.Policy.BurnRate = 2
	goal.Policy.MinEvents = -1
	if model.ValidateIntentSpec(goal) == nil {
		t.Fatal("offline intent accepted negative traffic guard")
	}
	goal.Policy.MinEvents = 100
	goal.StepSeconds = 120
	if model.ValidateIntentSpec(goal) == nil {
		t.Fatal("inline intent accepted misaligned burn window")
	}
}
