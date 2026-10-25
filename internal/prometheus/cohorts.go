package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func (c *Client) ReadCohorts(ctx context.Context, s model.SLO, labels []string, at time.Time) (v1.CohortEvidence, error) {
	if err := s.Validate(); err != nil {
		return v1.CohortEvidence{}, err
	}
	out := v1.CohortEvidence{At: at, WindowSeconds: s.WindowSeconds, Counts: []v1.CohortCount{}}
	if s.ProviderWindow == nil || len(labels) == 0 || len(labels) > 3 {
		return out, fmt.Errorf("cohort inspection requires provider-window counts and one to three labels")
	}
	keys := append([]string(nil), labels...)
	sort.Strings(keys)
	for i, key := range keys {
		if !model.LabelName.MatchString(key) || strings.HasPrefix(key, "__") || key == "le" || (i > 0 && keys[i-1] == key) {
			return out, fmt.Errorf("invalid cohort labels")
		}
	}
	query := func(selector string) (map[string]v1.CohortCount, error) {
		expression := fmt.Sprintf("topk(101, sum by (%s) (increase(%s[%ds])))", strings.Join(keys, ","), selector, s.WindowSeconds)
		var data struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []json.RawMessage `json:"value"`
			} `json:"result"`
		}
		err := c.telemetryRequest(ctx, "query", url.Values{"query": {expression}, "time": {strconv.FormatInt(at.Unix(), 10)}, "timeout": {"15s"}}, &data)
		if err != nil {
			return nil, err
		}
		if data.ResultType != "vector" || len(data.Result) > 101 {
			return nil, fmt.Errorf("invalid cohort vector")
		}
		result := map[string]v1.CohortCount{}
		for _, sample := range data.Result {
			if len(sample.Value) != 2 {
				return nil, fmt.Errorf("invalid cohort tuple")
			}
			var timestamp float64
			var text string
			if json.Unmarshal(sample.Value[0], &timestamp) != nil || timestamp != float64(at.Unix()) || json.Unmarshal(sample.Value[1], &text) != nil {
				return nil, fmt.Errorf("cohort timestamps must match the requested cutoff")
			}
			count, err := strconv.ParseFloat(text, 64)
			if err != nil || !model.Finite(count) || count < 0 || count > 1e18 {
				return nil, fmt.Errorf("invalid cohort count")
			}
			selected := map[string]string{}
			for _, key := range keys {
				selected[key] = sample.Metric[key]
			}
			key := model.DefinitionDigest(selected)
			if _, ok := result[key]; ok {
				return nil, fmt.Errorf("duplicate cohort")
			}
			result[key] = v1.CohortCount{Labels: selected, Total: count}
		}
		return result, nil
	}
	total, err := query(s.ProviderWindow.TotalSelector)
	if err != nil {
		return out, err
	}
	good, err := query(s.ProviderWindow.GoodSelector)
	if err != nil {
		return out, err
	}
	for key, item := range total {
		if g, ok := good[key]; ok {
			n := g.Total
			item.Good = &n
			delete(good, key)
		}
		out.Counts = append(out.Counts, item)
	}
	if len(good) > 0 {
		return out, fmt.Errorf("numerator includes cohorts missing from denominator")
	}
	sort.Slice(out.Counts, func(i, j int) bool {
		return model.DefinitionDigest(out.Counts[i].Labels) < model.DefinitionDigest(out.Counts[j].Labels)
	})
	return out, nil
}
