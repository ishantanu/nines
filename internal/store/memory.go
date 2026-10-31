package store

import (
	"context"
	"sort"
	"sync"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

type Memory struct {
	visual     map[string]v1.VisualEvaluation
	tombstones map[string]v1.RetirementReceipt
	batches    map[string]definitionReceipt
	archives   map[string]map[int64]SLOArchive
	mu         sync.Mutex
	topology   map[string][]model.TopologyRevision
	journeys   map[string][]model.JourneyRevision
	receipts   map[string]journeyReceipt
	runs       map[string]map[string]model.JourneyRun
	records    map[string]model.Record
	deliveries map[string]model.Delivery
}

func NewMemory() *Memory {
	return &Memory{records: map[string]model.Record{}, deliveries: map[string]model.Delivery{}}
}
func (m *Memory) Create(ctx context.Context, s model.SLO) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, deleted := m.tombstones[s.ID]; deleted {
		return model.ErrConflict
	}
	if _, ok := m.records[s.ID]; ok {
		return model.ErrConflict
	}
	m.records[s.ID] = clone(model.Record{SLO: s, Points: map[int64]model.Point{}, Alert: model.AlertState{Phase: "inactive"}})
	return nil
}
func (m *Memory) List(ctx context.Context) ([]model.SLO, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := make([]model.SLO, 0, len(m.records))
	for _, r := range m.records {
		out = append(out, clone(r).SLO)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (m *Memory) Read(ctx context.Context, id string) (model.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return model.Record{}, err
	}
	r, ok := m.records[id]
	if !ok {
		return model.Record{}, model.ErrNotFound
	}
	return clone(r), nil
}
func (m *Memory) Update(ctx context.Context, id string, fn func(*model.Record) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	r, ok := m.records[id]
	if !ok {
		return model.ErrNotFound
	}
	next := clone(r)
	if err := fn(&next); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if model.ConfigRevision(next.SLO) != model.ConfigRevision(r.SLO) {
		archive := archiveSLO(ctx, r)
		if m.archives == nil {
			m.archives = map[string]map[int64]SLOArchive{}
		}
		if m.archives[id] == nil {
			m.archives[id] = map[int64]SLOArchive{}
		}
		if _, exists := m.archives[id][archive.Summary.Revision]; exists {
			return model.ErrRevisionConflict
		}
		m.archives[id][archive.Summary.Revision] = archive
	}
	if !r.SLO.Retired() && next.SLO.Retired() {
		for key, d := range m.deliveries {
			if d.Event.SLOID == id && d.Status == "pending" {
				d.Status = "failed"
				d.LastError = "slo_archived"
				m.deliveries[key] = d
			}
		}
	}
	for _, e := range next.Events {
		m.deliveries[e.ID] = model.Delivery{Event: e, Status: "pending", NextAttempt: e.Time}
	}
	next.Events = nil
	m.records[id] = clone(next)
	return nil
}
func (m *Memory) Deliveries(ctx context.Context) ([]model.Delivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := make([]model.Delivery, 0, len(m.deliveries))
	for _, v := range m.deliveries {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Event.Time.Equal(out[j].Event.Time) {
			return out[i].Event.ID < out[j].Event.ID
		}
		return out[i].Event.Time.After(out[j].Event.Time)
	})
	if len(out) > 200 {
		out = out[:200]
	}
	return out, nil
}
func (m *Memory) Claim(ctx context.Context, now time.Time, lease time.Duration) (*model.Delivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var chosen *model.Delivery
	for _, d := range m.deliveries {
		if d.Status == "pending" && !d.NextAttempt.After(now) && (chosen == nil || d.NextAttempt.Before(chosen.NextAttempt) || (d.NextAttempt.Equal(chosen.NextAttempt) && d.Event.ID < chosen.Event.ID)) {
			v := d
			chosen = &v
		}
	}
	if chosen == nil {
		return nil, nil
	}
	chosen.Attempts++
	chosen.NextAttempt = now.Add(lease)
	m.deliveries[chosen.Event.ID] = *chosen
	return chosen, nil
}

// The attempt number is a fencing token: an expired worker cannot overwrite
// the result of a later lease holder. Delivery is at least once.
func (m *Memory) Finish(ctx context.Context, id string, attempt int, next time.Time, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	d, ok := m.deliveries[id]
	if !ok {
		return model.ErrNotFound
	}
	if d.Attempts != attempt || d.Status != "pending" {
		return nil
	}
	d.LastError = message
	d.NextAttempt = next
	if message == "" {
		d.Status = "sent"
	} else if attempt >= 8 {
		d.Status = "failed"
	}
	m.deliveries[id] = d
	return nil
}
func (m *Memory) Ping(ctx context.Context) error { return ctx.Err() }
func (m *Memory) Close()                         {}

// OperatorRoute reads deployment metadata without cloning measurement history.
func (m *Memory) OperatorRoute(ctx context.Context, id string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	record, ok := m.records[id]
	if !ok {
		return "", model.ErrNotFound
	}
	return record.OperatorRoute, nil
}
