package queryspec

import "testing"

func TestMetricsQLCounterCountAST(t *testing.T) {
	for _, q := range []string{`sum(increase(requests_total[60s]))`, `sum(increase_pure(requests_total{code="200"}[1m]))`, `sum(increase_prometheus(requests_total{code=~"2.."}[1m]))`} {
		if err := MetricsQLCount(q, 60); err != nil {
			t.Fatal(q, err)
		}
	}
	for _, q := range []string{`1`, `sum(rate(requests_total[1m]))`, `sum(increase(requests_total))`, `sum(increase(requests_total[5m]))`, `sum(increase(requests_total[1m] offset 1h))`, `sum(increase(requests_total[1m] @ 12345))`, `sum(increase(requests_total[1m:10s]))`, `sum(increase(requests_total[1i]))`, `sum(increase(requests_total[1m])) by (job)`, `sum(increase(requests_total[1m])) limit 1`, `sum(increase(requests_total[1m]))+1`, `WITH (x=requests_total) sum(increase(x[1m]))`, `sum(increase({job="api"}[1m]))`, `sum(increase_pure(requests_total[1m]) keep_metric_names)`, `sum(increase(requests_total[1m])) trailing`, `sum(increase(requests_total[1m]`, `sum(increase(nonexistent_function(requests_total)[1m]))`, `sum(increase(requests_total{code=~"("}[1m]))`, `sum(increase(ru(ru(ru(requests_total, requests_total), requests_total), requests_total)[1m]))`} {
		if err := MetricsQLCount(q, 60); err == nil {
			t.Fatal("unsafe or unsupported MetricsQL accepted", q)
		}
	}
}
