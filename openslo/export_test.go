package openslo

import (
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func definition() v1.ServiceLevelObjective {
	return v1.NewSLO("api-availability", "production", v1.SLOSpec{Service: "api", DisplayName: "API availability", Target: .999, WindowSeconds: 28 * 86400, StepSeconds: 60, ProviderWindow: &v1.ProviderWindow{GoodSelector: `requests_total{code="200"}`, TotalSelector: `requests_total`, ScrapeIntervalSeconds: 15}, Policies: []v1.BurnRatePolicy{{Name: "fast", Severity: "critical", ShortWindowSeconds: 300, LongWindowSeconds: 3600, BurnRate: 14.4, MinCoverage: 1}}})
}
func TestExportUsesOpenSLOUnitsAndReportsPolicyOmissions(t *testing.T) {
	d := definition()
	result, err := Export(d)
	if err != nil {
		t.Fatal(err)
	}
	if result.Document.APIVersion != "openslo/v1" || result.Document.Spec.TimeWindow[0].Duration != "28d" || result.Document.Spec.Indicator.Metadata.Name != "api-availability" || result.Document.Spec.Objectives[0].Target != .999 || !strings.Contains(strings.Join(result.Warnings, " "), "Burn policies are not exported") {
		t.Fatal(result)
	}
}
func TestRejectUnsupportedMappingsInsteadOfRewritingThem(t *testing.T) {
	for _, edit := range []func(*v1.ServiceLevelObjective){
		func(d *v1.ServiceLevelObjective) {
			d.Spec.ProviderWindow = nil
			d.Spec.GoodQuery = "vector(99)"
			d.Spec.TotalQuery = "vector(100)"
		},
		func(d *v1.ServiceLevelObjective) { d.Spec.ProviderWindow.GoodSelector = `sum(requests_total)` },
		func(d *v1.ServiceLevelObjective) { d.Metadata.Name = "api_availability" },
		func(d *v1.ServiceLevelObjective) { d.Spec.QueryLanguage = "metricsql" },
	} {
		d := definition()
		edit(&d)
		if _, err := Export(d); err == nil {
			t.Fatal("unsupported definition translated")
		}
	}
}
