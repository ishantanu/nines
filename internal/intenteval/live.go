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
func Live(ctx context.Context, lookup func(string) string, runtimeVersion string) (Report, error) {
	report := NewReport("live", runtimeVersion)
	if strings.TrimSpace(lookup("INTENT_MODEL_PROVIDER")) == "" || strings.TrimSpace(lookup("INTENT_MODEL_MODEL")) == "" {
		return report, errors.New("live evaluations require explicit INTENT_MODEL_PROVIDER and INTENT_MODEL_MODEL")
	}
	client, err := intentmodel.FromEnvironment(nil, lookup)
	if err != nil || client == nil {
		return report, errors.New("live model configuration is unavailable; review provider, model and credential references")
	}
	cases, err := Cases()
	if err != nil {
		return report, err
	}
	descriptor := client.Descriptor()
	var last time.Time
	for _, c := range cases {
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
	return report, Summary(report)
}
