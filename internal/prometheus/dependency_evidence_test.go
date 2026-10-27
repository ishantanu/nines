package prometheus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestGraphEvidenceKeepsActualSampleTimeAndContinuityQualified(t *testing.T) {
	at := time.Unix(1800000000, 0).UTC()
	for _, scenario := range []string{"complete", "partial", "stale", "unavailable", "future", "duplicate"} {
		t.Run(scenario, func(t *testing.T) {
			requests := 0
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Header.Get("X-Scope-OrgID") != "trusted" || r.Header.Get("Authorization") != "Bearer trusted-fixture" {
					t.Error("lost pinned provider authentication")
				}
				query := r.URL.Query().Get("query")
				var results []any
				item := func(kind, value string) map[string]any {
					return map[string]any{"metric": map[string]string{"client": "api", "server": "database", "connection_type": "database", "nines_graph_evidence": kind}, "value": []any{at.Unix(), value}}
				}
				if strings.Contains(query, "nines_graph_evidence") {
					if r.URL.Query().Get("limit") != "303" || !strings.Contains(query, `environment="production"`) {
						t.Error("unbounded metadata query")
					}
					if scenario == "unavailable" {
						http.Error(w, "private-provider-error", http.StatusServiceUnavailable)
						return
					}
					count, last := "60", "1799999985"
					if scenario == "partial" {
						count = "30"
					}
					if scenario == "stale" {
						last = "1799999700"
					}
					if scenario == "future" {
						last = "1800000010"
					}
					results = []any{item("samples", count), item("series", "1"), item("last", last)}
					if scenario == "duplicate" {
						results = append(results, item("last", last))
					}
				} else {
					results = []any{item("", "42")}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": results}})
			}))
			defer source.Close()
			c, e := NewMimir(source.URL, "trusted", "trusted-fixture")
			if e != nil {
				t.Fatal(e)
			}
			edges, truncated, e := c.ObserveDependencyEvidence(context.Background(), v1.GraphObservation{WindowSeconds: 900, Labels: map[string]string{"environment": "production"}}, at)
			if e != nil || truncated || len(edges) != 1 || edges[0].Requests != 42 || requests != 2 {
				t.Fatal("lost bounded traffic evidence", edges, e, requests)
			}
			evidence := edges[0].Evidence
			if scenario == "unavailable" || scenario == "future" || scenario == "duplicate" {
				if evidence != nil {
					t.Fatal("unsafe metadata treated as verified")
				}
				return
			}
			if evidence == nil || evidence.LastSampleAt == nil || evidence.Series != 1 || evidence.SampleContinuity == nil {
				t.Fatal("missing sample evidence")
			}
			continuity := 1.0
			if scenario == "partial" {
				continuity = .5
			}
			if *evidence.SampleContinuity != continuity {
				t.Fatal("wrong continuity", *evidence.SampleContinuity)
			}
			if scenario == "stale" && at.Sub(*evidence.LastSampleAt) != 300*time.Second {
				t.Fatal("query evaluation time replaced raw sample age")
			}
		})
	}
}
