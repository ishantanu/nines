package model

import (
	"net/url"
	"strings"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func validateBudgetPolicy(p *v1.ErrorBudgetPolicy) error {
	if p == nil {
		return nil
	}
	if !validText(p.Owner, 200) || strings.ContainsAny(p.Owner, "\r\n\x00") {
		return Invalid("errorBudgetPolicy.owner must identify a responsible team or person")
	}
	u, err := url.Parse(p.RunbookURL)
	if err != nil || len(p.RunbookURL) > 2000 || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return Invalid("errorBudgetPolicy.runbookURL must be an HTTP(S) URL without embedded credentials")
	}
	if !Finite(p.ReviewBelow) || p.ReviewBelow < 0 || p.ReviewBelow > 1 {
		return Invalid("errorBudgetPolicy.reviewBelow must be a remaining-budget fraction between 0 and 1")
	}
	if p.OnExhausted != "" && p.OnExhausted != "blocked" && p.OnExhausted != "review_required" {
		return Invalid("errorBudgetPolicy.onExhausted must be blocked or review_required")
	}
	return validateBudgetBands(p.Bands)
}

// MaxBudgetBands bounds the response levels in one policy.
const MaxBudgetBands = 5

func validateBudgetBands(bands []v1.BudgetBand) error {
	if len(bands) > MaxBudgetBands {
		return Invalid("errorBudgetPolicy.bands may have at most %d levels", MaxBudgetBands)
	}
	names, levels := map[string]bool{}, map[float64]bool{}
	for _, b := range bands {
		if !identifier.MatchString(b.Name) || names[b.Name] {
			return Invalid("each budget band needs a unique name of lowercase letters, digits and hyphens")
		}
		names[b.Name] = true
		if !Finite(b.Below) || b.Below <= 0 || b.Below >= 1 || levels[b.Below] {
			return Invalid("each budget band needs a distinct remaining-budget fraction above 0 and below 1")
		}
		levels[b.Below] = true
		if !validText(b.Response, 300) || strings.ContainsAny(b.Response, "\x00") {
			return Invalid("each budget band needs a response of 1 to 300 characters")
		}
	}
	return nil
}

// MatchBudgetBand returns the most severe band that applies: the one with the
// smallest threshold at or above the remaining budget fraction. It returns nil
// when none applies. An exhausted or overspent budget matches the lowest band.
func MatchBudgetBand(bands []v1.BudgetBand, remaining float64) *v1.BudgetBandMatch {
	var match *v1.BudgetBand
	for i := range bands {
		b := &bands[i]
		if remaining <= b.Below && (match == nil || b.Below < match.Below) {
			match = b
		}
	}
	if match == nil {
		return nil
	}
	return &v1.BudgetBandMatch{Name: match.Name, Below: match.Below, Response: match.Response}
}
