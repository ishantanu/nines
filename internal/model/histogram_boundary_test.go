package model

import (
	"strings"
	"testing"

	"github.com/prometheus/prometheus/promql/parser"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/promqlvalidate"
)

func TestIntegerHistogramBoundaryMatchesOnlyEquivalentLabels(t *testing.T) {
	threshold := 2.0
	m := v1.MeasurementSpec{Type: "latency", Metric: "duration_seconds", Labels: map[string]string{"article": "2", "service": "api"}, ThresholdSeconds: &threshold, ScrapeIntervalSeconds: 5}
	window := MeasurementWindow(m)
	if !canonicalHistogramSelector(window.GoodSelector) {
		t.Fatal(window.GoodSelector)
	}
	expression, e := promqlvalidate.InstantVector(window.GoodSelector)
	if e != nil {
		t.Fatal(e)
	}
	vector := expression.(*parser.VectorSelector)
	for _, matcher := range vector.LabelMatchers {
		if matcher.Name != "le" {
			continue
		}
		for _, value := range []string{"2", "2.0"} {
			if !matcher.Matches(value) {
				t.Fatal("equivalent boundary excluded", value)
			}
		}
		for _, value := range []string{"20", "2.01", "1.99", "2e0", "+Inf"} {
			if matcher.Matches(value) {
				t.Fatal("inexact boundary matched", value)
			}
		}
	}
	for _, unsafe := range []string{strings.Replace(window.GoodSelector, `2\\.0`, `3\\.0`, 1), strings.Replace(window.GoodSelector, `le=~`, `other=~`, 1), window.GoodSelector + " offset 1m", `duration_seconds{le=~".*"}`} {
		if canonicalHistogramSelector(unsafe) {
			t.Fatal("unsafe selector accepted", unsafe)
		}
	}
}
