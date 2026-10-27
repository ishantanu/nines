package service

import (
	"context"
	"slices"
	"sort"
	"time"

	"github.com/ishantanu/nines/internal/model"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

type graphEvidenceSource interface {
	ObserveDependencyEvidence(context.Context, v1.GraphObservation, time.Time) ([]v1.ObservedEdge, bool, error)
}

func graphEvidence(o v1.GraphObservation, sourceDigest string, at time.Time, sample *v1.GraphSampleEvidence, state string) *v1.GraphEdgeEvidence {
	_, age := o.EvidenceTiming()
	out := &v1.GraphEdgeEvidence{Source: o.Source, SourceDigest: sourceDigest, Metric: "traces_service_graph_request_total", ReadAt: at, From: at.Add(-time.Duration(o.WindowSeconds) * time.Second), To: at, Freshness: state, MaxSampleAgeSeconds: age, InstrumentationCoverage: "unknown", Samples: sample}
	if sample != nil && sample.LastSampleAt != nil {
		seconds := int64(at.Sub(*sample.LastSampleAt) / time.Second)
		if seconds < 0 {
			out.Samples = nil
			return out
		}
		out.SampleAgeSeconds = &seconds
		out.Freshness = "fresh"
		if seconds > age {
			out.Freshness = "stale"
		}
	}
	return out
}

func (s *Service) attachGraphJourneySteps(ctx context.Context, nodes map[string]v1.DependencyNode) error {
	if s.Journeys == nil {
		return nil
	}
	cache := map[string]model.JourneyRevision{}
	names := make([]string, 0, len(nodes))
	for name := range nodes {
		names = append(names, name)
	}
	sort.Strings(names)
	detailCount := 0
	for _, name := range names {
		node := nodes[name]
		if len(node.Journeys) > 10 {
			node.Journeys = node.Journeys[:10]
			node.JourneyLinksTruncated = true
		}
		ids := make([]string, 0, len(node.Objectives))
		for _, slo := range node.Objectives {
			ids = append(ids, slo.ID)
		}
		for i, link := range node.Journeys {
			resource, ok := cache[link.ID]
			if !ok {
				if len(cache) >= 128 {
					node.JourneyLinksTruncated = true
					continue
				}
				var err error
				resource, err = s.Journeys.JourneyRevision(ctx, link.ID, link.Revision)
				if err != nil {
					return err
				}
				cache[link.ID] = resource
			}
			for _, step := range resource.Manifest.Spec.Steps {
				matched := false
				for _, ref := range step.SLORefs {
					if slices.Contains(ids, ref) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
				if len(node.Journeys[i].Steps) >= 5 || detailCount >= 100 {
					node.JourneyLinksTruncated = true
					continue
				}
				node.Journeys[i].Steps = append(node.Journeys[i].Steps, v1.MapJourneyStep{ID: step.ID, Name: step.Name})
				detailCount++
			}
		}
		nodes[name] = node
	}
	return nil
}
