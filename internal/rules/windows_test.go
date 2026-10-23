package rules

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestProviderWindowRulesUseIndependentWindowsAndPinnedCutoffs(t *testing.T) {
	s := fixture()
	s.ID = "window-proof"
	s.Revision = 2
	s.StepSeconds = 10
	s.WindowSeconds = 120
	s.Policy.ShortWindowSeconds = 20
	s.Policy.LongWindowSeconds = 40
	s.GoodQuery = ""
	s.TotalQuery = ""
	s.ProviderWindow = &v1.ProviderWindow{GoodSelector: `request_good{service="api"}`, TotalSelector: `request_total{service="api"}`, ScrapeIntervalSeconds: 2}
	g, err := Generate(s, "broker-lab")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Rules) != 28 {
		t.Fatal(len(g.Rules))
	}
	for _, r := range g.Rules {
		if strings.Contains(r.Expr, "sum_over_time(nines:") || strings.Contains(r.Record, ":interval") {
			t.Fatal("interval summation retained", r)
		}
		if r.Labels["nines_revision"] != "2" || r.Labels["nines_measurement"] != "provider_window" {
			t.Fatal(r)
		}
	}
	if path := os.Getenv("WINDOW_RULES_PATH"); path != "" {
		b, err := yaml.Marshal(File{Groups: []Group{g}})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Interval data cannot be silently switched to overlapping windows.
	if err = s.Validate(); err != nil {
		t.Fatal(err)
	}
	s.GoodQuery = "legacy"
	s.TotalQuery = "legacy"
	if s.Validate() == nil {
		t.Fatal("mixed modes accepted")
	}
}
