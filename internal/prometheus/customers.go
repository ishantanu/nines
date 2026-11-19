package prometheus

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// ReadCustomers reads good and total counts for an explicit allowlist of
// customers, grouped by the policy label. The matcher is built only from
// validated values and quoted as exact alternatives, so the read is bounded by
// the allowlist size and cannot match anything else.
func (c *Client) ReadCustomers(ctx context.Context, s model.SLO, label string, values []string, at time.Time) (v1.CohortEvidence, error) {
	out := v1.CohortEvidence{At: at, WindowSeconds: s.WindowSeconds, Counts: []v1.CohortCount{}}
	if err := s.Validate(); err != nil {
		return out, err
	}
	if s.ProviderWindow == nil || s.Customers == nil || s.Customers.Label != label || len(values) == 0 || len(values) > model.MaxCustomers {
		return out, fmt.Errorf("customer inspection requires provider-window counts and the SLO's own bounded allowlist")
	}
	allowed := map[string]bool{}
	for _, customer := range s.Customers.Customers {
		allowed[customer.Value] = true
	}
	alternatives := make([]string, 0, len(values))
	for _, value := range values {
		if !allowed[value] {
			return out, fmt.Errorf("customer is not in the SLO allowlist")
		}
		alternatives = append(alternatives, regexp.QuoteMeta(value))
	}
	sort.Strings(alternatives)
	matcher := label + "=~" + strconv.Quote("^("+strings.Join(alternatives, "|")+")$")
	return c.readGrouped(ctx, s, []string{label}, matcher, at)
}
