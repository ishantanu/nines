package service

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/engine"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/store"
)

var errMeasurementContract = fmt.Errorf("measurement contract mismatch; revalidate the binding before collecting: %w", model.ErrUnavailable)

type Collector interface {
	Collect(context.Context, model.SLO, time.Time, time.Time) ([]model.Point, error)
}
type MetricSource struct {
	Kind      string
	Collector Collector
}

type Service struct {
	RuleManager   *rules.Manager
	DynamicSource func(string) (MetricSource, bool)
	IntentDrafter IntentDrafter
	DefinitionMu  sync.Mutex
	Sources       map[string]MetricSource
	Store         store.Store
	Journeys      store.JourneyStore
	Demo          bool
	TenantID      string
	TenantName    string
	SourceKind    string
	Collector     Collector
	Now           func() time.Time
	collectionMu  sync.Mutex
}

func New(st store.Store, c Collector) *Service {
	journeys, _ := st.(store.JourneyStore)
	kind := "manual"
	if c != nil {
		kind = "prometheus"
	}
	return &Service{Store: st, Journeys: journeys, Collector: c, Now: time.Now, TenantID: "default", TenantName: "Default workspace", SourceKind: kind}
}
func (s *Service) Create(ctx context.Context, c model.SLO) error {
	if c.Measurement != nil && c.ProviderWindow == nil {
		c.ProviderWindow = model.MeasurementWindow(*c.Measurement)
	}
	if err := c.Validate(); err != nil {
		return err
	}
	if err := s.ValidateSource(c); err != nil {
		return err
	}
	if err := s.validateBindingReference(ctx, c); err != nil {
		return err
	}
	c.Revision = 1
	c.CreatedAt = s.Now().UTC()
	return s.Store.Create(ctx, c)
}
func aligned(t time.Time, step int64) bool {
	return !t.IsZero() && t.Nanosecond() == 0 && t.Unix()%step == 0
}

const collectionInterval = 15 * time.Second

func (s *Service) nominalEvaluationAt(c model.SLO, now time.Time) time.Time {
	if s.CollectorFor(c) != nil && c.HasQueries() {
		now = now.Add(-time.Duration(c.StepSeconds) * time.Second)
	}
	return now.UTC().Truncate(time.Duration(c.StepSeconds) * time.Second)
}
func (s *Service) evaluationAt(c model.SLO) time.Time { return s.nominalEvaluationAt(c, s.Now()) }

// During the bounded collector scheduling interval, use its last completed
// cutoff rather than describing an interval it has not queried as missing.
// Failed, overdue, empty-result and historical reads never borrow old samples.
func (s *Service) recordEvaluationAt(r model.Record, now time.Time) time.Time {
	at := s.nominalEvaluationAt(r.SLO, now)
	h := r.Collection
	if s.CollectorFor(r.SLO) == nil || !r.SLO.HasQueries() || h.State != "succeeded" || h.Points <= 0 || h.SLORevision != model.ConfigRevision(r.SLO) || h.CompletedAt.IsZero() || now.Before(h.CompletedAt) || now.Sub(h.CompletedAt) > collectionInterval {
		return at
	}
	step := time.Duration(r.SLO.StepSeconds) * time.Second
	grace := ((collectionInterval + step - 1) / step) * step
	if !h.To.IsZero() && h.To.Before(at) && at.Sub(h.To) <= grace && aligned(h.To, r.SLO.StepSeconds) {
		return h.To
	}
	return at
}

func (s *Service) Status(ctx context.Context, id string, at time.Time) (model.Snapshot, error) {
	r, err := s.Store.Read(ctx, id)
	if err != nil {
		return model.Snapshot{}, err
	}
	return s.statusFromRecord(r, at)
}

// SLOView pairs a definition and status calculated from the same stored record.
type SLOView struct {
	MeasurementHealth *v1.BindingHealth `json:"measurementHealth,omitempty"`
	SLO               model.SLO         `json:"slo"`
	Status            model.Snapshot    `json:"status"`
}

func (s *Service) ReadSLO(ctx context.Context, id string) (SLOView, error) {
	r, err := s.Store.Read(ctx, id)
	if err != nil {
		return SLOView{}, err
	}
	status, err := s.statusFromRecord(r, time.Time{})
	if err != nil {
		return SLOView{}, err
	}
	view := SLOView{SLO: r.SLO, Status: status}
	if check, ok := s.measurementHealth(r); ok {
		view.MeasurementHealth = &check
	}
	return view, nil
}

func (s *Service) statusFromRecord(r model.Record, at time.Time) (model.Snapshot, error) {
	current := s.recordEvaluationAt(r, s.Now())
	if at.IsZero() {
		at = current
	}
	if at.After(s.Now()) {
		return model.Snapshot{}, model.Invalid("evaluation time cannot be in the future")
	}
	result := engine.EvaluateRecord(r, at, r.SLO.WindowSeconds)
	if at.Equal(current) {
		if health, ok := s.measurementHealth(r); ok && health.EvaluationBlocked {
			result.Reliability = nil
			result.BurnRate = nil
			result.BudgetAllowed = nil
			result.BudgetRemaining = nil
			result.BudgetRemainingFraction = nil
			result.Coverage = 0
			result.DataStatus = "measurement_drift"
		}
	}
	// Historical evaluations do not attach today's alert state to past data.
	if result.At.Equal(current) {
		result.Alert = r.Alert
	} else {
		result.Alert = model.AlertState{Phase: "historical"}
	}
	return result, nil
}
func (s *Service) Samples(ctx context.Context, id string) ([]model.Point, error) {
	r, err := s.Store.Read(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]model.Point, 0, len(r.Points))
	for _, p := range r.Points {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out, nil
}
func (s *Service) Ingest(ctx context.Context, id string, points []model.Point) error {
	if len(points) == 0 || len(points) > 10000 {
		return model.Invalid("provide 1–10000 samples")
	}
	return s.Store.Update(ctx, id, func(r *model.Record) error {
		if r.SLO.ProviderWindow != nil {
			return model.Invalid("provider-window SLOs require collection, not interval ingestion")
		}
		return s.apply(r, points, nil)
	})
}
func (s *Service) apply(r *model.Record, points []model.Point, replace *[2]time.Time) error {
	now := s.Now().UTC().Truncate(time.Duration(r.SLO.StepSeconds) * time.Second)
	cutoff := now.Add(-2 * time.Duration(r.SLO.WindowSeconds) * time.Second)
	seen := map[int64]bool{}
	for _, p := range points {
		if err := p.Validate(r.SLO.StepSeconds); err != nil {
			return err
		}
		if p.Time.After(now) {
			return model.Invalid("cannot ingest future intervals")
		}
		if !p.Time.After(cutoff) {
			return model.Invalid("sample is outside retention (two rolling windows)")
		}
		if seen[p.Time.Unix()] {
			return model.Invalid("duplicate timestamps within the same request")
		}
		seen[p.Time.Unix()] = true
	}
	for k := range r.Points {
		if k <= cutoff.Unix() {
			delete(r.Points, k)
		}
	}
	if replace != nil {
		for k := range r.Points {
			if k > replace[0].Unix() && k <= replace[1].Unix() {
				delete(r.Points, k)
			}
		}
	}
	for _, p := range points {
		p.Time = p.Time.UTC()
		r.Points[p.Time.Unix()] = p
	}
	if len(r.Points) > 200000 {
		return model.Invalid("SLO sample limit exceeded (200000)")
	}
	s.advance(r, s.recordEvaluationAt(*r, s.Now()))
	return nil
}
func (s *Service) advance(r *model.Record, at time.Time) {
	if health, ok := s.measurementHealth(*r); ok && health.EvaluationBlocked {
		return
	}
	state, events := engine.AdvanceAllRecord(*r, at)
	r.Alert = state
	r.Events = append(r.Events, events...)
}
func (s *Service) Evaluate(ctx context.Context, id string) error {
	return s.Store.Update(ctx, id, func(r *model.Record) error { s.advance(r, s.recordEvaluationAt(*r, s.Now())); return nil })
}

func (s *Service) Collect(ctx context.Context, id string, from, to time.Time) (count int, collectErr error) {
	ctx, finish := observability.Operation(ctx, "slo.collect", s.TenantID)
	defer func() { finish(collectErr) }()

	// The first release runs one collector at a time, bounding provider load and
	// preventing overlapping replay/live queries in a single service process.
	s.collectionMu.Lock()
	defer s.collectionMu.Unlock()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	r, err := s.Store.Read(ctx, id)
	if err != nil {
		return 0, err
	}
	c := r.SLO
	if s.CollectorFor(c) == nil {
		return 0, model.ErrUnavailable
	}
	if !c.HasQueries() {
		return 0, model.Invalid("this SLO has no Prometheus queries")
	}
	if !aligned(from, c.StepSeconds) || !aligned(to, c.StepSeconds) || !to.After(from) || to.Sub(from) > 31*24*time.Hour || to.After(s.Now().UTC().Truncate(time.Duration(c.StepSeconds)*time.Second)) {
		return 0, model.Invalid("collection requires aligned from < to, at most 31 days, ending in the past")
	}
	if from.Before(s.Now().UTC().Truncate(time.Duration(c.StepSeconds) * time.Second).Add(-2 * time.Duration(c.WindowSeconds) * time.Second)) {
		return 0, model.Invalid("collection range exceeds retention (two rolling windows)")
	}
	attempted := s.Now().UTC()
	var points []model.Point
	var windows []model.WindowObservation
	blocked := false
	if check, ok := s.measurementHealth(r); ok && check.EvaluationBlocked {
		blocked = true
	}
	if blocked {
		err = errMeasurementContract
	} else if c.ProviderWindow != nil {
		collector, ok := s.CollectorFor(c).(WindowCollector)
		if !ok {
			err = fmt.Errorf("source does not support provider-window collection")
		} else {
			windows, err = collector.CollectWindows(ctx, c, from, to)
		}
	} else {
		points, err = s.CollectorFor(c).Collect(ctx, c, from, to)
	}
	health := v1.CollectionHealth{State: "succeeded", AttemptedAt: attempted, CompletedAt: s.Now().UTC(), From: from, To: to, Source: c.Source, SLORevision: model.ConfigRevision(c), Points: len(points) + len(windows)}
	failure := func(code string, cause error) (int, error) {
		health.State = "failed"
		health.ErrorCode = code
		health.Points = 0
		persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if e := s.Store.Update(persist, id, func(r *model.Record) error {
			if model.ConfigRevision(r.SLO) != model.ConfigRevision(c) {
				return nil // A late failure belongs to the old definition, not the new measurement.
			}
			if !r.Collection.AttemptedAt.After(health.AttemptedAt) {
				health.LastSuccessAt = r.Collection.LastSuccessAt
				health.Measurement = r.Collection.Measurement
				r.Collection = health
			}
			return nil
		}); e != nil {
			return 0, fmt.Errorf("collection failed and health persistence failed: %w", e)
		}
		return 0, fmt.Errorf("collection failed: %w", cause)
	}
	if err == errMeasurementContract {
		return failure("measurement_contract_mismatch", err)
	}
	if err != nil {
		return failure("provider_error", err)
	}
	for _, p := range points {
		if !p.Time.After(from) || p.Time.After(to) {
			return failure("invalid_provider_samples", fmt.Errorf("collector returned samples outside requested interval"))
		}
	}
	rangeToReplace := [2]time.Time{from, to}
	err = s.Store.Update(ctx, id, func(r *model.Record) error {
		if check, ok := s.measurementHealth(*r); ok && check.EvaluationBlocked {
			return errMeasurementContract
		}
		if !r.Collection.AttemptedAt.After(health.AttemptedAt) {
			health.LastSuccessAt = health.CompletedAt
			health.Measurement = r.Collection.Measurement
			r.Collection = health
		}
		if model.ConfigRevision(r.SLO) != model.ConfigRevision(c) {
			return model.ErrRevisionConflict
		}
		if c.ProviderWindow != nil {
			return s.applyWindows(r, windows, from, to)
		}
		return s.apply(r, points, &rangeToReplace)
	})
	if err != nil {
		if err == errMeasurementContract {
			return failure("measurement_contract_mismatch", err)
		}
		return failure("collection_commit_failed", err)
	}
	return len(points) + len(windows), nil
}

func (s *Service) CollectorFor(c model.SLO) Collector {
	if c.Source == "" || c.Source == "primary" {
		return s.Collector
	}
	if source, ok := s.Sources[c.Source]; ok {
		return source.Collector
	}
	if s.DynamicSource != nil {
		source, _ := s.DynamicSource(c.Source)
		return source.Collector
	}
	return nil
}
func (s *Service) ValidateSource(c model.SLO) error {
	if c.Source != "" && c.Source != "primary" {
		if _, ok := s.Sources[c.Source]; !ok {
			if s.DynamicSource != nil {
				if _, found := s.DynamicSource(c.Source); found {
					return nil
				}
			}
			return model.Invalid("unknown source connection %q", c.Source)
		}
	}
	return nil
}
