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

func TestCohortAdapterPinsTenantCutoffAndPreservesMissingNumerators(t *testing.T) {
	at := time.Unix(1800000000, 0).UTC()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Scope-OrgID") != "isolated" || r.Header.Get("Authorization") != "Bearer fixture-token" || r.URL.Query().Get("time") != "1800000000" || r.URL.Query().Get("timeout") != "15s" {
			t.Error("query lost credential, tenant or cutoff")
		}
		q := r.URL.Query().Get("query")
		if !strings.HasPrefix(q, "topk(101, sum by (region) (increase(") {
			t.Error(q)
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(q, "request_total[") {
			_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"region":"a"},"value":[1800000000,"10"]},{"metric":{"region":"b"},"value":[1800000000,"1"]}]}}`)
		} else {
			_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"region":"a"},"value":[1800000000,"10"]}]}}`)
		}
	}))
	defer server.Close()
	c, err := NewMimir(server.URL, "isolated", "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	s := model.SLO{ID: "api", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 180, StepSeconds: 60, ProviderWindow: &v1.ProviderWindow{GoodSelector: "good_total", TotalSelector: "request_total", ScrapeIntervalSeconds: 15}}
	out, err := c.ReadCohorts(context.Background(), s, []string{"region"}, at)
	if err != nil || calls != 2 || !out.At.Equal(at) || len(out.Counts) != 2 {
		t.Fatal(out, err)
	}
	for _, item := range out.Counts {
		if item.Labels["region"] == "b" && item.Good != nil {
			t.Fatal("missing numerator zero-filled")
		}
	}
	before := calls
	if _, err = c.ReadCohorts(context.Background(), s, []string{"region) or vector(1)"}, at); err == nil || calls != before {
		t.Fatal("invalid label reached provider")
	}
}
