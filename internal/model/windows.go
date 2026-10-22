package model

import (
	"fmt"
	"time"
)

// WindowObservation is a provider's estimate for one complete window. Adjacent
// observations overlap and MUST NOT be summed into a longer window.
type WindowObservation struct {
	Time          time.Time `json:"time"`
	WindowSeconds int64     `json:"windowSeconds"`
	Revision      int64     `json:"revision"`
	Good          float64   `json:"good"`
	Total         float64   `json:"total"`
	Coverage      float64   `json:"coverage"`
}

func WindowKey(window int64, at time.Time) string { return fmt.Sprintf("%d/%d", window, at.Unix()) }
func (o WindowObservation) Validate(s SLO) error {
	if err := (Point{Time: o.Time, Good: o.Good, Total: o.Total}).Validate(s.StepSeconds); err != nil {
		return err
	}
	if o.Revision != ConfigRevision(s) || o.WindowSeconds < s.StepSeconds || o.WindowSeconds%s.StepSeconds != 0 || o.WindowSeconds > s.WindowSeconds || !Finite(o.Coverage) || o.Coverage < 0 || o.Coverage > 1 {
		return Invalid("invalid provider-window observation")
	}
	return nil
}
func (s SLO) Windows() []int64 {
	out := []int64{s.StepSeconds, s.WindowSeconds}
	for _, p := range s.AlertPolicies() {
		out = append(out, p.ShortWindowSeconds, p.LongWindowSeconds)
	}
	seen := map[int64]bool{}
	unique := []int64{}
	for _, w := range out {
		if !seen[w] {
			seen[w] = true
			unique = append(unique, w)
		}
	}
	return unique
}

// WindowQueries deliberately applies increase per series BEFORE sum, retaining
// reset handling and replica identity. Coverage is conservative scrape density
// of the least-observed population, plus freshness, never inferred from traffic.
func WindowQueries(s SLO, window int64) (good, total, coverage string) {
	p := s.ProviderWindow
	good = fmt.Sprintf("sum(increase(%s[%ds]))", p.GoodSelector, window)
	total = fmt.Sprintf("sum(increase(%s[%ds]))", p.TotalSelector, window)
	expected := float64(window) / float64(p.ScrapeIntervalSeconds)
	cov := func(q string) string {
		return fmt.Sprintf("(clamp_max(min(count_over_time(%s[%ds])) / %g, 1) and (max(time() - timestamp(%s)) <= %d))", q, window, expected, q, 2*p.ScrapeIntervalSeconds)
	}
	coverage = "(min(((" + cov(p.GoodSelector) + ") or vector(0)) or label_replace(((" + cov(p.TotalSelector) + ") or vector(0)), \"population\", \"total\", \"\", \"\"))) or vector(0)"
	return
}

// StableQuery selects exactly the latest completed UTC-aligned source cutoff.
func StableQuery(s SLO, query string) string {
	return fmt.Sprintf("last_over_time((%s)[%ds:%ds] offset %ds)", query, s.StepSeconds, s.StepSeconds, s.StepSeconds)
}
