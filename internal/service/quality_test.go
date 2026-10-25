package service

import (
	"context"
	"errors"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

func TestQualityDistinguishesFreshCountsLowVolumeAndStaleness(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		if err := s.Ingest(ctx, "availability", []model.Point{{Time: now.Add(-time.Duration(i) * time.Minute), Good: 100, Total: 100}}); err != nil {
			t.Fatal(err)
		}
	}
	q, err := s.MeasurementQuality(ctx, "availability", 1, false)
	if err != nil || q.State != "usable" || q.Total != 1000 || q.Coverage != 1 || q.Freshness.State != "fresh" {
		t.Fatal(q, err)
	}
	if err = s.Store.Update(ctx, "availability", func(r *model.Record) error { r.SLO.Quality = &v1.MeasurementQualityPolicy{MinEvents: 1001}; return nil }); err != nil {
		t.Fatal(err)
	}
	q, err = s.MeasurementQuality(ctx, "availability", 1, false)
	if err != nil || q.State != "limited" || q.Total != 1000 || q.MinimumEvents != 1001 {
		t.Fatal(q, err)
	}
	s.Now = func() time.Time { return now.Add(3 * time.Minute) }
	q, err = s.MeasurementQuality(ctx, "availability", 1, false)
	if err != nil || q.State != "unavailable" || q.Freshness.State != "stale" {
		t.Fatal(q, err)
	}
	if _, err = s.MeasurementQuality(ctx, "availability", 2, false); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("stale read accepted", err)
	}
}
func TestQualityZeroTrafficNeverBecomesUsableEvidence(t *testing.T) {
	s := setup(t)
	if err := s.Ingest(context.Background(), "availability", []model.Point{{Time: now}}); err != nil {
		t.Fatal(err)
	}
	q, err := s.MeasurementQuality(context.Background(), "availability", 0, false)
	if err != nil || q.State != "unavailable" || q.Total != 0 {
		t.Fatal(q, err)
	}
}

type qualityCohortSource struct {
	evidence v1.CohortEvidence
	hook     func()
	err      error
	calls    int
}

func (f *qualityCohortSource) Collect(context.Context, model.SLO, time.Time, time.Time) ([]model.Point, error) {
	return nil, nil
}
func (f *qualityCohortSource) ReadCohorts(context.Context, model.SLO, []string, time.Time) (v1.CohortEvidence, error) {
	f.calls++
	if f.hook != nil {
		f.hook()
	}
	return f.evidence, f.err
}
func cohortFixture(t *testing.T) (*Service, *qualityCohortSource) {
	t.Helper()
	ctx := context.Background()
	source := &qualityCohortSource{}
	s := New(store.NewMemory(), source)
	s.Now = func() time.Time { return now.Add(time.Minute) }
	c := model.SLO{ID: "cohort", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 180, StepSeconds: 60, ProviderWindow: &v1.ProviderWindow{GoodSelector: "good_total", TotalSelector: "request_total", ScrapeIntervalSeconds: 15}, Quality: &v1.MeasurementQualityPolicy{MinEvents: 100, CohortLabels: []string{"region"}}}
	if err := s.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Update(ctx, c.ID, func(r *model.Record) error {
		r.Windows[model.WindowKey(180, now)] = model.WindowObservation{Time: now, WindowSeconds: 180, Revision: 1, Good: 999, Total: 1000, Coverage: 1}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	good, none := 999.0, 0.0
	source.evidence = v1.CohortEvidence{At: now, WindowSeconds: 180, Counts: []v1.CohortCount{{Labels: map[string]string{"region": "healthy"}, Good: &good, Total: 999}, {Labels: map[string]string{"region": "affected"}, Good: &none, Total: 1}}}
	return s, source
}
func TestCohortInspectionIsExplicitAndRevealsAggregateMasking(t *testing.T) {
	s, source := cohortFixture(t)
	ctx := context.Background()
	if _, err := s.MeasurementQuality(ctx, "cohort", 1, false); err != nil || source.calls != 0 {
		t.Fatal("stored quality triggered provider read", err)
	}
	q, err := s.MeasurementQuality(ctx, "cohort", 1, true)
	if err != nil || source.calls != 1 || q.Cohorts.State != "available" || !q.Cohorts.MatchesAggregate || len(q.Cohorts.Items) != 2 {
		t.Fatal(q, err)
	}
	first := q.Cohorts.Items[0]
	if first.Labels["region"] != "affected" || first.Reliability == nil || *first.Reliability != 0 || first.State != "low_traffic" {
		t.Fatal(first)
	}
	source.evidence.Counts[1].Good = nil
	q, err = s.MeasurementQuality(ctx, "cohort", 1, true)
	if err != nil || q.Cohorts.MatchesAggregate || q.Cohorts.Items[0].Reliability != nil || q.Cohorts.Items[0].State != "missing_numerator" {
		t.Fatal(q, err)
	}
}
func TestCohortInvalidOrExcessEvidenceNeverLeaksPartialRanking(t *testing.T) {
	for _, kind := range []string{"duplicate", "invalid_count", "timestamp", "cardinality"} {
		t.Run(kind, func(t *testing.T) {
			s, source := cohortFixture(t)
			switch kind {
			case "duplicate":
				source.evidence.Counts = append(source.evidence.Counts, source.evidence.Counts[1])
			case "invalid_count":
				source.evidence.Counts[1].Total = -1
			case "timestamp":
				source.evidence.At = now.Add(time.Second)
			case "cardinality":
				for len(source.evidence.Counts) < 101 {
					source.evidence.Counts = append(source.evidence.Counts, source.evidence.Counts[0])
				}
			}
			q, err := s.MeasurementQuality(context.Background(), "cohort", 1, true)
			if err != nil || q.Cohorts.State == "available" || len(q.Cohorts.Items) != 0 {
				t.Fatal(q, err)
			}
		})
	}
}
func TestCohortReadRejectsDefinitionChangedDuringQuery(t *testing.T) {
	s, source := cohortFixture(t)
	source.hook = func() {
		if err := s.Store.Update(context.Background(), "cohort", func(r *model.Record) error { r.SLO.Revision++; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.MeasurementQuality(context.Background(), "cohort", 1, true); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal(err)
	}
}

func TestCohortReconciliationDoesNotHideOneFailedEventInLargePopulation(t *testing.T) {
	s, source := cohortFixture(t)
	ctx := context.Background()
	if err := s.Store.Update(ctx, "cohort", func(r *model.Record) error {
		r.Windows[model.WindowKey(180, now)] = model.WindowObservation{Time: now, Revision: 1, WindowSeconds: 180, Good: 1e9 - 1, Total: 1e9, Coverage: 1}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	good := 1e9
	source.evidence.Counts = []v1.CohortCount{{Labels: map[string]string{"region": "a"}, Good: &good, Total: 1e9}}
	q, err := s.MeasurementQuality(ctx, "cohort", 1, true)
	if err != nil || q.Cohorts.State != "available" || q.Cohorts.MatchesAggregate {
		t.Fatal("broad relative tolerance hid the failed event", q, err)
	}
}

type lineageCollector struct {
	fingerprint string
	hook        func()
	fail        bool
}

func (f *lineageCollector) SourceFingerprint() string { return f.fingerprint }
func (f *lineageCollector) Collect(_ context.Context, s model.SLO, from, to time.Time) ([]model.Point, error) {
	if f.hook != nil {
		f.hook()
	}
	if f.fail {
		return nil, errors.New("private upstream error")
	}
	var result []model.Point
	for at := from.Add(time.Duration(s.StepSeconds) * time.Second); !at.After(to); at = at.Add(time.Duration(s.StepSeconds) * time.Second) {
		result = append(result, model.Point{Time: at, Good: 100, Total: 100})
	}
	return result, nil
}
func TestBudgetRequiresCompleteCurrentSourceLineage(t *testing.T) {
	ctx := context.Background()
	source := &lineageCollector{fingerprint: "route-a"}
	s := New(store.NewMemory(), source)
	s.Now = func() time.Time { return now.Add(time.Minute) }
	c := model.SLO{ID: "lineage", Project: "p", Service: "api", Name: "API", Target: .99, StepSeconds: 60, WindowSeconds: 180, GoodQuery: "vector(100)", TotalQuery: "vector(100)", ErrorBudgetPolicy: &v1.ErrorBudgetPolicy{Owner: "oncall", RunbookURL: "https://example.test/runbook", ReviewBelow: .25}}
	if err := s.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	collect := func(from time.Time) {
		t.Helper()
		if _, err := s.Collect(ctx, c.ID, from, now); err != nil {
			t.Fatal(err)
		}
	}
	collect(now.Add(-3 * time.Minute))
	d, err := s.ErrorBudgetDecision(ctx, c.ID, 1)
	if err != nil || d.State != "proceed" {
		t.Fatal(d, err)
	}
	source.fingerprint = "route-b"
	collect(now.Add(-time.Minute))
	d, err = s.ErrorBudgetDecision(ctx, c.ID, 1)
	if err != nil || d.State != "insufficient_evidence" {
		t.Fatal("mixed source samples authorized proceeding", d, err)
	}
	collect(now.Add(-3 * time.Minute))
	d, err = s.ErrorBudgetDecision(ctx, c.ID, 1)
	if err != nil || d.State != "proceed" {
		t.Fatal("full backfill did not verify lineage", d, err)
	}
	if err = s.Ingest(ctx, c.ID, []model.Point{{Time: now, Good: 100, Total: 100}}); err != nil {
		t.Fatal(err)
	}
	d, err = s.ErrorBudgetDecision(ctx, c.ID, 1)
	if err != nil || d.State != "insufficient_evidence" {
		t.Fatal("manual edit retained provider lineage", d, err)
	}
	collect(now.Add(-3 * time.Minute))
	source.hook = func() { source.fingerprint = "route-c" }
	if _, err = s.Collect(ctx, c.ID, now.Add(-time.Minute), now); err == nil {
		t.Fatal("source change during collection accepted")
	}
	saved, _ := s.Store.Read(ctx, c.ID)
	if saved.Collection.SourceDigest != "route-b" || saved.Collection.State != "failed" {
		t.Fatal("failed query relabeled retained samples", saved.Collection)
	}
}
