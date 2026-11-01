// Package queryspec validates provider query contracts without executing them.
package queryspec

import (
	"fmt"
	"regexp"
	"strconv"
)

// Deliberately bounded: one count metric, exact tags or *, no arithmetic,
// grouping, interpolation, wildcards in values, percentiles or nested functions.
var datadogCount = regexp.MustCompile(`^sum:([a-zA-Z][a-zA-Z0-9_.]*)\{(\*|[a-zA-Z0-9_.\-/]+:[a-zA-Z0-9_.\-/]+(,[a-zA-Z0-9_.\-/]+:[a-zA-Z0-9_.\-/]+)*)\}\.as_count\(\)\.rollup\(sum,([1-9][0-9]{0,3})\)$`)

func DatadogCount(query string, step int64) (string, error) {
	m := datadogCount.FindStringSubmatch(query)
	if len(query) > 16000 || m == nil {
		return "", fmt.Errorf("datadog requires sum:metric{exact:tags}.as_count().rollup(sum,STEP), without grouping or arithmetic")
	}
	interval, _ := strconv.ParseInt(m[4], 10, 64)
	if step < 1 || step > 3600 || interval != step {
		return "", fmt.Errorf("datadog rollup must equal stepSeconds (1–3600)")
	}
	return m[1], nil
}
