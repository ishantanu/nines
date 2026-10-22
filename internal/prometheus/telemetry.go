package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// Selector accepts equality filters only; user strings never become executable PromQL.
func Selector(metric string, labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+strconv.Quote(labels[k]))
	}
	return metric + "{" + strings.Join(parts, ",") + "}"
}
func (c *Client) telemetryRequest(ctx context.Context, path string, q url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/v1/"+path+"?"+q.Encode(), nil)
	if err != nil {
		return fmt.Errorf("invalid telemetry request")
	}
	if c.orgID != "" {
		req.Header.Set("X-Scope-OrgID", c.orgID)
	}
	if c.basicUsername != "" {
		req.SetBasicAuth(c.basicUsername, c.basicPassword)
	}
	if c.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.BearerToken)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("telemetry source unavailable")
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != 200 {
		return fmt.Errorf("telemetry source returned HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (512<<10)+1))
	if err != nil || len(b) > 512<<10 {
		return fmt.Errorf("telemetry response exceeds bounds or cannot be read")
	}
	var e struct {
		Status   string          `json:"status"`
		Warnings []string        `json:"warnings"`
		Data     json.RawMessage `json:"data"`
	}
	if json.Unmarshal(b, &e) != nil || e.Status != "success" || len(e.Warnings) > 0 {
		return fmt.Errorf("telemetry source failed or returned partial data")
	}
	if json.Unmarshal(e.Data, out) != nil {
		return fmt.Errorf("invalid telemetry response")
	}
	return nil
}
func (c *Client) MetricNames(ctx context.Context, prefix string, at time.Time) ([]string, bool, error) {
	if len(prefix) > 100 || !regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`).MatchString(prefix) {
		return nil, false, model.Invalid("metric prefix requires 1–100 metric-name characters")
	}
	q := url.Values{"match[]": {"{__name__=~" + strconv.Quote(regexp.QuoteMeta(prefix)+".*") + "}"}, "start": {strconv.FormatInt(at.Add(-15*time.Minute).Unix(), 10)}, "end": {strconv.FormatInt(at.Unix(), 10)}, "limit": {"101"}}
	var names []string
	if err := c.telemetryRequest(ctx, "label/__name__/values", q, &names); err != nil {
		return nil, false, err
	}
	if len(names) > 101 {
		return nil, false, fmt.Errorf("source did not honor bounded metric lookup; narrow the prefix")
	}
	for _, n := range names {
		if !model.MetricName.MatchString(n) || !strings.HasPrefix(n, prefix) {
			return nil, false, fmt.Errorf("source returned invalid metric names")
		}
	}
	sort.Strings(names)
	truncated := len(names) > 100
	if truncated {
		names = names[:100]
	}
	if names == nil {
		names = []string{}
	}
	return names, truncated, nil
}
func (c *Client) Discover(ctx context.Context, metric string, labels map[string]string, at time.Time) (v1.TelemetryDiscovery, error) {
	d := v1.TelemetryDiscovery{Metric: metric, From: at.Add(-15 * time.Minute), To: at, Metadata: []v1.TelemetryMetadata{}, Series: []map[string]string{}, Warnings: []string{"Series are a bounded 15-minute index lookup; presence does not establish fresh samples, complete coverage or success semantics."}}
	if !model.MetricName.MatchString(metric) {
		return d, model.Invalid("invalid metric name")
	}
	if err := model.ValidateLabels(labels); err != nil {
		return d, err
	}
	var metadata map[string][]v1.TelemetryMetadata
	if err := c.telemetryRequest(ctx, "metadata", url.Values{"metric": {metric}, "limit": {"1"}, "limit_per_metric": {"10"}}, &metadata); err != nil {
		return d, err
	}
	// Histogram series names include a suffix while metadata belongs to the family.
	if len(metadata[metric]) == 0 {
		for _, suffix := range []string{"_bucket", "_count", "_sum"} {
			if !strings.HasSuffix(metric, suffix) {
				continue
			}
			family := strings.TrimSuffix(metric, suffix)
			var familyMetadata map[string][]v1.TelemetryMetadata
			if err := c.telemetryRequest(ctx, "metadata", url.Values{"metric": {family}, "limit": {"1"}, "limit_per_metric": {"10"}}, &familyMetadata); err != nil {
				return d, err
			}
			if len(familyMetadata) > 1 || len(familyMetadata[family]) > 10 {
				return d, fmt.Errorf("source metadata exceeds bounds")
			}
			isHistogram := len(familyMetadata[family]) > 0
			for _, m := range familyMetadata[family] {
				isHistogram = isHistogram && m.Type == "histogram"
			}
			if isHistogram {
				metric = family
				d.Metric = family
				metadata = familyMetadata
				d.Warnings = append(d.Warnings, "Histogram series name resolved to its metadata family.")
			}
			break
		}
	}
	if len(metadata) > 1 || len(metadata[metric]) > 10 {
		return d, fmt.Errorf("source metadata exceeds bounds")
	}
	for _, m := range metadata[metric] {
		if len(m.Type) > 40 || len(m.Unit) > 100 || len(m.Help) > 4000 {
			return d, fmt.Errorf("source metadata exceeds bounds")
		}
		d.Metadata = append(d.Metadata, m)
	}
	metrics := []string{metric}
	histogram := false
	for _, m := range d.Metadata {
		histogram = histogram || m.Type == "histogram"
	}
	if histogram {
		metrics = []string{metric + "_bucket", metric + "_count"}
	}
	q := url.Values{"start": {strconv.FormatInt(d.From.Unix(), 10)}, "end": {strconv.FormatInt(at.Unix(), 10)}, "limit": {"101"}}
	for _, name := range metrics {
		q.Add("match[]", Selector(name, labels))
	}
	if err := c.telemetryRequest(ctx, "series", q, &d.Series); err != nil {
		return d, err
	}
	if len(d.Series) > 101 {
		return d, fmt.Errorf("source did not honor bounded series lookup; add population labels")
	}
	for _, series := range d.Series {
		if len(series) > 40 {
			return d, fmt.Errorf("source series labels exceed bounds")
		}
		allowed := false
		for _, n := range metrics {
			allowed = allowed || series["__name__"] == n
		}
		if !allowed {
			return d, fmt.Errorf("source returned unmatched series")
		}
		for k, v := range series {
			if !model.LabelName.MatchString(k) || len(v) > 2000 {
				return d, fmt.Errorf("source series labels exceed bounds")
			}
		}
		for k, v := range labels {
			if series[k] != v {
				return d, fmt.Errorf("source returned unmatched labels")
			}
		}
	}
	sort.Slice(d.Series, func(i, j int) bool {
		a, _ := json.Marshal(d.Series[i])
		b, _ := json.Marshal(d.Series[j])
		return string(a) < string(b)
	})
	d.Truncated = len(d.Series) > 100
	if d.Truncated {
		d.Series = d.Series[:100]
		d.Warnings = append(d.Warnings, "Series lookup was truncated; refine labels before compiling.")
	}
	return d, nil
}
func (c *Client) ObserveDependencies(ctx context.Context, o v1.GraphObservation, at time.Time) ([]v1.ObservedEdge, bool, error) {
	if err := model.ValidateLabels(o.Labels); err != nil {
		return nil, false, err
	}
	if o.WindowSeconds < 60 || o.WindowSeconds > 3600 {
		return nil, false, model.Invalid("invalid observation window")
	}
	query := fmt.Sprintf("topk(101, sum by (client,server,connection_type) (increase(%s[%ds])))", Selector("traces_service_graph_request_total", o.Labels), o.WindowSeconds)
	var data struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []json.RawMessage `json:"value"`
		} `json:"result"`
	}
	if err := c.telemetryRequest(ctx, "query", url.Values{"query": {query}, "time": {strconv.FormatInt(at.Unix(), 10)}, "timeout": {"5s"}, "limit": {"101"}}, &data); err != nil {
		return nil, false, err
	}
	if data.ResultType != "vector" || len(data.Result) > 101 {
		return nil, false, fmt.Errorf("invalid bounded service graph vector")
	}
	out := []v1.ObservedEdge{}
	seen := map[string]bool{}
	for _, s := range data.Result {
		caller, callee, kind := s.Metric["client"], s.Metric["server"], s.Metric["connection_type"]
		if strings.TrimSpace(caller) == "" || strings.TrimSpace(callee) == "" || len(caller) > 200 || len(callee) > 200 || len(kind) > 100 || strings.ContainsAny(caller+callee+kind, "\x00\r\n") || len(s.Value) != 2 {
			return nil, false, fmt.Errorf("invalid service graph labels or sample")
		}
		var timestamp float64
		var value string
		if json.Unmarshal(s.Value[0], &timestamp) != nil || json.Unmarshal(s.Value[1], &value) != nil || timestamp != float64(at.Unix()) {
			return nil, false, fmt.Errorf("invalid service graph timestamp")
		}
		n, err := strconv.ParseFloat(value, 64)
		if err != nil || !model.Finite(n) || n < 0 {
			return nil, false, fmt.Errorf("invalid service graph count")
		}
		if n == 0 {
			continue
		}
		key := caller + "\x00" + callee + "\x00" + kind
		if seen[key] {
			return nil, false, fmt.Errorf("duplicate service graph edge")
		}
		seen[key] = true
		out = append(out, v1.ObservedEdge{Caller: caller, Callee: callee, ConnectionType: kind, Requests: n})
	}
	truncated := len(data.Result) > 100
	if len(out) > 100 {
		out = out[:100]
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		return a.Caller+"\x00"+a.Callee+"\x00"+a.ConnectionType < b.Caller+"\x00"+b.Callee+"\x00"+b.ConnectionType
	})
	return out, truncated, nil
}
