package rules

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/promqltest"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// The generated interval recordings must judge every interval exactly as the
// engine does. Both sides read the same synthetic series through the real
// PromQL engine: the engine side evaluates the SLO's own good and total
// queries (what the collector stores) and applies model.ClassifyInterval; the
// rule side evaluates the generated nines:sli_*:interval expressions.
func TestIntervalRecordingsAgreeWithTheEngineUnderRealPromQL(t *testing.T) {
	// Good and total events per 15-second scrape, one row per minute.
	minutes := []struct{ good, total int }{
		{240, 240}, // clean and busy
		{228, 240}, // 5% noise
		{100, 240}, // incident
		{0, 0},     // idle: nothing processed at all
		{30, 30},   // clean but slow
		{125, 130}, // just above the throughput line, some failures
		{0, 40},    // total outage
	}
	var good, total strings.Builder
	g, n := 0, 0
	good.WriteString(" requests_good 0")
	total.WriteString(" requests_total 0")
	for _, m := range minutes {
		for i := 0; i < 4; i++ {
			g += m.good / 4
			n += m.total / 4
			fmt.Fprintf(&good, " %d", g)
			fmt.Fprintf(&total, " %d", n)
		}
	}
	storage := promqltest.LoadedStorage(t, "load 15s\n"+good.String()+"\n"+total.String())
	t.Cleanup(func() { _ = storage.Close() })
	engine := promqltest.NewTestEngineWithOpts(t, promql.EngineOpts{MaxSamples: 1e6, Timeout: time.Minute})
	eval := func(expr string, at time.Time) (float64, bool) {
		t.Helper()
		q, err := engine.NewInstantQuery(context.Background(), storage, nil, expr, at)
		if err != nil {
			t.Fatalf("%s: %v", expr, err)
		}
		defer q.Close()
		res := q.Exec(context.Background())
		if res.Err != nil {
			t.Fatalf("%s: %v", expr, res.Err)
		}
		v, err := res.Vector()
		if err != nil {
			t.Fatal(err)
		}
		if len(v) == 0 {
			return 0, false
		}
		if len(v) != 1 {
			t.Fatalf("%s returned %d series", expr, len(v))
		}
		return v[0].F, true
	}
	for name, policy := range map[string]v1.BadIntervalPolicy{
		"good-minute":            {ErrorRate: 0.06},
		"good-minute with floor": {ErrorRate: 0.06, MinEvents: 200},
		"throughput":             {MinGoodEvents: 120},
	} {
		t.Run(name, func(t *testing.T) {
			s := fixture()
			s.BadInterval = &policy
			group, err := Generate(s, "tenant")
			if err != nil {
				t.Fatal(err)
			}
			exprs := map[string]string{}
			for _, r := range group.Rules {
				if strings.HasSuffix(r.Record, ":interval") {
					exprs[r.Record] = r.Expr
				}
			}
			if len(exprs) != 2 {
				t.Fatalf("expected good and total interval rules, got %v", exprs)
			}
			judgedBad := 0
			for i := range minutes {
				at := time.Unix(int64(60*(i+1)), 0)
				rawGood, okG := eval(s.GoodQuery, at)
				rawTotal, okT := eval(s.TotalQuery, at)
				if !okG || !okT {
					t.Fatalf("minute %d has no source data", i)
				}
				wantGood, wantTotal := model.ClassifyInterval(rawGood, rawTotal, policy)
				// Recording rules read the last completed step, so the rule
				// evaluated one step later records this interval.
				later := at.Add(time.Duration(s.StepSeconds) * time.Second)
				gotGood, _ := eval(exprs["nines:sli_good:interval"], later)
				gotTotal, _ := eval(exprs["nines:sli_total:interval"], later)
				if gotGood != wantGood || gotTotal != wantTotal {
					t.Errorf("minute %d (good %v, total %v): rules recorded (%v,%v), engine judged (%v,%v)", i, rawGood, rawTotal, gotGood, gotTotal, wantGood, wantTotal)
				}
				if wantTotal == 1 && wantGood == 0 {
					judgedBad++
				}
			}
			// Each policy must actually discriminate on this data.
			if judgedBad == 0 || judgedBad == len(minutes) {
				t.Fatalf("%d of %d minutes judged bad; the fixture does not exercise the policy", judgedBad, len(minutes))
			}
		})
	}
}
