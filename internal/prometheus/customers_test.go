package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func customerPromSLO() model.SLO {
	m := v1.MeasurementSpec{Type: "availability", Metric: "http_requests_total", Labels: map[string]string{"service": "api"}, SuccessLabels: map[string]string{"status": "200"}, ScrapeIntervalSeconds: 15, ObservationPoint: "Completed API requests."}
	return model.SLO{ID: "api", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 180, StepSeconds: 60, Source: "primary", Measurement: &m, ProviderWindow: model.MeasurementWindow(m),
		Customers: &v1.CustomerPolicy{Label: "account", Customers: []v1.CustomerRef{{Value: "acme-01"}, {Value: "glob.ex"}}}}
}

func TestCustomerReadIsLimitedToTheAllowlistWithQuotedAlternatives(t *testing.T) {
	at := time.Unix(1800000000, 0).UTC()
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("query"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"account":"acme-01"},"value":[1800000000,"10"]}]}}`)
	}))
	defer server.Close()
	c, err := NewMimir(server.URL, "isolated", "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	s := customerPromSLO()
	out, err := c.ReadCustomers(context.Background(), s, "account", []string{"glob.ex", "acme-01"}, at)
	if err != nil || len(out.Counts) != 1 || len(queries) != 2 {
		t.Fatal(out, err, queries)
	}
	for _, q := range queries {
		// Exact alternatives, sorted, with the dot escaped so it cannot match another value.
		if !strings.Contains(q, `account=~"^(acme-01|glob\\.ex)$"`) || !strings.HasPrefix(q, "topk(101, sum by (account) (") {
			t.Fatalf("customer matcher missing or unquoted: %s", q)
		}
		if !strings.Contains(q, `service="api"`) {
			t.Fatalf("the measurement population must be preserved: %s", q)
		}
	}
	before := len(queries)
	if _, err = c.ReadCustomers(context.Background(), s, "account", []string{"acme-01", "not-allowed"}, at); err == nil || len(queries) != before {
		t.Fatal("a value outside the allowlist reached the provider")
	}
	if _, err = c.ReadCustomers(context.Background(), s, "region", []string{"acme-01"}, at); err == nil || len(queries) != before {
		t.Fatal("a different label reached the provider")
	}
}

func TestWithMatcherInsertsIntoBareAndBracedSelectors(t *testing.T) {
	for in, want := range map[string]string{
		"metric":                   `metric{a="b"}`,
		`metric{x="y"}`:            `metric{x="y",a="b"}`,
		"metric{}":                 `metric{a="b"}`,
		`m{le=~"^(0.3|0.3\\.0)$"}`: `m{le=~"^(0.3|0.3\\.0)$",a="b"}`,
	} {
		if got := withMatcher(in, `a="b"`); got != want {
			t.Errorf("%s -> %s, want %s", in, got, want)
		}
	}
}
