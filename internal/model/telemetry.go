package model

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

type MeasurementBinding v1.MeasurementBinding
type ServiceDependencyMap v1.ServiceDependencyMap
type TopologyRevision struct {
	Manifest  json.RawMessage `json:"manifest"`
	Kind      string          `json:"kind"`
	ID        string          `json:"id"`
	Revision  int64           `json:"revision"`
	Digest    string          `json:"digest"`
	AppliedAt time.Time       `json:"appliedAt"`
}

var MetricName = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]{0,199}$`)
var LabelName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,99}$`)

func ValidateLabels(labels map[string]string) error {
	if len(labels) > 16 {
		return Invalid("at most 16 equality label filters are supported")
	}
	for k, v := range labels {
		if !LabelName.MatchString(k) || strings.HasPrefix(k, "__") || k == "le" || len(v) > 200 || strings.ContainsAny(v, "\x00\r\n") {
			return Invalid("invalid label filter; reserved labels and values over 200 bytes are unsupported")
		}
	}
	return nil
}
func validService(s string) bool { return validText(s, 200) && !strings.ContainsAny(s, "\x00\r\n") }
func topologyHeader(version, kind, expected string, meta v1.Metadata, display string) error {
	if version != APIVersion || kind != expected || !identifier.MatchString(meta.Name) || !validText(meta.Project, 200) || !validText(display, 200) {
		return Invalid("invalid version, kind, metadata or displayName")
	}
	return nil
}
func (b MeasurementBinding) Validate() error {
	if err := topologyHeader(b.APIVersion, b.Kind, v1.BindingKind, b.Metadata, b.Spec.DisplayName); err != nil {
		return err
	}
	s := b.Spec
	if s.ScrapeIntervalSeconds != 0 && (s.ScrapeIntervalSeconds < 1 || s.ScrapeIntervalSeconds > s.StepSeconds/2 || s.StepSeconds%s.ScrapeIntervalSeconds != 0) {
		return Invalid("scrapeIntervalSeconds must divide stepSeconds with at least two scrapes per step")
	}
	if !identifier.MatchString(s.Source) || !MetricName.MatchString(s.Metric) || !validService(s.Service) || !validText(s.ObservationPoint, 500) || s.StepSeconds < 1 || s.StepSeconds > 3600 {
		return Invalid("binding requires source, service, metric, observationPoint and stepSeconds (1–3600)")
	}
	if err := ValidateLabels(s.Labels); err != nil {
		return err
	}
	if err := ValidateLabels(s.SuccessLabels); err != nil {
		return err
	}
	switch s.Measurement {
	case "availability":
		if !oneOf(s.CountFrom, "", "counter", "histogram") {
			return Invalid("availability countFrom must be counter or histogram")
		}
		if len(s.SuccessLabels) == 0 || s.ThresholdSeconds != nil {
			return Invalid("availability requires successLabels and no thresholdSeconds")
		}
		for k := range s.SuccessLabels {
			if _, ok := s.Labels[k]; ok {
				return Invalid("successLabels must refine, not repeat or override population labels")
			}
		}
	case "latency":
		if s.CountFrom != "" {
			return Invalid("latency does not accept countFrom; it uses histogram buckets and count")
		}
		if len(s.SuccessLabels) > 0 || s.ThresholdSeconds == nil || !Finite(*s.ThresholdSeconds) || *s.ThresholdSeconds <= 0 || *s.ThresholdSeconds > 86400 {
			return Invalid("latency requires a positive thresholdSeconds at most 86400 and no successLabels")
		}
	default:
		return Invalid("measurement must be availability or latency")
	}
	return nil
}
func (g ServiceDependencyMap) Validate() error {
	if err := topologyHeader(g.APIVersion, g.Kind, v1.DependencyMapKind, g.Metadata, g.Spec.DisplayName); err != nil {
		return err
	}
	if len(g.Spec.Services) < 1 || len(g.Spec.Services) > 40 || len(g.Spec.Dependencies) > 100 {
		return Invalid("map requires 1–40 services and at most 100 dependencies")
	}
	names := map[string]bool{}
	refs := map[string]bool{}
	for _, n := range g.Spec.Services {
		if p := n.Position; p != nil && (p.X < 0 || p.Y < 0 || p.X > 10000 || p.Y > 10000) {
			return Invalid("service positions must be integer coordinates from 0 to 10000")
		}
		if !validService(n.Name) || !validText(n.DisplayName, 200) || names[n.Name] || len(n.SLORefs) > 10 {
			return Invalid("services require unique names, displayName and at most 10 SLO references")
		}
		names[n.Name] = true
		for _, id := range n.SLORefs {
			if !identifier.MatchString(id) || refs[id] {
				return Invalid("SLO references must be valid and assigned to one service only")
			}
			refs[id] = true
		}
	}
	if len(refs) > 100 {
		return Invalid("map may reference at most 100 SLOs")
	}
	edges := map[string]bool{}
	for _, e := range g.Spec.Dependencies {
		key := e.Caller + "\x00" + e.Callee
		if !oneOf(e.Origin, "", "manual", "kubernetes") || !names[e.Caller] || !names[e.Callee] || e.Caller == e.Callee || edges[key] || len(e.Description) > 500 {
			return Invalid("dependencies require unique distinct declared caller/callee services")
		}
		edges[key] = true
	}
	if o := g.Spec.Observed; o != nil {
		if !identifier.MatchString(o.Source) {
			return Invalid("observed requires a configured source")
		}
		if err := ValidateGraphObservation(*o); err != nil {
			return err
		}
		if err := ValidateLabels(o.Labels); err != nil {
			return err
		}
	}
	return nil
}

func ValidateGraphObservation(o v1.GraphObservation) error {
	interval, age := o.EvidenceTiming()
	if o.WindowSeconds < 60 || o.WindowSeconds > 3600 || interval < 1 || interval > 300 || interval*2 > o.WindowSeconds || age < interval || age > 3600 {
		return Invalid("observed requires a 60–3600s window, 1–300s scrape interval with two samples per window, and a sample age limit from scrape interval to 3600s")
	}
	return ValidateLabels(o.Labels)
}
