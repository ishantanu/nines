package prometheus

import (
	"context"
	"fmt"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

// CollectWindows queries full-window estimates at each cutoff. No window is
// reconstructed from short increases, and all three query timestamps must match.
func (c *Client) CollectWindows(ctx context.Context, s model.SLO, from, to time.Time) ([]model.WindowObservation, error) {
	if s.ProviderWindow == nil {
		return nil, model.Invalid("providerWindow is required")
	}
	return c.CollectWindowRange(ctx, s, from, to, s.StepSeconds)
}

// CollectWindowRange samples independently evaluated full windows. It never changes
// the SLO definition or sums overlapping observations.
func (c *Client) CollectWindowRange(ctx context.Context, s model.SLO, from, to time.Time, resolution int64) ([]model.WindowObservation, error) {
	if s.ProviderWindow == nil || resolution < s.StepSeconds || resolution%s.StepSeconds != 0 {
		return nil, model.Invalid("resolution must be an aligned multiple of the SLO step")
	}
	out := []model.WindowObservation{}
	step := time.Duration(resolution) * time.Second
	for _, w := range s.Windows() {
		gq, tq, cq := model.WindowQueries(s, w)
		for cursor := from; cursor.Before(to); {
			end := cursor.Add(10000 * step)
			if end.After(to) {
				end = to
			}
			g, err := c.query(ctx, gq, cursor.Add(step), end, resolution)
			if err != nil {
				return nil, fmt.Errorf("window %d good: %w", w, err)
			}
			t, err := c.query(ctx, tq, cursor.Add(step), end, resolution)
			if err != nil {
				return nil, fmt.Errorf("window %d total: %w", w, err)
			}
			coverage, err := c.query(ctx, cq, cursor.Add(step), end, resolution)
			if err != nil {
				return nil, fmt.Errorf("window %d coverage: %w", w, err)
			}
			// A missing numerator/denominator remains absent. No zero-fill.
			for at, good := range g {
				total, ok := t[at]
				if !ok {
					continue
				}
				cov, ok := coverage[at]
				if !ok {
					continue
				}
				o := model.WindowObservation{Time: time.Unix(at, 0).UTC(), WindowSeconds: w, Revision: model.ConfigRevision(s), Good: good, Total: total, Coverage: cov}
				if err := o.Validate(s); err != nil {
					return nil, fmt.Errorf("invalid window sample: %w", err)
				}
				out = append(out, o)
			}
			cursor = end
		}
	}
	return out, nil
}
