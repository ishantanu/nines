package rules

import (
	"strconv"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/model"
)

func TestIntervalRecordingHistoryIsIsolatedAcrossMeasurementEdits(t *testing.T) {
	s := model.SLO{ID: "editable", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 3600, StepSeconds: 60, GoodQuery: "vector(99)", TotalQuery: "vector(100)", Revision: 1, Policy: &model.Policy{ShortWindowSeconds: 300, LongWindowSeconds: 900, BurnRate: 4, MinCoverage: 1}}
	for _, revision := range []int64{1, 2} {
		s.Revision = revision
		if revision == 2 {
			s.GoodQuery = "vector(95)"
		}
		group, err := GenerateOwned(s, "commerce", "stable-uid")
		if err != nil {
			t.Fatal(err)
		}
		for _, rule := range group.Rules {
			if rule.Labels["nines_revision"] != strconv.FormatInt(revision, 10) {
				t.Fatal("recording lacks revision fence", rule.Record, rule.Labels)
			}
			if (strings.Contains(rule.Expr, "nines:sli_good:interval{") || strings.Contains(rule.Expr, "nines:slo_burn_rate:ratio{")) && !strings.Contains(rule.Expr, "nines_revision=\""+strconv.FormatInt(revision, 10)+"\"") {
				t.Fatal("query can consume another revision", rule.Expr)
			}
		}
	}
}
