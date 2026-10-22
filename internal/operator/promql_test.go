package operator

import (
	"strings"
	"testing"
)

func TestOperatorRenderRejectsInvalidPromQL(t *testing.T) {
	for _, query := range []string{"sum(", "1", "up[5m]"} {
		d := definition()
		d.Spec.GoodQuery = query
		if _, err := PrometheusRule(d, "commerce", "team-commerce", "owner-uid", nil, 1); err == nil || !strings.Contains(err.Error(), "goodQuery") {
			t.Fatalf("operator accepted invalid SLO: %q %v", query, err)
		}
	}
}
