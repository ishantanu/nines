package newrelic

import (
	"context"
	"fmt"
	"strings"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// NativeResults builds a fixed query from a verified native identity. Provider-
// returned NRQL is used only to recognize the supported SUM metric profile;
// arbitrary resultQueries are never executed.
func (c *Client) NativeResults(ctx context.Context, row NativeResource, target float64, from, to time.Time) (v1.NativeSLOResults, error) {
	out := v1.NativeSLOResults{Provider: "newrelic", NativeID: row.ID, URL: c.NativeURL(row.GUID), From: from, To: to, Target: target, State: "no_data", BudgetBasis: "derived_from_native_counts", Points: []v1.NativeSLOPoint{}, Warnings: []string{"New Relic derived SLI metrics begin after native configuration and may lag by two minutes. No earlier raw-event history is substituted.", "Attainment and budget are calculated from native SUM good/valid metrics; budget is not a separately reported vendor field. History points are interval ratios, not rolling-window compliance. These results do not change Nines coverage, burn alerts or notification evidence."}}
	if !ValidEntityGUID(row.GUID) || row.ID == "" || !model.Finite(target) || target <= 0 || target >= 1 || from.Unix() < 0 || from.Nanosecond() != 0 || to.Nanosecond() != 0 || !to.After(from) || to.Sub(from) > 31*24*time.Hour {
		return out, model.Invalid("native history requires a valid identity, target and whole-second range of at most 31 days")
	}
	for _, q := range []struct{ metric, alias, actual string }{{"good", "Good", row.ResultQueries.GoodEvents.NRQL}, {"valid", "Valid", row.ResultQueries.ValidEvents.NRQL}} {
		expected := fmt.Sprintf("SELECT sum(newrelic.sli.%s) AS '%s' FROM Metric WHERE entity.guid = '%s' UNTIL 2 minutes AGO", q.metric, q.alias, row.GUID)
		if q.actual != expected {
			return out, model.Invalid("native New Relic result queries differ from the supported SUM metric profile")
		}
	}
	// At most 300 native buckets and one request; absolute bounds are aligned for
	// exact metadata validation. Aggregate uses these same bounds.
	step := int64((to.Unix() - from.Unix() + 299) / 300)
	if step < 60 {
		step = 60
	}
	step = ((step + 59) / 60) * 60

	alignedFrom := time.Unix(((from.Unix()+step-1)/step)*step, 0).UTC()
	alignedTo := time.Unix((to.Unix()/step)*step, 0).UTC()
	if !alignedTo.After(alignedFrom) {
		return out, model.Invalid("select at least one complete %d-second native interval", step)
	}
	if !alignedFrom.Equal(from) || !alignedTo.Equal(to) {
		out.Warnings = append(out.Warnings, fmt.Sprintf("Requested bounds were clipped inward to complete %d-second native buckets. The returned from/to values identify the effective range.", step))
	}
	from, to = alignedFrom, alignedTo
	out.From = from
	out.To = to

	suffix := fmt.Sprintf(" SINCE %d UNTIL %d TIMESERIES %d SECONDS", from.UnixMilli(), to.UnixMilli(), step)
	query := func(metric string) string {
		return fmt.Sprintf("SELECT sum(newrelic.sli.%s) AS nines_count FROM Metric WHERE entity.guid = '%s'", metric, row.GUID) + suffix
	}
	res, err := c.request(ctx, countDocument, map[string]any{"account": c.account, "good": query("good"), "total": query("valid")})
	if err != nil {
		return out, err
	}
	account := res.Data.Actor.Account
	good, err := readBuckets(account.Good, from, to, step)
	if err != nil {
		return out, err
	}
	total, err := readBuckets(account.Total, from, to, step)
	if err != nil {
		return out, err
	}
	if len(good) != len(total) {
		return out, model.Invalid("native good/valid buckets do not match")
	}
	var gsum, tsum float64
	gaps := false
	for at := from.Unix() + step; at <= to.Unix(); at += step {
		p := v1.NativeSLOPoint{Time: time.Unix(at, 0).UTC()}
		g, gok := good[at]
		t, tok := total[at]
		if gok != tok {
			return out, model.Invalid("native good/valid buckets do not match")
		}
		if !gok {
			gaps = true
		}
		if gok {
			if g > t {
				return out, model.Invalid("native good count exceeds valid count")
			}
			if t > 0 {
				v := g / t
				p.Attainment = &v
				gsum += g
				tsum += t
			}
		}
		out.Points = append(out.Points, p)
	}
	if tsum > 0 {
		v := gsum / tsum
		b := (v - target) / (1 - target)
		out.Attainment = &v
		out.BudgetRemaining = &b
		out.State = "observed"
		if gaps {
			out.State = "partial"
			out.Warnings = append(out.Warnings, "Native history has missing intervals. Attainment uses available counts and does not establish full-range coverage.")
		}
	}
	return out, nil
}
func (c *Client) NativeCatalog(ctx context.Context, entity string) ([]NativeResource, error) {
	return c.nativeRows(ctx, entity)
}
func (c *Client) NativeDraft(row NativeResource, source, name, project, service string) (v1.ServiceLevelObjective, error) {
	out := v1.ServiceLevelObjective{APIVersion: v1.APIVersion, Kind: v1.SLOKind, Metadata: v1.Metadata{Name: name, Project: project}}
	doc, err := nativeDocument(row)
	if err != nil {
		return out, err
	}
	if doc.Events.AccountID != c.account || len(doc.Objectives) != 1 || doc.Events.GoodEvents.From != doc.Events.ValidEvents.From {
		return out, model.Invalid("import requires one objective and same-account, same-event good/valid COUNT queries")
	}
	roll := doc.Objectives[0].TimeWindow.Rolling
	if roll.Unit != "DAY" || (roll.Count != 1 && roll.Count != 7 && roll.Count != 14 && roll.Count != 28 && roll.Count != 30) {
		return out, model.Invalid("unsupported native rolling window")
	}
	query := func(q NativeEventQuery) string {
		v := "SELECT count(*) FROM " + q.From
		if strings.TrimSpace(q.Where) != "" {
			v += " WHERE " + q.Where
		}
		return v
	}
	out.Spec = v1.SLOSpec{Source: source, Service: service, DisplayName: row.Name, Target: doc.Objectives[0].Target / 100, WindowSeconds: roll.Count * 86400, StepSeconds: 60, QueryLanguage: "nrql", GoodQuery: query(doc.Events.GoodEvents), TotalQuery: query(doc.Events.ValidEvents), Lifecycle: v1.LifecycleExperimental}
	if err := model.SLODefinition(out).Validate(); err != nil {
		return out, err
	}
	return out, c.ValidateSLO(model.SLO{QueryLanguage: "nrql", StepSeconds: 60, GoodQuery: out.Spec.GoodQuery, TotalQuery: out.Spec.TotalQuery})
}
