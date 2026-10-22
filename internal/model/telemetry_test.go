package model_test

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestTelemetryResourceStrictYAML(t *testing.T) {
	for _, resource := range []any{testtelemetry.Binding(), testtelemetry.Graph(true)} {
		b, err := yaml.Marshal(resource)
		if err != nil {
			t.Fatal(err)
		}
		d, err := model.ParseDefinition(b)
		if err != nil || d.Manifest() == nil {
			t.Fatal(err)
		}
		if _, err = model.ParseDefinition(append(b, []byte("unexpected: true\n")...)); err == nil {
			t.Fatal("unknown field accepted")
		}
		if _, err = model.ParseDefinition(append(b, append([]byte("---\n"), b...)...)); err == nil {
			t.Fatal("multiple documents accepted")
		}
	}
}
func TestBindingLabelsCannotBecomeQueries(t *testing.T) {
	b := testtelemetry.Binding()
	b.Spec.Metric = `up) or vector(1)`
	if b.Validate() == nil {
		t.Fatal("query injection accepted")
	}
	b = testtelemetry.Binding()
	b.Spec.SuccessLabels = map[string]string{"service": "other"}
	if b.Validate() == nil {
		t.Fatal("population override accepted")
	}
	if model.ValidateLabels(map[string]string{"__name__": "up"}) == nil {
		t.Fatal("reserved label accepted")
	}
	b = testtelemetry.Binding()
	b.Spec.Labels["service"] = strings.Repeat("x", 201)
	if b.Validate() == nil {
		t.Fatal("oversized label accepted")
	}
}
