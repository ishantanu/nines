package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

type intentContextSource struct{ calls int }

func (c *intentContextSource) Collect(context.Context, model.SLO, time.Time, time.Time) ([]model.Point, error) {
	c.calls++
	return nil, errors.New("must not collect")
}
func (c *intentContextSource) MetricNames(context.Context, string, time.Time) ([]string, bool, error) {
	c.calls++
	return nil, false, errors.New("must not discover")
}

func TestIntentContextScopeEvidenceAndNoProviderCalls(t *testing.T) {
	s := setup(t)
	s.TenantID = "alpha"
	upstream := &intentContextSource{}
	s.Collector = upstream
	s.Sources = map[string]MetricSource{"secondary": {Kind: "mimir", Collector: upstream}}
	ctx := context.Background()
	for _, c := range []model.SLO{
		{ID: "same-service-other-source", Project: "p", Service: "api", Name: "Other source", Source: "secondary", Target: .99, WindowSeconds: 600, StepSeconds: 60},
		{ID: "different-service", Project: "p", Service: "worker", Name: "Other service", Target: .99, WindowSeconds: 600, StepSeconds: 60},
		{ID: "different-project", Project: "private", Service: "api", Name: "Other project", Target: .99, WindowSeconds: 600, StepSeconds: 60},
	} {
		if err := s.Create(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	out, err := s.IntentContext(ctx, v1.IntentContextInput{Service: "api", Source: "primary", Project: "p"})
	if err != nil || out.Tenant != "alpha" || len(out.Objectives) != 1 || out.Objectives[0].Resource.ID != "availability" {
		t.Fatal(out, err)
	}
	if upstream.calls != 0 || !out.Source.Configured || !out.Source.MetricNameDiscovery {
		t.Fatal("context contacted a provider or lost capabilities", upstream.calls, out.Source)
	}
	if out.Objectives[0].Measurement.Reliability != nil || out.TextTrust != resourceTextTrust {
		t.Fatal("missing data or resource trust changed", out)
	}
	if _, err = s.IntentContext(ctx, v1.IntentContextInput{Service: "api", Source: "missing", Project: "p"}); !errors.Is(err, model.ErrNotFound) {
		t.Fatal(err)
	}
}
func TestIntentContextBoundedPagesAndPinnedScope(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	s.TenantID = "alpha"
	for i := 0; i < 102; i++ {
		service := "worker"
		if i == 101 {
			service = "api"
		}
		if err := s.Create(ctx, model.SLO{ID: fmt.Sprintf("a-%03d", i), Project: "p", Service: service, Name: "Synthetic", Target: .99, WindowSeconds: 600, StepSeconds: 60}); err != nil {
			t.Fatal(err)
		}
	}
	in := v1.IntentContextInput{Service: "api", Source: "primary", Project: "p", Limit: 1}
	out, err := s.IntentContext(ctx, in)
	if err != nil || len(out.Objectives) != 0 || out.NextCursor == "" {
		t.Fatal(out, err)
	}
	cursor := out.NextCursor
	in.Cursor = cursor
	out, err = s.IntentContext(ctx, in)
	if err != nil || len(out.Objectives) != 1 || out.Objectives[0].Resource.ID != "a-101" || out.NextCursor == "" {
		t.Fatal(out, err)
	}
	in.Cursor = out.NextCursor
	out, err = s.IntentContext(ctx, in)
	if err != nil || len(out.Objectives) != 1 || out.Objectives[0].Resource.ID != "availability" || out.NextCursor != "" {
		t.Fatal(out, err)
	}
	for _, change := range []string{"tenant", "service", "source", "project"} {
		copy := v1.IntentContextInput{Service: "api", Source: "primary", Project: "p", Cursor: cursor}
		switch change {
		case "tenant":
			s.TenantID = "beta"
		case "service":
			copy.Service = "worker"
		case "source":
			copy.Source = "secondary"
			s.Sources = map[string]MetricSource{"secondary": {Kind: "mimir"}}
		case "project":
			copy.Project = "other"
		}
		if _, err = s.IntentContext(ctx, copy); err == nil {
			t.Fatal("cursor scope escaped", change)
		}
		s.TenantID = "alpha"
	}
	for _, bad := range []v1.IntentContextInput{{Service: "api", Source: "primary"}, {Service: strings.Repeat("x", 201), Source: "primary", Project: "p"}, {Service: "api", Source: "primary", Project: "p", Limit: 11}, {Service: "api", Source: "primary", Project: "p", Journey: "journey"}, {Service: "api", Source: "primary", Project: "p", JourneyRevision: 1}} {
		if _, err = s.IntentContext(ctx, bad); err == nil {
			t.Fatal("invalid context accepted", bad)
		}
	}
}
func TestIntentContextJourneyRequiresCurrentProjectRevision(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	j := model.Journey{APIVersion: v1.APIVersion, Kind: v1.JourneyKind, Metadata: model.JourneyMetadata{Name: "checkout", Project: "p"}, Spec: model.JourneySpec{DisplayName: "Checkout", Description: "Synthetic", Owner: "team", Criticality: "critical", SLOs: []model.JourneySLO{{ID: "availability", Dimension: "availability", Required: true}}, Steps: []model.JourneyStep{{ID: "start", Name: "Begin", Actor: "user", SLORefs: []string{"availability"}}}}}
	if _, err := s.ApplyJourney(ctx, j, 0); err != nil {
		t.Fatal(err)
	}
	in := v1.IntentContextInput{Service: "api", Source: "primary", Project: "p", Journey: "checkout", JourneyRevision: 1}
	out, err := s.IntentContext(ctx, in)
	if err != nil || out.Journey == nil || out.Journey.Resource.Revision != 1 || out.Journey.LinkedSLOs != 1 {
		t.Fatal(out, err)
	}
	in.JourneyRevision = 2
	if _, err = s.IntentContext(ctx, in); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal(err)
	}
	in.JourneyRevision = 1
	in.Project = "other"
	if _, err = s.IntentContext(ctx, in); err == nil {
		t.Fatal("cross-project journey accepted")
	}
}

func TestIntentContextPreservesExistingServiceAndProjectNames(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	c := model.SLO{ID: "unicode", Service: "checkout.api/v2", Project: "Production Europe", Name: "Synthetic", Target: .99, WindowSeconds: 600, StepSeconds: 60}
	if err := s.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	out, err := s.IntentContext(ctx, v1.IntentContextInput{Service: c.Service, Source: "primary", Project: c.Project})
	if err != nil || len(out.Objectives) != 1 || out.Objectives[0].Resource.ID != "unicode" {
		t.Fatal(out, err)
	}
}
