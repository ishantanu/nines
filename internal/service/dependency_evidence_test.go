package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

type qualifiedGraphSource struct {
	testtelemetry.Source
	at  time.Time
	age time.Duration
}

func (q *qualifiedGraphSource) ObserveDependencyEvidence(context.Context, v1.GraphObservation, time.Time) ([]v1.ObservedEdge, bool, error) {
	last := q.at.Add(-q.age)
	continuity := .5
	return []v1.ObservedEdge{{Caller: "api", Callee: "database", Requests: 42, Evidence: &v1.GraphSampleEvidence{LastSampleAt: &last, Samples: 30, Series: 1, SampleContinuity: &continuity, ResolutionSeconds: 15}}}, false, nil
}
func TestDependencyEvidenceDistinguishesReadingFromFreshSamples(t *testing.T) {
	ctx := context.Background()
	for _, age := range []time.Duration{15 * time.Second, 5 * time.Minute} {
		s, _, e := testtelemetry.New()
		if e != nil {
			t.Fatal(e)
		}
		at := time.Unix(1800000000, 0).UTC()
		q := &qualifiedGraphSource{at: at, age: age}
		s.Collector = q
		s.SourceKind = "prometheus"
		s.Now = func() time.Time { return at }
		view, e := s.DependencyView(ctx, "services", 1)
		if e != nil {
			t.Fatal(e)
		}
		if view.InstrumentationCoverage != "unknown" || view.DeclaredEdges != 1 || view.MatchedDeclaredEdges != 1 {
			t.Fatal("missing graph scope evidence", view)
		}
		for _, edge := range view.Edges {
			if edge.Alignment != "matched" || edge.Evidence == nil || edge.Evidence.SampleAgeSeconds == nil || *edge.Evidence.SampleAgeSeconds != int64(age/time.Second) {
				t.Fatal("wrong edge evidence", edge)
			}
			expected := "fresh"
			if age > 45*time.Second {
				expected = "stale"
			}
			if edge.Evidence.Freshness != expected {
				t.Fatal("stale evidence reported fresh", edge)
			}
		}
	}
}

type changingGraphSource struct {
	testtelemetry.Source
	edges  []v1.ObservedEdge
	fail   bool
	change bool
}

func (q *changingGraphSource) ObserveDependencyEvidence(context.Context, v1.GraphObservation, time.Time) ([]v1.ObservedEdge, bool, error) {
	if q.change {
		q.Route = "changed-after-read"
	}
	if q.fail {
		return nil, false, errors.New("private-provider-failure")
	}
	return q.edges, false, nil
}
func (q *changingGraphSource) SourceFingerprint() string { return q.Route }
func TestDependencyEvidenceMissingRenamedAndBoundedFrontiers(t *testing.T) {
	for _, scenario := range []string{"missing", "renamed", "large", "failed", "changed"} {
		t.Run(scenario, func(t *testing.T) {
			s, _, e := testtelemetry.New()
			if e != nil {
				t.Fatal(e)
			}
			q := &changingGraphSource{}
			q.Route = "original"
			s.Collector = q
			s.SourceKind = "prometheus"
			switch scenario {
			case "renamed":
				q.edges = []v1.ObservedEdge{{Caller: "api-v2", Callee: "database-v2", Requests: 10}}
			case "large":
				for i := 0; i < 150; i++ {
					q.edges = append(q.edges, v1.ObservedEdge{Caller: fmt.Sprintf("caller-%d", i), Callee: fmt.Sprintf("callee-%d", i), Requests: 1})
				}
			case "failed":
				q.fail = true
			case "changed":
				q.change = true
				q.edges = []v1.ObservedEdge{{Caller: "api", Callee: "database", Requests: 1}}
			}
			view, e := s.DependencyView(context.Background(), "services", 1)
			if e != nil {
				t.Fatal(e)
			}
			if view.MatchedDeclaredEdges != 0 || view.InstrumentationCoverage != "unknown" {
				t.Fatal("missing evidence established coverage or a match", view)
			}
			if scenario == "large" && (!view.Truncated || len(view.Nodes) > 80 || len(view.Edges) > 101) {
				t.Fatal("unbounded dependency response", len(view.Nodes), len(view.Edges), view.Truncated)
			}
			expected := "not_seen"
			if scenario == "failed" || scenario == "changed" {
				expected = "unavailable"
				if view.ObservationState != "unavailable" {
					t.Fatal("failed or changed source accepted", view)
				}
			}
			for _, edge := range view.Edges {
				if edge.Provenance == "declared" && (edge.Evidence == nil || edge.Evidence.Freshness != expected) {
					t.Fatal("missing declared link lost qualification", edge)
				}
				if edge.Provenance == "observed" && scenario == "renamed" && edge.Alignment != "observed_only" {
					t.Fatal("renamed service silently matched", edge)
				}
			}
			raw, _ := json.Marshal(view)
			if strings.Contains(string(raw), "private-provider-failure") {
				t.Fatal("provider error leaked")
			}
		})
	}
}
