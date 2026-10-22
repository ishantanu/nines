package rules

import (
	"testing"

	"github.com/ishantanu/nines/internal/model"
)

func TestSeverityRulesUseDistinctLabelsAndSharedWindows(t *testing.T) {
	s := fixture()
	s.Policy = nil
	s.Policies = []model.Policy{{Name: "fast", Severity: "critical", ShortWindowSeconds: 300, LongWindowSeconds: 3600, BurnRate: 14.4, MinCoverage: 1}, {Name: "slow", Severity: "warning", ShortWindowSeconds: 300, LongWindowSeconds: 7200, BurnRate: 6, ForSeconds: 60, MinCoverage: 1}}
	g, err := Generate(s, "commerce")
	if err != nil {
		t.Fatal(err)
	}
	alerts := map[string]string{}
	recordings := map[string]bool{}
	for _, r := range g.Rules {
		if r.Alert != "" {
			alerts[r.Labels["nines_policy"]] = r.Labels["severity"]
		} else {
			key := r.Record + "/" + r.Labels["window"]
			if recordings[key] {
				t.Fatal("duplicate recording", key)
			}
			recordings[key] = true
		}
	}
	if len(alerts) != 2 || alerts["fast"] != "critical" || alerts["slow"] != "warning" {
		t.Fatal(alerts)
	}
}
