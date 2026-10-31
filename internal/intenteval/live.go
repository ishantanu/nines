package intenteval

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ishantanu/nines/internal/intentmodel"
)

// Live requires explicit provider and model selection. It contacts only that
// configured model with the suite's synthetic text; no telemetry is queried.
func Live(ctx context.Context, lookup func(string) string, runtimeVersion string, selected ...string) (Report, error) {
	report := NewReport("live", runtimeVersion)
	if len(selected) > 1 {
		return report, errors.New("invalid case selection")
	}
	only := ""
	if len(selected) == 1 {
		only = selected[0]
	}
	report.SelectedCase = only
	cases, err := Cases()
	if err != nil {
		return report, err
	}
	if only != "" {
		found := false
		for _, c := range cases {
			if c.ID == only && !c.FixtureOnly {
				found = true
				break
			}
		}
		if !found {
			return report, errors.New("unknown or fixture-only live draft case")
		}
	}
	if strings.TrimSpace(lookup("INTENT_MODEL_PROVIDER")) == "" || strings.TrimSpace(lookup("INTENT_MODEL_MODEL")) == "" {
		return report, errors.New("live evaluations require explicit INTENT_MODEL_PROVIDER and INTENT_MODEL_MODEL")
	}
	client, err := intentmodel.FromEnvironment(nil, lookup)
	if err != nil || client == nil {
		return report, errors.New("live model configuration is unavailable; review provider, model and credential references")
	}
	descriptor := client.Descriptor()
	var last time.Time
	for _, c := range cases {
		if only != "" && c.ID != only {
			continue
		}
		if c.FixtureOnly {
			report.Skipped = append(report.Skipped, c.ID)
			continue
		}
		if !last.IsZero() {
			timer := time.NewTimer(time.Until(last.Add(5 * time.Second)))
			select {
			case <-ctx.Done():
				timer.Stop()
				return report, ctx.Err()
			case <-timer.C:
			}
		}
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
		last = time.Now()
		report.Add(Evaluate(ctx, c, descriptor.Provider, client))
	}
	report.Complete = ctx.Err() == nil
	if ctx.Err() != nil {
		return report, ctx.Err()
	}
	if only != "" {
		if len(report.Results) != 1 || report.Failed != 0 {
			return report, errors.New("selected draft case failed")
		}
		return report, nil
	}
	return report, Summary(report)
}
