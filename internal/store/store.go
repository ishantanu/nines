package store

import (
	"context"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

// Update serializes a single SLO and commits samples, alert state and new
// outbox events atomically. Callbacks must perform no network operations.
type Store interface {
	Create(context.Context, model.SLO) error
	List(context.Context) ([]model.SLO, error)
	Read(context.Context, string) (model.Record, error)
	Update(context.Context, string, func(*model.Record) error) error
	Deliveries(context.Context) ([]model.Delivery, error)
	Claim(context.Context, time.Time, time.Duration) (*model.Delivery, error)
	Finish(context.Context, string, int, time.Time, string) error
	Ping(context.Context) error
	Close()
}

func clone(r model.Record) model.Record {
	c := r
	if r.SLO.Retirement != nil {
		v := copyJSON(*r.SLO.Retirement)
		c.SLO.Retirement = &v
	}
	if r.SLO.ErrorBudgetPolicy != nil {
		p := copyJSON(*r.SLO.ErrorBudgetPolicy)
		c.SLO.ErrorBudgetPolicy = &p
	}
	if r.SLO.Quality != nil {
		q := copyJSON(*r.SLO.Quality)
		c.SLO.Quality = &q
	}
	if r.RuleAutomation != nil {
		v := copyJSON(*r.RuleAutomation)
		c.RuleAutomation = &v
	}
	if r.SLO.Measurement != nil {
		measured := copyJSON(*r.SLO.Measurement)
		c.SLO.Measurement = &measured
	}
	if r.Collection.Measurement != nil {
		health := *r.Collection.Measurement
		c.Collection.Measurement = &health
	}
	if r.SLO.Intent != nil {
		i := copyJSON(*r.SLO.Intent)
		c.SLO.Intent = &i
	}
	if r.SLO.MeasurementBinding != nil {
		b := *r.SLO.MeasurementBinding
		c.SLO.MeasurementBinding = &b
	}
	c.Windows = make(map[string]model.WindowObservation, len(r.Windows))
	for k, v := range r.Windows {
		c.Windows[k] = v
	}
	if r.SLO.ProviderWindow != nil {
		p := *r.SLO.ProviderWindow
		c.SLO.ProviderWindow = &p
	}
	c.Points = make(map[int64]model.Point, len(r.Points))
	for k, v := range r.Points {
		c.Points[k] = v
	}
	c.SLO.Policies = append([]model.Policy(nil), r.SLO.Policies...)
	c.Alert.Policies = cloneAlertPolicies(r.Alert.Policies)
	if r.SLO.Policy != nil {
		p := *r.SLO.Policy
		c.SLO.Policy = &p
	}
	if r.Alert.PendingSince != nil {
		t := *r.Alert.PendingSince
		c.Alert.PendingSince = &t
	}
	if r.Alert.LastEvaluated != nil {
		t := *r.Alert.LastEvaluated
		c.Alert.LastEvaluated = &t
	}
	c.Events = nil
	return c
}
