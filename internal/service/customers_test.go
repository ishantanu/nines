package service

import (
	"context"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

type customerSource struct {
	qualityCohortSource
	evidence v1.CohortEvidence
	err      error
	calls    int
	asked    []string
}

func (f *customerSource) ReadCustomers(_ context.Context, _ model.SLO, _ string, values []string, _ time.Time) (v1.CohortEvidence, error) {
	f.calls++
	f.asked = values
	return f.evidence, f.err
}

func customerFixture(t *testing.T) (*Service, *customerSource) {
	t.Helper()
	ctx := context.Background()
	source := &customerSource{}
	s := New(store.NewMemory(), source)
	s.Now = func() time.Time { return now.Add(time.Minute) }
	m := v1.MeasurementSpec{Type: "availability", Metric: "http_requests_total", Labels: map[string]string{"service": "api"}, SuccessLabels: map[string]string{"status": "200"}, ScrapeIntervalSeconds: 15, ObservationPoint: "Completed API requests."}
	c := model.SLO{ID: "cust", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 180, StepSeconds: 60, Source: "primary", Measurement: &m, ProviderWindow: model.MeasurementWindow(m),
		Quality:   &v1.MeasurementQualityPolicy{MinEvents: 100},
		Customers: &v1.CustomerPolicy{Label: "account", Customers: []v1.CustomerRef{{Value: "acme-01", Name: "Acme"}, {Value: "globex"}, {Value: "initech", Target: ptr(.9)}, {Value: "ghost"}}}}
	if err := s.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Update(ctx, c.ID, func(r *model.Record) error {
		// Service-wide: 99.5% reliable, a 0.5x burn, comfortably inside budget.
		r.Windows[model.WindowKey(180, now)] = model.WindowObservation{Time: now, WindowSeconds: 180, Revision: 1, Good: 99500, Total: 100000, Coverage: 1}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return s, source
}

func ptr(v float64) *float64 { return &v }

func TestCustomerReportShowsACustomerTheAggregateHides(t *testing.T) {
	s, source := customerFixture(t)
	acme, globex, initech := 900.0, 5000.0, 76.0
	source.evidence = v1.CohortEvidence{At: now, WindowSeconds: 180, Counts: []v1.CohortCount{
		{Labels: map[string]string{"account": "acme-01"}, Good: &acme, Total: 1000},  // 90%: a 10x burn
		{Labels: map[string]string{"account": "globex"}, Good: &globex, Total: 5000}, // perfect
		{Labels: map[string]string{"account": "initech"}, Good: &initech, Total: 80}, // 95% against its own 90% target
	}}
	report, err := s.CustomerReport(context.Background(), "cust", 1)
	if err != nil || report.State != "available" || len(report.Items) != 4 || source.calls != 1 {
		t.Fatalf("%+v %v", report, err)
	}
	if len(source.asked) != 4 {
		t.Fatalf("only the allowlist may be asked for: %v", source.asked)
	}
	worst := report.Items[0]
	if worst.Name != "Acme" || worst.BurnRate == nil || *worst.BurnRate < 9.9 || !worst.MaskedByAggregate {
		t.Fatalf("a 10x burn hidden by a healthy aggregate must be flagged: %+v", worst)
	}
	if report.ServiceBurnRate == nil || *report.ServiceBurnRate > 1 {
		t.Fatalf("the aggregate must look healthy: %v", report.ServiceBurnRate)
	}
	byName := map[string]v1.CustomerResult{}
	for _, item := range report.Items {
		byName[item.Name] = item
		if strings.Contains(item.Alias, "acme") || !strings.HasPrefix(item.Alias, "customer-") {
			t.Fatalf("alias must be opaque: %s", item.Alias)
		}
	}
	// A custom target changes the burn: 95% reliable against a 90% target is a 0.5x burn.
	if i := byName["initech"]; i.Target != .9 || i.BurnRate == nil || *i.BurnRate > .51 || i.MaskedByAggregate || i.State != "low_traffic" {
		t.Fatalf("per-customer target and low-traffic handling: %+v", i)
	}
	// No series is unknown, never healthy.
	if g := byName["ghost"]; g.State != "no_series" || g.Reliability != nil || g.BurnRate != nil || g.MaskedByAggregate {
		t.Fatalf("a customer with no data must not look healthy: %+v", g)
	}
	if byName["globex"].MaskedByAggregate || byName["globex"].State != "observed" {
		t.Fatalf("%+v", byName["globex"])
	}
}

func TestCustomerReportNeverGuessesOrAcceptsBadEvidence(t *testing.T) {
	s, source := customerFixture(t)
	good := 10.0
	missing := v1.CohortEvidence{At: now, WindowSeconds: 180, Counts: []v1.CohortCount{{Labels: map[string]string{"account": "acme-01"}, Total: 50}}}
	source.evidence = missing
	r, err := s.CustomerReport(context.Background(), "cust", 0)
	if err != nil || r.State != "available" || r.Items[len(r.Items)-1].State == "" {
		t.Fatalf("%+v %v", r, err)
	}
	var acme v1.CustomerResult
	for _, item := range r.Items {
		if item.Name == "Acme" {
			acme = item
		}
	}
	if acme.State != "missing_numerator" || acme.Reliability != nil {
		t.Fatalf("a missing numerator must stay unknown: %+v", acme)
	}
	for name, evidence := range map[string]v1.CohortEvidence{
		"wrong cutoff":       {At: now.Add(time.Hour), WindowSeconds: 180, Counts: nil},
		"wrong window":       {At: now, WindowSeconds: 60},
		"good above total":   {At: now, WindowSeconds: 180, Counts: []v1.CohortCount{{Labels: map[string]string{"account": "acme-01"}, Good: &good, Total: 5}}},
		"duplicate customer": {At: now, WindowSeconds: 180, Counts: []v1.CohortCount{{Labels: map[string]string{"account": "acme-01"}, Total: 5}, {Labels: map[string]string{"account": "acme-01"}, Total: 5}}},
		"extra labels":       {At: now, WindowSeconds: 180, Counts: []v1.CohortCount{{Labels: map[string]string{"account": "acme-01", "region": "x"}, Total: 5}}},
	} {
		source.evidence = evidence
		r, err = s.CustomerReport(context.Background(), "cust", 0)
		if err != nil || r.State != "unavailable" || len(r.Items) != 0 {
			t.Errorf("%s: %+v %v", name, r, err)
		}
	}
	if _, err = s.CustomerReport(context.Background(), "cust", 9); err == nil {
		t.Fatal("a stale expected revision was accepted")
	}
}

func TestCustomerReportStatesWhenNotConfiguredOrUnsupported(t *testing.T) {
	s, _ := cohortFixture(t) // an SLO without a customers policy
	r, err := s.CustomerReport(context.Background(), "cohort", 0)
	if err != nil || r.State != "not_configured" || len(r.Items) != 0 || len(r.Limitations) == 0 {
		t.Fatalf("%+v %v", r, err)
	}
}
