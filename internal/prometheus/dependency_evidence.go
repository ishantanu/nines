package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// ObserveDependencyEvidence adds a separately qualified metadata query to the
// bounded traffic estimate. Metadata failure never turns old counters into
// fresh traffic or hides the fact that only part of the evidence is available.
func (c *Client) ObserveDependencyEvidence(ctx context.Context, o v1.GraphObservation, at time.Time) ([]v1.ObservedEdge, bool, error) {
	if err := model.ValidateGraphObservation(o); err != nil {
		return nil, false, err
	}
	edges, truncated, err := c.ObserveDependencies(ctx, o, at)
	if err != nil || len(edges) == 0 {
		return edges, truncated, err
	}
	metadata, limited, err := c.graphMetadata(ctx, o, at)
	if err != nil {
		return edges, truncated, nil
	}
	for i, e := range edges {
		if evidence, ok := metadata[graphKey(e.Caller, e.Callee, e.ConnectionType)]; ok {
			copy := evidence
			edges[i].Evidence = &copy
		}
	}
	return edges, truncated || limited, nil
}
func graphKey(caller, callee, kind string) string { return caller + "\x00" + callee + "\x00" + kind }
func (c *Client) graphMetadata(ctx context.Context, o v1.GraphObservation, at time.Time) (map[string]v1.GraphSampleEvidence, bool, error) {
	interval, _ := o.EvidenceTiming()
	resolution := max(interval, (o.WindowSeconds+239)/240)
	selector := Selector("traces_service_graph_request_total", o.Labels)
	by := "(client,server,connection_type)"
	expression := func(kind, query string) string {
		return fmt.Sprintf(`label_replace(topk(101, %s), "nines_graph_evidence", %q, "", "")`, query, kind)
	}
	samples := fmt.Sprintf("sum by %s (count_over_time(%s[%ds]))", by, selector, o.WindowSeconds)
	series := fmt.Sprintf("count by %s (count_over_time(%s[%ds]))", by, selector, o.WindowSeconds)
	last := fmt.Sprintf("max by %s (max_over_time(timestamp(%s)[%ds:%ds]))", by, selector, o.WindowSeconds, resolution)
	query := expression("samples", samples) + " or " + expression("series", series) + " or " + expression("last", last)
	var data struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []json.RawMessage `json:"value"`
		} `json:"result"`
	}
	if err := c.telemetryRequest(ctx, "query", url.Values{"query": {query}, "time": {strconv.FormatInt(at.Unix(), 10)}, "timeout": {"5s"}, "limit": {"303"}}, &data); err != nil {
		return nil, false, err
	}
	if data.ResultType != "vector" || len(data.Result) > 303 {
		return nil, false, fmt.Errorf("invalid bounded graph metadata")
	}
	type entry struct {
		samples v1.GraphSampleEvidence
		kinds   map[string]bool
	}
	rows := map[string]*entry{}
	counts := map[string]int{}
	for _, r := range data.Result {
		caller, callee, kind := r.Metric["client"], r.Metric["server"], r.Metric["connection_type"]
		if !graphLabel(caller, 200) || !graphLabel(callee, 200) || len(kind) > 100 || !graphLabel(kind, 100) && kind != "" || len(r.Value) != 2 {
			return nil, false, fmt.Errorf("invalid graph metadata labels")
		}
		var timestamp float64
		var raw string
		if json.Unmarshal(r.Value[0], &timestamp) != nil || timestamp != float64(at.Unix()) || json.Unmarshal(r.Value[1], &raw) != nil {
			return nil, false, fmt.Errorf("invalid graph metadata timestamp")
		}
		value, e := strconv.ParseFloat(raw, 64)
		if e != nil || !model.Finite(value) || value < 0 {
			return nil, false, fmt.Errorf("invalid graph metadata value")
		}
		field := r.Metric["nines_graph_evidence"]
		key := graphKey(caller, callee, kind)
		row := rows[key]
		if row == nil {
			row = &entry{kinds: map[string]bool{}, samples: v1.GraphSampleEvidence{ResolutionSeconds: resolution}}
			rows[key] = row
		}
		if row.kinds[field] {
			return nil, false, fmt.Errorf("duplicate graph metadata")
		}
		row.kinds[field] = true
		counts[field]++
		switch field {
		case "samples":
			if value > 1e9 || value != math.Trunc(value) {
				return nil, false, fmt.Errorf("invalid sample count")
			}
			row.samples.Samples = value
		case "series":
			if value > 1e6 || value != math.Trunc(value) {
				return nil, false, fmt.Errorf("invalid series count")
			}
			row.samples.Series = int64(value)
		case "last":
			if value > float64(at.Unix()) || value < float64(at.Add(-time.Duration(o.WindowSeconds+300)*time.Second).Unix()) {
				return nil, false, fmt.Errorf("invalid sample age")
			}
			last := time.Unix(int64(value), int64((value-math.Trunc(value))*1e9)).UTC()
			row.samples.LastSampleAt = &last
		default:
			return nil, false, fmt.Errorf("unexpected graph metadata field")
		}
	}
	out := map[string]v1.GraphSampleEvidence{}
	for key, row := range rows {
		if len(row.kinds) != 3 || row.samples.Series < 1 || row.samples.Samples < 1 {
			continue
		}
		continuity := math.Min(1, row.samples.Samples/(float64(row.samples.Series)*math.Ceil(float64(o.WindowSeconds)/float64(interval))))
		row.samples.SampleContinuity = &continuity
		out[key] = row.samples
	}
	return out, counts["samples"] > 100 || counts["series"] > 100 || counts["last"] > 100, nil
}
func graphLabel(s string, max int) bool {
	if len(s) == 0 || len(s) > max {
		return false
	}
	for _, r := range s {
		if r == '\x00' || r == '\r' || r == '\n' {
			return false
		}
	}
	return true
}
