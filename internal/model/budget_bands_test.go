package model

import (
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestBudgetBandsPickTheMostSevereLevelThatApplies(t *testing.T) {
	bands := []v1.BudgetBand{
		{Name: "protect", Below: 0.25, Response: "Halt risky launches."},
		{Name: "watch", Below: 0.5, Response: "Prioritize reliability bugs."},
	}
	for remaining, want := range map[float64]string{0.9: "", 0.5001: "", 0.5: "watch", 0.3: "watch", 0.25: "protect", 0.1: "protect", 0: "protect", -2: "protect"} {
		got := MatchBudgetBand(bands, remaining)
		name := ""
		if got != nil {
			name = got.Name
		}
		if name != want {
			t.Errorf("remaining %v: band %q, want %q", remaining, name, want)
		}
	}
	if MatchBudgetBand(nil, 0) != nil {
		t.Error("no bands must match nothing")
	}
}

func TestBudgetBandValidation(t *testing.T) {
	ok := []v1.BudgetBand{{Name: "watch", Below: 0.5, Response: "Reliability bugs first."}, {Name: "protect", Below: 0.2, Response: "Halt risky launches."}}
	if err := validateBudgetBands(ok); err != nil {
		t.Fatal(err)
	}
	tooMany := []v1.BudgetBand{}
	for i := 0; i < 6; i++ {
		tooMany = append(tooMany, v1.BudgetBand{Name: "b" + string(rune('a'+i)), Below: 0.1 * float64(i+1), Response: "x"})
	}
	for name, bands := range map[string][]v1.BudgetBand{
		"too many":        tooMany,
		"duplicate name":  {{Name: "a", Below: .5, Response: "x"}, {Name: "a", Below: .2, Response: "y"}},
		"duplicate level": {{Name: "a", Below: .5, Response: "x"}, {Name: "b", Below: .5, Response: "y"}},
		"below zero":      {{Name: "a", Below: 0, Response: "x"}},
		"below one":       {{Name: "a", Below: 1, Response: "x"}},
		"bad name":        {{Name: "A b", Below: .5, Response: "x"}},
		"empty response":  {{Name: "a", Below: .5, Response: ""}},
		"long response":   {{Name: "a", Below: .5, Response: strings.Repeat("x", 301)}},
	} {
		if err := validateBudgetBands(bands); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
