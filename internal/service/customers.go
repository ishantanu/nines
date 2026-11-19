package service

import (
	"context"
	"math"
	"sort"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// CustomerSource is optional. Adapters read whole-window counts for an explicit
// allowlist at one cutoff; credentials and query execution stay adapter concerns.
type CustomerSource interface {
	ReadCustomers(context.Context, model.SLO, string, []string, time.Time) (v1.CohortEvidence, error)
}

var customerLimitations = []string{
	"Only the customers listed in the SLO are reported, and each is measured against the same SLI over the same window as the SLO.",
	"A customer with no matching series is reported as no data, never as healthy.",
	"Customer figures are observed estimates at one cutoff. They do not establish statistical confidence, and small customers are noisy.",
	"Reporting does not create per-customer alerts, change the SLO's budget or promise a customer a service level.",
}

// CustomerReport is an explicit, bounded provider read for the SLO's allowlist.
// It never writes samples, deploys rules or logs customer identifiers.
func (s *Service) CustomerReport(ctx context.Context, id string, expected int64) (v1.CustomerReport, error) {
	if expected < 0 {
		return v1.CustomerReport{}, model.Invalid("expectedRevision must be nonnegative")
	}
	r, err := s.Store.Read(ctx, id)
	if err != nil {
		return v1.CustomerReport{}, err
	}
	if expected > 0 && model.ConfigRevision(r.SLO) != expected {
		return v1.CustomerReport{}, model.ErrRevisionConflict
	}
	x, err := s.statusFromRecord(r, time.Time{})
	if err != nil {
		return v1.CustomerReport{}, err
	}
	q := s.qualityFromRecord(r, x, s.Now(), false)
	report := v1.CustomerReport{APIVersion: v1.APIVersion, Tenant: s.TenantID, SLOID: id, Revision: q.Revision, State: "not_configured", Message: "Add spec.customers to report an SLO for a short list of customers.", At: x.At, WindowSeconds: r.SLO.WindowSeconds, ServiceBurnRate: x.BurnRate, Items: []v1.CustomerResult{}, Limitations: append([]string{}, customerLimitations...)}
	policy := r.SLO.Customers
	if policy == nil {
		return report, nil
	}
	report.Label = policy.Label
	reader, ok := s.CollectorFor(r.SLO).(CustomerSource)
	if !ok || r.SLO.ProviderWindow == nil {
		report.State = "unsupported"
		report.Message = "This provider or measurement mode cannot supply per-customer counts."
		return report, nil
	}
	values := make([]string, 0, len(policy.Customers))
	for _, c := range policy.Customers {
		values = append(values, c.Value)
	}
	job, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	evidence, readErr := reader.ReadCustomers(job, r.SLO, policy.Label, values, x.At)
	if ctx.Err() != nil {
		return report, ctx.Err()
	}
	latest, err := s.Store.Read(ctx, id)
	if err != nil {
		return report, err
	}
	if model.ConfigRevision(latest.SLO) != q.Revision || s.SourceFingerprint(r.SLO.Source) != q.SourceDigest {
		return report, model.ErrRevisionConflict
	}
	report.State = "unavailable"
	report.Message = "The provider could not supply complete, valid customer evidence."
	if readErr != nil || !evidence.At.Equal(x.At) || evidence.WindowSeconds != r.SLO.WindowSeconds || len(evidence.Counts) > len(policy.Customers) {
		return report, nil
	}
	counts := map[string]v1.CohortCount{}
	for _, item := range evidence.Counts {
		value, found := item.Labels[policy.Label]
		if !found || len(item.Labels) != 1 || !model.Finite(item.Total) || item.Total < 0 || item.Total > 1e18 {
			return report, nil
		}
		if _, dup := counts[value]; dup {
			return report, nil
		}
		if item.Good != nil && (!model.Finite(*item.Good) || *item.Good < 0 || *item.Good > item.Total) {
			return report, nil
		}
		counts[value] = item
	}
	for _, customer := range policy.Customers {
		name := customer.Name
		if name == "" {
			name = customer.Value
		}
		target := r.SLO.Target
		if customer.Target != nil {
			target = *customer.Target
		}
		result := v1.CustomerResult{Name: name, Alias: model.CustomerAlias(s.TenantID, id, customer.Value), Target: target, State: "no_series"}
		if item, found := counts[customer.Value]; found {
			result.Total, result.Good = item.Total, item.Good
			switch {
			case item.Good == nil:
				result.State = "missing_numerator"
			case item.Total == 0:
				result.State = "no_traffic"
			default:
				reliability := *item.Good / item.Total
				burn := (1 - reliability) / (1 - target)
				remaining := 1 - burn
				result.Reliability, result.BurnRate, result.BudgetRemainingFraction = &reliability, &burn, &remaining
				result.State = "observed"
				if item.Total < q.MinimumEvents {
					result.State = "low_traffic"
				} else if burn > 1 && (x.BurnRate == nil || *x.BurnRate <= 1) {
					result.MaskedByAggregate = true
				}
			}
		}
		report.Items = append(report.Items, result)
	}
	// Worst burn first; customers without a measurable burn follow, in the
	// order they were configured.
	sort.SliceStable(report.Items, func(i, j int) bool {
		a, b := report.Items[i].BurnRate, report.Items[j].BurnRate
		if a == nil || b == nil {
			return a != nil && b == nil
		}
		return *a > *b && !math.IsNaN(*a)
	})
	report.State = "available"
	report.Message = "Observed per-customer estimates at the same cutoff as the SLO."
	return report, nil
}
