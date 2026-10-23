package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func (c *Client) DiscoverMetricNames(ctx context.Context, source, prefix string) (v1.MetricNames, error) {
	var out v1.MetricNames
	if !identifier.MatchString(source) {
		return out, errors.New("invalid source")
	}
	err := c.scopedRequest(ctx, http.MethodGet, "/api/connections/"+source+"/metrics?"+url.Values{"prefix": {prefix}}.Encode(), nil, 1<<20, &out)
	return out, err
}
func (c *Client) DiscoverTelemetry(ctx context.Context, source, metric string, labels map[string]string) (v1.TelemetryDiscovery, error) {
	var out v1.TelemetryDiscovery
	if !identifier.MatchString(source) {
		return out, errors.New("invalid source")
	}
	b, _ := json.Marshal(labels)
	err := c.scopedRequest(ctx, http.MethodGet, "/api/connections/"+source+"/telemetry?"+url.Values{"metric": {metric}, "labels": {string(b)}}.Encode(), nil, 1<<20, &out)
	return out, err
}
func (c *Client) CompileBinding(ctx context.Context, id string, in v1.CompileBindingInput) (v1.CompiledBinding, error) {
	var out v1.CompiledBinding
	if !identifier.MatchString(id) || in.ExpectedRevision < 1 {
		return out, errors.New("valid binding ID and positive expectedRevision required")
	}
	b, err := json.Marshal(in)
	if err == nil {
		err = c.scopedRequest(ctx, http.MethodPost, "/api/bindings/"+id+"/compile", b, 1<<20, &out)
	}
	return out, err
}
func (c *Client) DependencyMap(ctx context.Context, id string, expected int64) (v1.DependencyView, error) {
	var out v1.DependencyView
	if !identifier.MatchString(id) || expected < 0 {
		return out, errors.New("invalid dependency-map request")
	}
	q := url.Values{}
	if expected > 0 {
		q.Set("expectedRevision", strconv.FormatInt(expected, 10))
	}
	err := c.scopedRequest(ctx, http.MethodGet, "/api/dependency-maps/"+id+"/view?"+q.Encode(), nil, 1<<20, &out)
	return out, err
}
func (c *Client) TopologyResources(ctx context.Context, kind, cursor string, limit int) (v1.TopologyCatalog, error) {
	var out v1.TopologyCatalog
	if limit < 0 || limit > 100 || len(cursor) > 1024 {
		return out, errors.New("invalid topology page")
	}
	q := url.Values{"kind": {kind}, "cursor": {cursor}}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	err := c.scopedRequest(ctx, http.MethodGet, "/api/topology/resources?"+q.Encode(), nil, 1<<20, &out)
	return out, err
}

// CompileMeasurement verifies the SLO-owned measurement without saving resources
// or deploying rules. Native versioned SLO YAML or JSON is accepted.
func (c *Client) CompileMeasurement(ctx context.Context, definition []byte) (v1.CompiledMeasurement, error) {
	var out v1.CompiledMeasurement
	if len(definition) == 0 || len(definition) > 1<<20 {
		return out, errors.New("provide an SLO definition of at most 1 MiB")
	}
	err := c.scopedMediaRequest(ctx, http.MethodPost, "/api/slos/compile", definition, 1<<20, "application/yaml", &out)
	return out, err
}
