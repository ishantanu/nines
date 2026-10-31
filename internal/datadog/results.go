package datadog

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// NativeResults reads the provider's metric SLO history, with corrections explicitly
// disabled. The caller must first verify ownership/content of the native revision.
func (c *Client) NativeResults(ctx context.Context, id string, target float64, from, to time.Time) (v1.NativeSLOResults, error) {
	out := v1.NativeSLOResults{Provider: "datadog", NativeID: id, URL: c.NativeURL(id), From: from, To: to, Target: target, State: "no_data", BudgetBasis: "provider_reported", Points: []v1.NativeSLOPoint{}, Warnings: []string{"Datadog corrections are disabled for this read. Provider history is independent from Nines coverage, burn alerts and notification delivery.", "History points are interval ratios, not rolling-window compliance. The aggregate covers only the requested range."}}
	if !nativeIDPattern.MatchString(id) || !model.Finite(target) || target <= 0 || target >= 1 || from.Unix() < 0 || from.Nanosecond() != 0 || to.Nanosecond() != 0 || !to.After(from) || to.Sub(from) > 31*24*time.Hour {
		return out, model.Invalid("native history requires a valid ID, target and whole-second range of at most 31 days")
	}
	var res struct {
		Errors []json.RawMessage `json:"errors"`
		Data   *struct {
			From    *int64 `json:"from_ts"`
			To      *int64 `json:"to_ts"`
			Type    string `json:"type"`
			Overall *struct {
				SLI     *float64            `json:"sli_value"`
				Budget  map[string]*float64 `json:"error_budget_remaining"`
				Errors  []json.RawMessage   `json:"errors"`
				Preview bool                `json:"preview"`
			} `json:"overall"`
			Series *struct {
				Interval  int64     `json:"interval"`
				Message   string    `json:"message"`
				Times     []float64 `json:"times"`
				Numerator *struct {
					Values []*float64 `json:"values"`
				} `json:"numerator"`
				Denominator *struct {
					Values []*float64 `json:"values"`
				} `json:"denominator"`
			} `json:"series"`
		} `json:"data"`
	}
	q := url.Values{"from_ts": {strconv.FormatInt(from.Unix(), 10)}, "to_ts": {strconv.FormatInt(to.Unix(), 10)}, "target": {strconv.FormatFloat(target*100, 'f', -1, 64)}, "apply_correction": {"false"}}
	if err := c.nativeRequest(ctx, http.MethodGet, "/api/v1/slo/"+id+"/history", q, nil, &res); err != nil {
		return out, err
	}
	d := res.Data
	if len(res.Errors) > 0 || d == nil || d.From == nil || d.To == nil || *d.From != from.Unix() || *d.To != to.Unix() || d.Type != "metric" || d.Overall == nil || len(d.Overall.Errors) > 0 || d.Overall.Preview {
		return out, model.Invalid("Datadog native history is incomplete, shifted, previewed or unsupported")
	}
	if d.Series == nil || d.Series.Numerator == nil || d.Series.Denominator == nil || d.Series.Message != "" || d.Series.Interval < 1 || len(d.Series.Times) > 2000 || len(d.Series.Numerator.Values) != len(d.Series.Times) || len(d.Series.Denominator.Values) != len(d.Series.Times) {
		return out, model.Invalid("Datadog native history series is incomplete or exceeds 2000 points")
	}
	seenTraffic := false
	var previous int64
	for i, ts := range d.Series.Times {
		if !model.Finite(ts) || ts != math.Trunc(ts) || ts < float64(from.UnixMilli()) || ts >= float64(to.UnixMilli()) || i > 0 && int64(ts) <= previous {
			return out, model.Invalid("Datadog native history timestamps are invalid")
		}
		previous = int64(ts)
		p := v1.NativeSLOPoint{Time: time.UnixMilli(int64(ts)).UTC()}
		good, total := d.Series.Numerator.Values[i], d.Series.Denominator.Values[i]
		if good != nil && total != nil {
			if !model.Finite(*good) || !model.Finite(*total) || *good < 0 || *total < 0 || *good > *total {
				return out, model.Invalid("Datadog native history contains invalid counts")
			}
			if *total > 0 {
				v := *good / *total
				p.Attainment = &v
				seenTraffic = true
			}
		}
		out.Points = append(out.Points, p)
	}
	if !seenTraffic {
		return out, nil
	}
	if d.Overall.SLI == nil || !model.Finite(*d.Overall.SLI) || *d.Overall.SLI < 0 || *d.Overall.SLI > 100 {
		return out, model.Invalid("Datadog native aggregate attainment is missing or invalid")
	}
	v := *d.Overall.SLI / 100
	out.Attainment = &v
	out.State = "observed"
	if b := d.Overall.Budget["custom"]; b != nil {
		if !model.Finite(*b) || *b > 100.000001 {
			return out, model.Invalid("Datadog native budget is invalid")
		}
		v := *b / 100
		out.BudgetRemaining = &v
	}
	return out, nil
}

// NativeCatalog refuses truncated/ambiguous listings rather than claiming completeness.
func (c *Client) NativeCatalog(ctx context.Context) ([]NativeResource, error) {
	rows := []NativeResource{}
	seen := map[string]bool{}
	for offset := 0; offset < 1000; offset += 100 {
		var res struct {
			Data   []NativeResource  `json:"data"`
			Errors []json.RawMessage `json:"errors"`
		}
		if err := c.nativeRequest(ctx, http.MethodGet, "/api/v1/slo", url.Values{"limit": {"100"}, "offset": {strconv.Itoa(offset)}}, nil, &res); err != nil {
			return nil, err
		}
		if res.Data == nil || len(res.Data) > 100 || len(res.Errors) > 0 {
			return nil, model.ErrUnavailable
		}
		for _, row := range res.Data {
			if !nativeIDPattern.MatchString(row.ID) || seen[row.ID] {
				return nil, model.ErrUnavailable
			}
			seen[row.ID] = true
			rows = append(rows, row)
		}
		if len(res.Data) < 100 {
			return rows, nil
		}
	}
	return nil, model.Invalid("native SLO catalog exceeds the 1000-resource limit")
}

func (c *Client) NativeDraft(row NativeResource, source, name, project, service string) (v1.ServiceLevelObjective, error) {
	out := v1.ServiceLevelObjective{APIVersion: v1.APIVersion, Kind: v1.SLOKind, Metadata: v1.Metadata{Name: name, Project: project}}
	if row.Type != "metric" || len(row.Thresholds) != 1 {
		return out, model.Invalid("only metric SLOs with one threshold can be imported")
	}
	days := int64(0)
	switch row.Thresholds[0].Timeframe {
	case "7d":
		days = 7
	case "30d":
		days = 30
	default:
		return out, model.Invalid("only 7- or 30-day metric SLOs can be imported")
	}
	out.Spec = v1.SLOSpec{Source: source, Service: service, DisplayName: row.Name, Target: row.Thresholds[0].Target / 100, WindowSeconds: days * 86400, StepSeconds: 60, QueryLanguage: "datadog", GoodQuery: row.Query.Numerator + ".rollup(sum,60)", TotalQuery: row.Query.Denominator + ".rollup(sum,60)", Lifecycle: v1.LifecycleExperimental}
	if err := model.SLODefinition(out).Validate(); err != nil {
		return out, err
	}
	return out, c.ValidateSLO(model.SLO{QueryLanguage: "datadog", StepSeconds: 60, GoodQuery: out.Spec.GoodQuery, TotalQuery: out.Spec.TotalQuery})
}
