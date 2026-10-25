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
	return nil
}
