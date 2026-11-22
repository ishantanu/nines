package service_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

type incidentFixture struct {
	t   *testing.T
	svc *service.Service
	now time.Time
}

func newIncidentFixture(t *testing.T) *incidentFixture {
	t.Helper()
	f := &incidentFixture{t: t, svc: service.New(store.NewMemory(), nil), now: time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)}
	f.svc.Now = func() time.Time { return f.now }
	f.svc.IncidentsEnabled = true
	return f
}

// slo creates an SLO with critical and warning policies and a 1-hour window.
func (f *incidentFixture) slo(id string) {
	f.t.Helper()
	critical := model.Policy{Name: "fast-burn", Severity: "critical", ShortWindowSeconds: 300, LongWindowSeconds: 3600, BurnRate: 14.4, MinCoverage: 1}
	warning := model.Policy{Name: "slow-burn", Severity: "warning", ShortWindowSeconds: 600, LongWindowSeconds: 3600, BurnRate: 2, MinCoverage: 1}
	if err := f.svc.Create(context.Background(), model.SLO{ID: id, Project: "shop", Name: id, Service: "api", Target: 0.99, WindowSeconds: 3600, StepSeconds: 60, Policies: []model.Policy{critical, warning}}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *incidentFixture) journey(id string, slos ...string) {
	f.t.Helper()
	links := ""
	steps := ""
	for _, s := range slos {
		links += "    - {id: " + s + ", dimension: availability, required: true}\n"
		steps += s + ","
	}
	manifest := "apiVersion: nines.dev/v1alpha1\nkind: CriticalUserJourney\nmetadata: {name: " + id + ", project: shop}\nspec:\n  displayName: " + id + "\n  description: Buy.\n  owner: team\n  criticality: high\n  slos:\n" + links + "  steps:\n    - {id: buy, name: Buy, actor: user, sloRefs: [" + steps[:len(steps)-1] + "]}\n"
	d, err := model.ParseDefinition([]byte(manifest))
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err = f.svc.ApplyDefinition(context.Background(), d, 0); err != nil {
		f.t.Fatal(err)
	}
}

// alert sets a policy's alert phase, as the engine would, and reconciles.
func (f *incidentFixture) alert(id, policy string, firing bool) {
	f.t.Helper()
	at := f.now
	if err := f.svc.Store.Update(context.Background(), id, func(r *model.Record) error {
		if r.Alert.Policies == nil {
			r.Alert.Policies = map[string]model.AlertState{}
		}
		state := model.AlertState{Phase: "inactive", Policy: policy}
		if firing {
			state.Phase, state.FiringSince = "firing", &at
		}
		for _, p := range r.SLO.Policies {
			if p.Name == policy {
				state.Severity = p.Severity
			}
		}
		r.Alert.Policies[policy] = state
		return nil
	}); err != nil {
		f.t.Fatal(err)
	}
	if err := f.svc.ReconcileIncidents(context.Background(), id); err != nil {
		f.t.Fatal(err)
	}
}

func (f *incidentFixture) incidents() []service.IncidentView {
	f.t.Helper()
	list, err := f.svc.ListIncidents(context.Background(), 50)
	if err != nil {
		f.t.Fatal(err)
	}
	return list
}

func TestCriticalAlertOpensOneIncidentAndWarningsDoNot(t *testing.T) {
	f := newIncidentFixture(t)
	f.slo("checkout")
	f.journey("purchase", "checkout")
	f.alert("checkout", "slow-burn", true)
	if len(f.incidents()) != 0 {
		t.Fatal("a warning alert opened an incident")
	}
	f.alert("checkout", "fast-burn", true)
	f.now = f.now.Add(time.Minute)
	f.alert("checkout", "fast-burn", true) // Reconciling again changes nothing.
	list := f.incidents()
	if len(list) != 1 {
		t.Fatalf("%d incidents", len(list))
	}
	i := list[0]
	if i.Source != "alert" || i.State != model.IncidentDetected || i.Revision != 1 || !i.OpenedAt.Equal(f.now.Add(-time.Minute)) {
		t.Fatalf("%+v", i.Incident)
	}
	// The revisions in force are recorded.
	if i.SLOs[0].ConfigRevision != 1 || i.SLOs[0].Policy != "fast-burn" || len(i.Journeys) != 1 || i.Journeys[0].ID != "purchase" || i.Journeys[0].Revision != 1 {
		t.Fatalf("%+v %+v", i.SLOs, i.Journeys)
	}
}

func TestOverlappingAlertsGroupByJourney(t *testing.T) {
	f := newIncidentFixture(t)
	for _, id := range []string{"checkout", "payments", "search"} {
		f.slo(id)
	}
	f.journey("purchase", "checkout", "payments")
	f.journey("browse", "search")
	f.alert("checkout", "fast-burn", true)
	f.alert("payments", "fast-burn", true)
	f.alert("search", "fast-burn", true)
	list := f.incidents()
	if len(list) != 2 {
		t.Fatalf("want one incident for purchase and one for browse, got %d", len(list))
	}
	for _, i := range list {
		if i.HasSLO("checkout") && (!i.HasSLO("payments") || len(i.Impact) != 2) {
			t.Fatal("alerts sharing a journey did not join one incident", i.SLOs)
		}
		if i.HasSLO("search") && len(i.SLOs) != 1 {
			t.Fatal("an unrelated alert joined", i.SLOs)
		}
	}
}

func TestFlappingAlertStaysOneIncident(t *testing.T) {
	f := newIncidentFixture(t)
	f.slo("checkout")
	f.alert("checkout", "fast-burn", true)
	for range 3 {
		f.now = f.now.Add(5 * time.Minute)
		f.alert("checkout", "fast-burn", false)
		if i := f.incidents()[0]; i.State != model.IncidentMonitoring || i.Firing() {
			t.Fatalf("cleared alert left %s", i.State)
		}
		f.now = f.now.Add(5 * time.Minute)
		f.alert("checkout", "fast-burn", true)
		if i := f.incidents()[0]; i.State != model.IncidentDetected || !i.Firing() {
			t.Fatalf("refired alert left %s", i.State)
		}
	}
	list := f.incidents()
	if len(list) != 1 || list[0].Revision != 7 {
		t.Fatalf("%d incidents at revision %d", len(list), list[0].Revision)
	}
	// After resolution, a new burn opens a new incident.
	i := list[0]
	if _, err := f.svc.TransitionIncident(context.Background(), i.ID, i.Revision, model.IncidentResolved, "ana", "Fixed."); err != nil {
		t.Fatal(err)
	}
	f.alert("checkout", "fast-burn", false)
	f.alert("checkout", "fast-burn", true)
	if len(f.incidents()) != 2 {
		t.Fatal("a burn after resolution did not open a new incident")
	}
}

func TestDeclaredIncidentCollectsLaterAlerts(t *testing.T) {
	f := newIncidentFixture(t)
	f.slo("checkout")
	declared, err := f.svc.DeclareIncident(context.Background(), "ana", "Checkout feels slow", []string{"checkout"})
	if err != nil || declared.Source != "manual" || declared.DeclaredBy != "ana" || declared.SLOs[0].Firing {
		t.Fatal(declared, err)
	}
	f.alert("checkout", "fast-burn", true)
	list := f.incidents()
	if len(list) != 1 || !list[0].SLOs[0].Firing || list[0].Timeline[len(list[0].Timeline)-1].Kind != "alert_fired" {
		t.Fatal("the alert did not join the declared incident", list)
	}
	if _, err = f.svc.DeclareIncident(context.Background(), "ana", "", nil); err == nil {
		t.Fatal("untitled incident accepted")
	}
	if _, err = f.svc.DeclareIncident(context.Background(), "ana", "x", []string{"missing"}); err == nil {
		t.Fatal("unknown SLO accepted")
	}
}

func TestIncidentTransitionsAreDecisionsAtARevision(t *testing.T) {
	f := newIncidentFixture(t)
	i, err := f.svc.DeclareIncident(context.Background(), "ana", "Outage", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = f.svc.TransitionIncident(ctx, i.ID, 1, model.IncidentMitigated, "ana", "Rolled back."); err != nil {
		t.Fatal(err)
	}
	// A responder acting on what they read earlier is refused.
	if _, err = f.svc.TransitionIncident(ctx, i.ID, 1, model.IncidentResolved, "ben", ""); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("stale transition accepted", err)
	}
	if _, err = f.svc.TransitionIncident(ctx, i.ID, 2, "closed", "ben", ""); err == nil {
		t.Fatal("unknown state accepted")
	}
	got, err := f.svc.TransitionIncident(ctx, i.ID, 2, model.IncidentResolved, "ben", "")
	if err != nil || got.ResolvedAt == nil || got.MitigatedAt == nil || got.Timeline[len(got.Timeline)-1].Actor != "ben" {
		t.Fatal(got, err)
	}
	if _, err = f.svc.TransitionIncident(ctx, i.ID, 3, model.IncidentDetected, "ben", ""); err == nil {
		t.Fatal("a resolved incident was reopened")
	}
}

// Impact uses the same arithmetic as status: bad events during the incident
// over the budget the window allowed at its end. Missing data is unknown.
func TestIncidentImpactMatchesStatusArithmetic(t *testing.T) {
	f := newIncidentFixture(t)
	f.slo("checkout")
	// One hour of 1000 requests a minute. 20 bad a minute for the last 10
	// minutes: the window allows 600 bad events, the incident had 200.
	var points []model.Point
	start := f.now.Add(-time.Hour)
	for m := 1; m <= 60; m++ {
		good := 1000.0
		if m > 50 {
			good = 980
		}
		points = append(points, model.Point{Time: start.Add(time.Duration(m) * time.Minute), Good: good, Total: 1000})
	}
	if err := f.svc.Ingest(context.Background(), "checkout", points); err != nil {
		t.Fatal(err)
	}
	declared, err := f.svc.DeclareIncident(context.Background(), "ana", "Errors", []string{"checkout"})
	if err != nil {
		t.Fatal(err)
	}
	// Backdate the opening to the start of the bad minutes.
	f.now = f.now.Add(0)
	if err := f.svc.Store.(store.IncidentStore).UpdateIncidents(context.Background(), func(open []model.Incident) ([]model.Incident, error) {
		i := open[0]
		i.OpenedAt = start.Add(50 * time.Minute)
		i.Revision++
		return []model.Incident{i}, nil
	}); err != nil {
		t.Fatal(err)
	}
	v, err := f.svc.Incident(context.Background(), declared.ID)
	if err != nil {
		t.Fatal(err)
	}
	impact := v.Impact[0]
	if impact.BadEvents == nil || *impact.BadEvents != 200 || impact.BudgetConsumedFraction == nil || math.Abs(*impact.BudgetConsumedFraction-200.0/600) > 1e-9 {
		t.Fatalf("%+v", impact)
	}
	status, err := f.svc.Status(context.Background(), "checkout", f.now)
	if err != nil || math.Abs(*status.BudgetAllowed-600) > 1e-9 {
		t.Fatal("status disagrees on the budget", status.BudgetAllowed, err)
	}
	// A minute of missing data during the incident makes the impact unknown.
	if err := f.svc.Store.Update(context.Background(), "checkout", func(r *model.Record) error {
		delete(r.Points, start.Add(55*time.Minute).Unix())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	v, _ = f.svc.Incident(context.Background(), declared.ID)
	if v.Impact[0].BudgetConsumedFraction != nil || v.Impact[0].Reason != "1 of 10 intervals during the incident have no data" {
		t.Fatalf("%+v", v.Impact[0])
	}
}

func TestIncidentsNeedTheGate(t *testing.T) {
	f := newIncidentFixture(t)
	f.svc.IncidentsEnabled = false
	f.slo("checkout")
	if err := f.svc.Store.Update(context.Background(), "checkout", func(r *model.Record) error {
		at := f.now
		r.Alert.Policies = map[string]model.AlertState{"fast-burn": {Phase: "firing", Severity: "critical", FiringSince: &at}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Evaluate(context.Background(), "checkout"); err != nil {
		t.Fatal(err)
	}
	if len(f.incidents()) != 0 {
		t.Fatal("incident opened with the feature off")
	}
}
