package rules

import (
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func TestLifecycleRuleGenerationRetainsRecordingsWithoutAlertLeakage(t *testing.T) {
	for _, providerWindow := range []bool{false, true} {
		s := lifecycleRuleFixture(providerWindow)
		s.Policy.MinEvents = 100
		active, err := GenerateOwned(s, "tenant", "owner-uid")
		if err != nil {
			t.Fatal(err)
		}
		var recordings []Rule
		for _, rule := range active.Rules {
			if rule.Record != "" {
				recordings = append(recordings, rule)
			}
		}
		for _, lifecycle := range []v1.SLOLifecycle{v1.LifecycleExperimental, v1.LifecycleAspirational, v1.LifecycleDeprecated} {
			s.Lifecycle = lifecycle
			group, err := GenerateOwned(s, "tenant", "owner-uid")
			if err != nil {
				t.Fatal(err)
			}
			if len(group.Rules) != len(recordings) {
				t.Fatalf("recording windows lost for %s", lifecycle)
			}
			for i, rule := range group.Rules {
				if rule.Alert != "" || rule.Record != recordings[i].Record || rule.Expr != recordings[i].Expr {
					t.Fatal("alert leakage or measurement changed", rule)
				}
			}
		}
	}
}

func TestMinEventsRuleGuardsBothWindowsWithScopedCounts(t *testing.T) {
	for _, providerWindow := range []bool{false, true} {
		s := lifecycleRuleFixture(providerWindow)
		s.Policy.MinEvents = 100
		group, err := GenerateOwned(s, "tenant", "owner-uid")
		if err != nil {
			t.Fatal(err)
		}
		alerts := 0
		for _, rule := range group.Rules {
			if rule.Alert == "" {
				continue
			}
			alerts++
			if strings.Count(rule.Expr, ">= 100)") != 2 {
				t.Fatal("both count guards required", rule.Expr)
			}
			if !providerWindow {
				for _, window := range []string{"[300s] offset 60s", "[3600s] offset 60s"} {
					if !strings.Contains(rule.Expr, window) {
						t.Fatal("wrong count window", rule.Expr)
					}
				}
				if !strings.Contains(rule.Expr, `nines_resource_uid="owner-uid"`) || !strings.Contains(rule.Expr, `nines_tenant="tenant"`) {
					t.Fatal("count guard not owner scoped", rule.Expr)
				}
			} else if strings.Contains(rule.Expr, "sum_over_time(nines:") {
				t.Fatal("overlapping observations summed")
			}
		}
		if alerts != 1 {
			t.Fatal(alerts)
		}
	}
}

func lifecycleRuleFixture(providerWindow bool) model.SLO {
	s := fixture()
	if providerWindow {
		s.GoodQuery, s.TotalQuery = "", ""
		s.ProviderWindow = &v1.ProviderWindow{GoodSelector: "requests_good", TotalSelector: "requests_total", ScrapeIntervalSeconds: 15}
	}
	return s
}
