package slotemplates

import (
	"errors"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
)

func TestCatalogProducesIndependentValidNonPagingSLOs(t *testing.T) {
	catalog, err := List("")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	categories := map[string]bool{}
	if len(catalog.Templates) != 15 || len(catalog.Categories) != 6 || len(catalog.Digest) != 64 {
		t.Fatal(catalog)
	}
	for _, template := range catalog.Templates {
		t.Run(template.ID, func(t *testing.T) {
			if seen[template.ID] || len(template.Digest) != 64 || template.Revision != 1 {
				t.Fatal(template)
			}
			seen[template.ID] = true
			categories[template.Category] = true
			draft, err := Render(template.ID, v1.TemplateValues{})
			if err != nil {
				t.Fatal(err)
			}
			d, err := model.ParseSLODefinition([]byte(draft.YAML))
			if err != nil {
				t.Fatal(err)
			}
			if draft.TelemetryVerified || d.Spec.Lifecycle != v1.LifecycleExperimental || d.Spec.MeasurementBinding != nil || d.Spec.Measurement == nil || len(draft.Warnings) < 4 {
				t.Fatal(draft)
			}
			if _, err = rules.Generate(d.SLO(), "tenant"); err != nil {
				t.Fatal(err)
			}
			m := d.Spec.Measurement
			m.Labels["changed"] = "true"
			fresh, _ := Get(template.ID)
			if fresh.Manifest.Spec.Measurement.Labels["changed"] != "" {
				t.Fatal("shared mutable template")
			}
			again, _ := Render(template.ID, v1.TemplateValues{})
			if again.YAML != draft.YAML || again.ManifestDigest != draft.ManifestDigest || again.TemplateDigest != template.Digest {
				t.Fatal("unstable output")
			}
		})
	}
	if len(categories) != 6 {
		t.Fatal(categories)
	}
	filtered, _ := List("agent")
	if filtered.Digest != catalog.Digest || len(filtered.Templates) != 4 {
		t.Fatal(filtered)
	}
	if _, err = List("invented"); err == nil {
		t.Fatal(err)
	}
	if _, err = Get("../private"); !errors.Is(err, model.ErrNotFound) {
		t.Fatal(err)
	}
}
func TestRenderOverridesAreValidatedWithoutChangingCatalog(t *testing.T) {
	target := .95
	window := int64(86400)
	d, err := Render("agent-task-completion", v1.TemplateValues{Name: "support-completion", Project: "support", Service: "support-agent", Source: "production", Target: &target, WindowSeconds: &window})
	if err != nil {
		t.Fatal(err)
	}
	if d.Manifest.Metadata.Name != "support-completion" || d.Manifest.Spec.Source != "production" || d.Manifest.Spec.Measurement.Labels["service"] != "support-agent" || d.Manifest.Spec.Target != target {
		t.Fatal(d)
	}
	invalidTarget := 1.0
	zeroWindow := int64(0)
	for _, v := range []v1.TemplateValues{{Name: "../escape"}, {Source: "http://other"}, {Target: &invalidTarget}, {WindowSeconds: &zeroWindow}, {Measurement: &v1.MeasurementSpec{Type: "latency", Metric: "p99"}}} {
		if _, err = Render("agent-task-completion", v); err == nil {
			t.Fatal("accepted invalid override", v)
		}
	}
	fresh, _ := Get("agent-task-completion")
	if fresh.Manifest.Spec.Target == target {
		t.Fatal("changed catalog")
	}
}
