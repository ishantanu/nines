package queryspec

import "testing"

func TestNRQLCountSubset(t *testing.T) {
	for _, query := range []string{"SELECT count(*) FROM Transaction WHERE appName = 'nines-api' AND error = false", "select count(*) from NinesRequest WHERE duration < 0.3", "SELECT count(*) FROM NinesRequest WHERE result = 'GOOD AND SAFE'", "SELECT count(*) FROM NinesRequest"} {
		q, err := NewRelicCount(query, 60)
		if err != nil || q.EventType == "" || q.NRQL == "" {
			t.Fatal(query, q, err)
		}
	}
	for _, query := range []string{"SELECT average(duration) FROM Transaction", "SELECT percentile(duration, 99) FROM Transaction", "SELECT count(*) FROM Metric", "SELECT count(*) FROM Transaction SINCE 1 day ago", "SELECT count(*) FROM Transaction FACET appName", "SELECT count(*) FROM Transaction TIMESERIES AUTO", "SELECT count(*) FROM Transaction WHERE appName = 'api' OR true", "SELECT count(*) FROM Transaction; SELECT count(*) FROM Other", "SELECT count(*) FROM Transaction WHERE appName = 'a\\b'", "SELECT count(*) FROM Transaction WHERE appName = 'a\nb'", "SELECT count(*) FROM Transaction WHERE duration < 0.3 EXTRAPOLATE", "SELECT count(*) AS value FROM Transaction", "SELECT count(*) FROM Transaction, Other"} {
		if _, err := NewRelicCount(query, 60); err == nil {
			t.Fatal("accepted unsupported query", query)
		}
	}
}
