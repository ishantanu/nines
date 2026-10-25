package model

import (
	"strings"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func validateQuality(q *v1.MeasurementQualityPolicy, step int64) error {
	if q == nil {
		return nil
	}
	if !Finite(q.MinEvents) || q.MinEvents < 0 || q.MinEvents > 1e18 {
		return Invalid("quality.minEvents must be a finite count between 0 and 1e18")
	}
	if q.MaxSampleAgeSeconds < 0 || q.MaxSampleAgeSeconds > 31*86400 || (q.MaxSampleAgeSeconds > 0 && q.MaxSampleAgeSeconds < step) {
		return Invalid("quality.maxSampleAgeSeconds must be zero (automatic), or between stepSeconds and 31 days")
	}
	if len(q.CohortLabels) > 3 {
		return Invalid("quality supports up to three cohortLabels")
	}
	seen := map[string]bool{}
	for _, label := range q.CohortLabels {
		if !LabelName.MatchString(label) || strings.HasPrefix(label, "__") || label == "le" || seen[label] {
			return Invalid("quality.cohortLabels require unique, ordinary metric label names")
		}
		seen[label] = true
	}
	return nil
}
