package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/prometheus"
	"github.com/ishantanu/nines/internal/store"
)

var ErrTelemetry = errors.New("configured telemetry source could not supply bounded, complete evidence")

type TelemetrySource interface {
	MetricNames(context.Context, string, time.Time) ([]string, bool, error)
	Discover(context.Context, string, map[string]string, time.Time) (v1.TelemetryDiscovery, error)
	ObserveDependencies(context.Context, v1.GraphObservation, time.Time) ([]v1.ObservedEdge, bool, error)
}

func (s *Service) telemetrySource(source string) (TelemetrySource, error) {
	if !validResourceID(source) {
		return nil, model.Invalid("invalid configured source name")
	}
	c, ok := s.CollectorFor(model.SLO{Source: source}).(TelemetrySource)
	if !ok {
		return nil, model.ErrUnavailable
	}
	return c, nil
}
func (s *Service) DiscoverMetricNames(ctx context.Context, source, prefix string) (v1.MetricNames, error) {
	out := v1.MetricNames{APIVersion: v1.APIVersion, Tenant: s.TenantID, Source: source, ObservedAt: s.Now().UTC().Truncate(time.Second)}
	c, err := s.telemetrySource(source)
	if err != nil {
		return out, err
	}
	out.Names, out.Truncated, err = c.MetricNames(ctx, prefix, out.ObservedAt)
	if err != nil {
		var invalid *model.ValidationError
		if errors.As(err, &invalid) {
			return out, err
		}
		return out, ErrTelemetry
	}
	return out, nil
}
func (s *Service) DiscoverTelemetry(ctx context.Context, source, metric string, labels map[string]string) (v1.TelemetryDiscovery, error) {
	if !model.MetricName.MatchString(metric) {
		return v1.TelemetryDiscovery{}, model.Invalid("invalid metric name")
	}
	if err := model.ValidateLabels(labels); err != nil {
		return v1.TelemetryDiscovery{}, err
	}
	c, err := s.telemetrySource(source)
	if err != nil {
		return v1.TelemetryDiscovery{}, err
	}
	out, err := c.Discover(ctx, metric, labels, s.Now().UTC().Truncate(time.Second))
	if err != nil {
		return out, ErrTelemetry
	}
	out.APIVersion = v1.APIVersion
	out.Tenant = s.TenantID
	out.Source = source
	out.Fingerprint = model.DefinitionDigest(struct {
		Source, Metric string
		Metadata       []v1.TelemetryMetadata
		Series         []map[string]string
	}{source, out.Metric, out.Metadata, out.Series})
	return out, nil
}
func (s *Service) topologyStore() (store.TopologyStore, error) {
	st, ok := s.Store.(store.TopologyStore)
	if !ok {
		return nil, model.ErrUnavailable
	}
	return st, nil
}
func (s *Service) ReadTopology(ctx context.Context, kind, id string, revision int64) (model.TopologyRevision, error) {
	if (kind != v1.BindingKind && kind != v1.DependencyMapKind) || !validResourceID(id) || revision < 0 {
		return model.TopologyRevision{}, model.Invalid("invalid topology resource")
	}
	st, err := s.topologyStore()
	if err != nil {
		return model.TopologyRevision{}, err
	}
	return st.ReadTopology(ctx, kind, id, revision)
}
func (s *Service) validateTopology(ctx context.Context, d model.Definition) error {
	if d.Binding != nil {
		if err := d.Binding.Validate(); err != nil {
			return err
		}
		_, err := s.telemetrySource(d.Binding.Spec.Source)
		return err
	}
	if d.DependencyMap == nil {
		return model.Invalid("topology definition required")
	}
	g := d.DependencyMap
	if err := g.Validate(); err != nil {
		return err
	}
	if g.Spec.Observed != nil {
		if _, err := s.telemetrySource(g.Spec.Observed.Source); err != nil {
			return err
		}
	}
	for _, node := range g.Spec.Services {
		for _, id := range node.SLORefs {
			r, err := s.Store.Read(ctx, id)
			if err != nil {
				return err
			}
			if r.SLO.Service != node.Name {
				return model.Invalid("SLO %s belongs to service %s, not %s", id, r.SLO.Service, node.Name)
			}
		}
	}
	return nil
}
func (s *Service) planTopology(ctx context.Context, d model.Definition) (ResourcePlan, error) {
	p := ResourcePlan{APIVersion: v1.APIVersion, Kind: d.Kind(), Manifest: d.Manifest(), Digest: model.DefinitionDigest(d.Manifest()), Action: "create"}
	if err := s.validateTopology(ctx, d); err != nil {
		return p, err
	}
	old, err := s.ReadTopology(ctx, d.Kind(), d.Metadata().Name, 0)
	if errors.Is(err, model.ErrNotFound) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	p.ExpectedRevision = old.Revision
	p.Previous = ResourceRevision{old.Manifest, old.Revision, old.Digest}
	p.Action = "update"
	if p.Digest == old.Digest {
		p.Action = "unchanged"
	}
	return p, nil
}
func (s *Service) applyTopology(ctx context.Context, d model.Definition, expected int64) (ResourceRevision, error) {
	if err := s.validateTopology(ctx, d); err != nil {
		return ResourceRevision{}, err
	}
	st, err := s.topologyStore()
	if err != nil {
		return ResourceRevision{}, err
	}
	b, err := json.Marshal(d.Manifest())
	if err != nil {
		return ResourceRevision{}, err
	}
	next := model.TopologyRevision{Manifest: b, Kind: d.Kind(), ID: d.Metadata().Name, Digest: model.DefinitionDigest(d.Manifest()), AppliedAt: s.Now().UTC()}
	r, err := st.ApplyTopology(ctx, next, expected)
	return ResourceRevision{r.Manifest, r.Revision, r.Digest}, err
}

type topologyCursor struct{ Tenant, Kind, After string }

func (s *Service) ListTopology(ctx context.Context, kind, cursor string, limit int) (v1.TopologyCatalog, error) {
	out := v1.TopologyCatalog{APIVersion: v1.APIVersion, Tenant: s.TenantID, Items: []v1.TopologyResource{}}
	if kind != "" && kind != v1.BindingKind && kind != v1.DependencyMapKind {
		return out, model.Invalid("invalid topology kind")
	}
	if limit == 0 {
		limit = 25
	}
	if limit < 1 || limit > 100 {
		return out, model.Invalid("limit must be 1–100")
	}
	after := ""
	if cursor != "" {
		var c topologyCursor
		b, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || len(b) > 512 || json.Unmarshal(b, &c) != nil || c.Tenant != s.TenantID || c.Kind != kind || len(c.After) > 100 {
			return out, model.Invalid("invalid or mismatched cursor")
		}
		after = c.After
	}
	st, err := s.topologyStore()
	if err != nil {
		return out, err
	}
	items, err := st.ListTopology(ctx, kind, after, limit+1)
	if err != nil {
		return out, err
	}
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		b, _ := json.Marshal(topologyCursor{s.TenantID, kind, last.Kind + "/" + last.ID})
		out.NextCursor = base64.RawURLEncoding.EncodeToString(b)
	}
	for _, r := range items {
		var header struct {
			Spec struct {
				DisplayName string `json:"displayName"`
			} `json:"spec"`
		}
		if err = json.Unmarshal(r.Manifest, &header); err != nil {
			return out, err
		}
		out.Items = append(out.Items, v1.TopologyResource{Kind: r.Kind, ID: r.ID, DisplayName: header.Spec.DisplayName, Revision: r.Revision, Digest: r.Digest, AppliedAt: r.AppliedAt})
	}
	return out, nil
}

// BindingQueries emits disjoint count windows, never rates, quantiles or overlapping windows.
func BindingQueries(b model.MeasurementBinding) (good, total string) {
	s := b.Spec
	population := map[string]string{}
	for k, v := range s.Labels {
		population[k] = v
	}
	metric := s.Metric
	if s.Measurement == "latency" || s.CountFrom == "histogram" {
		metric += "_count"
	}
	count := func(name string, labels map[string]string) string {
		return fmt.Sprintf("sum(increase(%s[%ds]))", prometheus.Selector(name, labels), s.StepSeconds)
	}
	total = count(metric, population)
	if s.Measurement == "latency" {
		population["le"] = strconv.FormatFloat(*s.ThresholdSeconds, 'g', -1, 64)
		good = count(s.Metric+"_bucket", population)
	} else {
		for k, v := range s.SuccessLabels {
			population[k] = v
		}
		good = count(metric, population)
	}
	return
}

// BindingWindow pins the same populations as the legacy queries, but leaves
// the duration to the evaluator instead of embedding a short interval.
func BindingWindow(b model.MeasurementBinding) *v1.ProviderWindow {
	s := b.Spec
	return model.MeasurementWindow(v1.MeasurementSpec{Type: s.Measurement, Metric: s.Metric, Labels: s.Labels, SuccessLabels: s.SuccessLabels, CountFrom: s.CountFrom, ThresholdSeconds: s.ThresholdSeconds, ScrapeIntervalSeconds: s.ScrapeIntervalSeconds, ObservationPoint: s.ObservationPoint})
}
func verifyBindingEvidence(b model.MeasurementBinding, d v1.TelemetryDiscovery) error {
	if d.Truncated || len(d.Series) == 0 || len(d.Metadata) != 1 {
		return model.Invalid("compilation requires one unambiguous metadata type and nonempty, untruncated series evidence; refine labels or fix instrumentation")
	}
	meta := d.Metadata[0]
	s := b.Spec
	if s.Measurement == "availability" {
		expectedType := "counter"
		countMetric := s.Metric
		if s.CountFrom == "histogram" {
			expectedType = "histogram"
			countMetric += "_count"
		}
		if meta.Type != expectedType {
			return model.Invalid("availability countFrom requires %s metadata", expectedType)
		}
		success := false
		for _, series := range d.Series {
			if series["__name__"] != countMetric {
				continue
			}
			match := true
			for k, v := range s.SuccessLabels {
				if value, ok := series[k]; !ok || value != v {
					match = false
				}
			}
			success = success || match
		}
		if !success {
			return model.Invalid("successLabels do not match an observed population series")
		}
		return nil
	}
	if meta.Type != "histogram" || !strings.HasSuffix(s.Metric, "_seconds") || (meta.Unit != "" && meta.Unit != "seconds") {
		return model.Invalid("latency requires a classic histogram with a _seconds name and seconds units")
	}
	// Every population must expose both a count and the exact canonical threshold bucket.
	// This rejects missing boundaries instead of interpolating or converting a quantile.
	groups := map[string]int{}
	boundary := strconv.FormatFloat(*s.ThresholdSeconds, 'g', -1, 64)
	for _, series := range d.Series {
		labels := map[string]string{}
		for k, v := range series {
			if k != "__name__" && k != "le" {
				labels[k] = v
			}
		}
		key := model.DefinitionDigest(labels)
		flag := 0
		switch series["__name__"] {
		case s.Metric + "_count":
			flag = 1
		case s.Metric + "_bucket":
			if series["le"] == boundary {
				flag = 2
			}
			if series["le"] == "+Inf" {
				flag = 4
			}
		}
		groups[key] |= flag
	}
	for _, flags := range groups {
		if flags != 7 {
			return model.Invalid("every histogram population needs _count, +Inf and the exact threshold bucket %s; native histograms and interpolated boundaries are unsupported", boundary)
		}
	}
	return nil
}
func (s *Service) CompileBinding(ctx context.Context, id string, in v1.CompileBindingInput) (v1.CompiledBinding, error) {
	out := v1.CompiledBinding{APIVersion: v1.APIVersion, Tenant: s.TenantID, CompilerVersion: "provider-windows/1"}
	if in.ExpectedRevision < 1 {
		return out, model.Invalid("expectedRevision must be positive")
	}
	r, err := s.ReadTopology(ctx, v1.BindingKind, id, 0)
	if err != nil {
		return out, err
	}
	if r.Revision != in.ExpectedRevision {
		return out, model.ErrRevisionConflict
	}
	var b model.MeasurementBinding
	if err = json.Unmarshal(r.Manifest, &b); err != nil {
		return out, err
	}
	if err = b.Validate(); err != nil {
		return out, err
	}
	out.Definition = v1.MeasurementBinding(b)
	out.Binding = v1.BindingReference{Name: id, Revision: r.Revision, Digest: r.Digest}
	if b.Spec.ScrapeIntervalSeconds == 0 {
		return out, model.Invalid("set scrapeIntervalSeconds on the binding before compiling provider windows")
	}
	window := BindingWindow(b)
	out.Manifest = v1.NewSLO(in.Name, b.Metadata.Project, v1.SLOSpec{MeasurementBinding: &out.Binding, DisplayName: in.DisplayName, Service: b.Spec.Service, Source: b.Spec.Source, Target: in.Target, WindowSeconds: in.WindowSeconds, StepSeconds: b.Spec.StepSeconds, ProviderWindow: window, Policies: in.Policies, Policy: in.Policy})
	def := model.SLODefinition(out.Manifest)
	if err = def.Validate(); err != nil {
		return out, err
	}
	out.Evidence, err = s.DiscoverTelemetry(ctx, b.Spec.Source, b.Spec.Metric, b.Spec.Labels)
	if err != nil {
		return out, err
	}
	if err = verifyBindingEvidence(b, out.Evidence); err != nil {
		return out, err
	}
	latest, err := s.ReadTopology(ctx, v1.BindingKind, id, 0)
	if err != nil {
		return out, err
	}
	if latest.Revision != r.Revision {
		return out, model.ErrRevisionConflict
	}
	encoded, err := yaml.Marshal(out.Manifest)
	if err != nil {
		return out, err
	}
	out.YAML = string(encoded)
	out.Digest = model.DefinitionDigest(out.Manifest)
	out.Warnings = []string{"Review observation point, population and success semantics before applying the SLO YAML.", "Full-window counter increases remain scrape-based estimates; they are not exact event logs.", "Coverage measures the least-observed population's scrape density and freshness; sparse/new/retired series can reduce coverage. Uninstrumented populations cannot be inferred.", "Changing a measurement keeps the SLO name and creates a new revision. Previous definitions and retained evidence are archived; active measurement coverage starts fresh."}
	return out, nil
}
func (s *Service) validateBindingReference(ctx context.Context, c model.SLO) error {
	if c.MeasurementBinding == nil {
		return nil
	}
	ref := c.MeasurementBinding
	r, err := s.ReadTopology(ctx, v1.BindingKind, ref.Name, ref.Revision)
	if err != nil {
		return err
	}
	if ref.Digest != r.Digest {
		return model.Invalid("measurement binding digest does not match its immutable revision")
	}
	var b model.MeasurementBinding
	if err = json.Unmarshal(r.Manifest, &b); err != nil {
		return err
	}
	if c.Intent != nil && !model.IntentMatchesBinding(v1.SLOIntent{Metadata: v1.Metadata{Project: c.Project}, Spec: *c.Intent}, b) {
		return model.Invalid("intent does not match its referenced binding")
	}
	// Historical interval manifests remain valid against their immutable binding.
	good, total := BindingQueries(b)
	matches := c.GoodQuery == good && c.TotalQuery == total && c.ProviderWindow == nil
	if c.ProviderWindow != nil {
		matches = c.GoodQuery == "" && c.TotalQuery == "" && model.DefinitionDigest(c.ProviderWindow) == model.DefinitionDigest(BindingWindow(b))
	}
	if c.Project != b.Metadata.Project || c.Service != b.Spec.Service || sourceID(c.Source) != sourceID(b.Spec.Source) || c.StepSeconds != b.Spec.StepSeconds || !matches {
		return model.Invalid("SLO measurement differs from its referenced binding")
	}
	return nil
}
func (s *Service) DependencyView(ctx context.Context, id string, expected int64) (v1.DependencyView, error) {
	out := v1.DependencyView{APIVersion: v1.APIVersion, Tenant: s.TenantID, ID: id, ObservedAt: s.Now().UTC().Truncate(time.Second), Nodes: []v1.DependencyNode{}, Edges: []v1.DependencyEdge{}, ObservationState: "not_configured", Warnings: []string{"Arrows run from caller to dependency. Component SLOs and dependency edges do not measure end-to-end journey success or establish causality."}}
	r, err := s.ReadTopology(ctx, v1.DependencyMapKind, id, 0)
	if err != nil {
		return out, err
	}
	if expected < 0 {
		return out, model.Invalid("expectedRevision must be nonnegative")
	}
	if expected > 0 && expected != r.Revision {
		return out, model.ErrRevisionConflict
	}
	out.Revision = r.Revision
	var g model.ServiceDependencyMap
	if err = json.Unmarshal(r.Manifest, &g); err != nil {
		return out, err
	}
	if err = g.Validate(); err != nil {
		return out, err
	}
	nodes := map[string]v1.DependencyNode{}
	for _, n := range g.Spec.Services {
		node := v1.DependencyNode{Name: n.Name, DisplayName: n.DisplayName, Declared: true, Health: "unknown", Objectives: []v1.MapObjective{}, Journeys: []v1.MapJourney{}}
		journeys := map[string]bool{}
		health := "healthy"
		for _, id := range n.SLORefs {
			c, err := s.SLOContext(ctx, id, v1.ContextOptions{})
			if err != nil {
				return out, err
			}
			h := c.Health
			if c.Freshness.State != "fresh" || c.Collection.State == "failed" || c.Collection.State == "unconfigured" {
				h = "unknown"
			}
			if c.Resource.Service != n.Name {
				h = "unknown"
				out.Warnings = append(out.Warnings, "Service assignment changed for SLO "+id+"; review the map definition.")
			}
			node.Objectives = append(node.Objectives, v1.MapObjective{ID: id, Revision: c.Resource.Revision, DisplayName: c.Resource.DisplayName, Health: h, Reliability: c.Measurement.Reliability, Target: c.Measurement.Target, DataStatus: c.Measurement.DataStatus, Freshness: c.Freshness.State, CollectionState: c.Collection.State})
			if healthRank(h) > healthRank(health) {
				health = h
			}
			links, err := s.SearchResources(ctx, v1.SearchOptions{Kind: v1.JourneyKind, LinkedSLO: id, Limit: 10})
			if err != nil {
				return out, err
			}
			node.JourneyLinksTruncated = node.JourneyLinksTruncated || links.NextCursor != ""
			for _, j := range links.Items {
				if !journeys[j.ID] {
					journeys[j.ID] = true
					node.Journeys = append(node.Journeys, v1.MapJourney{ID: j.ID, DisplayName: j.DisplayName, Revision: j.Revision})
				}
			}
		}
		if len(node.Objectives) > 0 {
			node.Health = health
		}
		sort.Slice(node.Journeys, func(i, j int) bool { return node.Journeys[i].ID < node.Journeys[j].ID })
		nodes[n.Name] = node
	}
	for _, e := range g.Spec.Dependencies {
		out.Edges = append(out.Edges, v1.DependencyEdge{Caller: e.Caller, Callee: e.Callee, Provenance: "declared", Critical: e.Critical, Description: e.Description})
	}
	if o := g.Spec.Observed; o != nil {
		out.Source = o.Source
		to := out.ObservedAt
		from := to.Add(-time.Duration(o.WindowSeconds) * time.Second)
		out.From = &from
		out.To = &to
		out.ObservationState = "unavailable"
		c, err := s.telemetrySource(o.Source)
		if err == nil {
			edges, truncated, observeErr := c.ObserveDependencies(ctx, *o, to)
			if observeErr == nil {
				out.ObservationState = "observed"
				out.Truncated = truncated
				if len(edges) == 0 {
					out.ObservationState = "no_edges"
				}
				for _, e := range edges {
					// Cap nodes as well as edges; the omitted frontier remains explicit.
					added := 0
					for _, name := range []string{e.Caller, e.Callee} {
						if _, ok := nodes[name]; !ok {
							added++
						}
					}
					if e.Caller == e.Callee {
						added /= 2
					}
					if len(nodes)+added > 80 {
						out.Truncated = true
						continue
					}
					for _, name := range []string{e.Caller, e.Callee} {
						if _, ok := nodes[name]; !ok {
							nodes[name] = v1.DependencyNode{Name: name, DisplayName: name, Health: "unknown", Objectives: []v1.MapObjective{}, Journeys: []v1.MapJourney{}}
						}
					}
					requests := e.Requests
					out.Edges = append(out.Edges, v1.DependencyEdge{Caller: e.Caller, Callee: e.Callee, Provenance: "observed", ConnectionType: e.ConnectionType, Requests: &requests})
				}
			}
		}
		out.Warnings = append(out.Warnings, "Observed edges use Tempo service-graph request counters. Trace sampling, missing instrumentation and virtual/database nodes can make this graph incomplete; absence is not proof of no dependency.")
		if out.ObservationState == "unavailable" {
			out.Warnings = append(out.Warnings, "Observed telemetry is unavailable; declared dependencies remain visible.")
		}
	}
	// Potential impact follows reverse reachability; it never changes measured health.
	for name, node := range nodes {
		visited := map[string]bool{name: true}
		queue := []string{name}
		journeys := map[string]v1.MapJourney{}
		for i := 0; i < len(queue); i++ {
			current := queue[i]
			n := nodes[current]
			node.ImpactTruncated = node.ImpactTruncated || n.JourneyLinksTruncated
			for _, j := range n.Journeys {
				journeys[j.ID] = j
			}
			for _, e := range out.Edges {
				if e.Callee == current && !visited[e.Caller] {
					visited[e.Caller] = true
					queue = append(queue, e.Caller)
				}
			}
		}
		node.PotentialImpact = []v1.MapJourney{}
		for _, j := range journeys {
			node.PotentialImpact = append(node.PotentialImpact, j)
		}
		sort.Slice(node.PotentialImpact, func(i, j int) bool { return node.PotentialImpact[i].ID < node.PotentialImpact[j].ID })
		if len(node.PotentialImpact) > 20 {
			node.PotentialImpact = node.PotentialImpact[:20]
			node.ImpactTruncated = true
		}
		nodes[name] = node
	}
	for _, n := range nodes {
		out.Nodes = append(out.Nodes, n)
	}
	sort.Slice(out.Nodes, func(i, j int) bool { return out.Nodes[i].Name < out.Nodes[j].Name })
	sort.Slice(out.Edges, func(i, j int) bool {
		a, b := out.Edges[i], out.Edges[j]
		return a.Caller+"\x00"+a.Callee+"\x00"+a.Provenance+"\x00"+a.ConnectionType < b.Caller+"\x00"+b.Callee+"\x00"+b.Provenance+"\x00"+b.ConnectionType
	})
	return out, nil
}
func healthRank(h string) int {
	switch h {
	case "degraded":
		return 3
	case "at_risk":
		return 2
	case "unknown":
		return 1
	}
	return 0
}
