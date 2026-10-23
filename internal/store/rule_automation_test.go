package store_test

import (
	"context"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

func TestRuleAutomationPersistenceAndIsolation(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			var st store.Store = store.NewMemory()
			dsn := ""
			if pg {
				st, dsn = postgres(t)
			}
			ctx := context.Background()
			slo := model.SLO{ID: "auto-status", Project: "test", Name: "Auto", Service: "api", Target: .99, WindowSeconds: 3600, StepSeconds: 60}
			if e := st.Create(ctx, slo); e != nil {
				t.Fatal(e)
			}
			next := time.Now().UTC().Add(time.Minute)
			if e := st.Update(ctx, slo.ID, func(r *model.Record) error {
				r.RuleAutomation = &v1.RuleAutomationStatus{Enabled: true, Paused: true, Revision: 1, NextAttemptAt: &next, IntegrationStage: v1.IntegrationStage{State: "paused", Message: "Owner paused"}}
				return nil
			}); e != nil {
				t.Fatal(e)
			}
			if pg {
				p, e := store.NewPostgres(ctx, dsn)
				if e != nil {
					t.Fatal(e)
				}
				t.Cleanup(p.Close)
				st = p
			}
			reader := st.(store.RuleAutomationReader)
			state, e := reader.RuleAutomation(ctx, slo.ID)
			if e != nil || state == nil || !state.Paused || state.State != "paused" {
				t.Fatal(state, e)
			}
			state.Message = "tampered"
			state.NextAttemptAt = nil
			record, e := st.Read(ctx, slo.ID)
			if e != nil || record.RuleAutomation.Message != "Owner paused" || record.RuleAutomation.NextAttemptAt == nil {
				t.Fatal("mutable status escaped storage", record.RuleAutomation, e)
			}
			if e = st.Update(ctx, slo.ID, func(r *model.Record) error { r.Collection.State = "succeeded"; return nil }); e != nil {
				t.Fatal(e)
			}
			state, e = reader.RuleAutomation(ctx, slo.ID)
			if e != nil || !state.Paused {
				t.Fatal("collection lost automation pause", state, e)
			}
		})
	}
}
