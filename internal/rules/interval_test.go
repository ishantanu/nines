package rules

import (
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestBadIntervalRecordsJudgedIntervalsAndKeepsPlainSLOsUnchanged(t *testing.T) {
	plain := fixture()
	base, err := Generate(plain, "tenant")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range base.Rules {
		if strings.Contains(r.Expr, ">= bool") {
			t.Fatal("a plain SLO must not change", r.Expr)
		}
	}
	s := fixture()
	s.BadInterval = &v1.BadIntervalPolicy{ErrorRate: 0.06, MinEvents: 20}
	g, err := Generate(s, "tenant")
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, r := range g.Rules {
		switch r.Record {
		case "nines:sli_good:interval":
			found++
			if !strings.Contains(r.Expr, "<= bool 0.06") || !strings.Contains(r.Expr, ">= bool 20") {
				t.Fatal("good interval must apply the error rate and floor", r.Expr)
			}
		case "nines:sli_total:interval":
			found++
			if !strings.Contains(r.Expr, ">= bool 20") || strings.Contains(r.Expr, "<= bool") {
				t.Fatal("total interval counts judged intervals only", r.Expr)
			}
		}
	}
	if found != 2 {
		t.Fatalf("expected both interval recordings, found %d", found)
	}
	// The default floor is one event, so idle intervals are never judged.
	s.BadInterval.MinEvents = 0
	g, err = Generate(s, "tenant")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range g.Rules {
		if r.Record == "nines:sli_total:interval" && !strings.Contains(r.Expr, ">= bool 1)") {
			t.Fatal("an idle interval must not be judged", r.Expr)
		}
	}
}
