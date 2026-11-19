package rules

import (
	"strings"
	"testing"
)

func TestConfidenceAddsTheSameThresholdToBothRulePaths(t *testing.T) {
	for _, providerWindow := range []bool{false, true} {
		plain, err := GenerateOwned(lifecycleRuleFixture(providerWindow), "tenant", "owner-uid")
		if err != nil {
			t.Fatal(err)
		}
		for _, rule := range plain.Rules {
			if rule.Alert != "" && strings.Contains(rule.Expr, "sqrt(") {
				t.Fatal("a policy without confidence must not change its rule", rule.Expr)
			}
		}
		s := lifecycleRuleFixture(providerWindow)
		s.Policy.Confidence = 0.99
		group, err := GenerateOwned(s, "tenant", "owner-uid")
		if err != nil {
			t.Fatalf("providerWindow=%v: %v", providerWindow, err)
		}
		alerts := 0
		for _, rule := range group.Rules {
			if rule.Alert == "" {
				continue
			}
			alerts++
			// One confidence clause per burn window, each carrying the same z score the engine uses.
			if strings.Count(rule.Expr, "sqrt(") != 2 || strings.Count(rule.Expr, "2.5758293") != 2 {
				t.Fatalf("providerWindow=%v: expected two confidence clauses with z=2.5758293: %s", providerWindow, rule.Expr)
			}
		}
		if alerts != 1 {
			t.Fatal(alerts)
		}
	}
}
