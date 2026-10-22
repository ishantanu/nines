package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/ishantanu/nines/internal/model"
)

func generateWindows(s model.SLO, tenant, ownerUID string) (Group, error) {
	connection := ConnectionID(s)
	hash := sha256.Sum256([]byte(tenant + "/" + connection))
	g := Group{Name: "nines-" + s.ID + "-" + hex.EncodeToString(hash[:5]), Interval: fmt.Sprintf("%ds", s.StepSeconds)}
	labels := func(window int64) map[string]string {
		l := map[string]string{"nines_managed": "true", "nines_tenant": tenant, "nines_source": connection, "nines_slo": s.ID, "nines_measurement": "provider_window", "nines_revision": fmt.Sprint(model.ConfigRevision(s))}
		if ownerUID != "" {
			l["nines_resource_uid"] = ownerUID
		}
		if window > 0 {
			l["window"] = fmt.Sprintf("%ds", window)
		}
		return l
	}
	add := func(name, expr string, w int64) {
		g.Rules = append(g.Rules, Rule{Record: name, Expr: expr, Labels: labels(w)})
	}
	add("nines:slo_target:ratio", "vector("+strconv.FormatFloat(s.Target, 'g', -1, 64)+")", 0)
	add("nines:slo_evaluation_timestamp:seconds", fmt.Sprintf("floor(vector(time()) / %d) * %d - %d", s.StepSeconds, s.StepSeconds, s.StepSeconds), 0)
	burns, coverages := map[int64]string{}, map[int64]string{}
	for _, w := range s.Windows() {
		// The step-sized observation is for chart detail only; it never contributes
		// to another window, so burst boundary errors cannot accumulate.
		good, total, cov := model.WindowQueries(s, w)
		good = model.StableQuery(s, good)
		total = model.StableQuery(s, total)
		cov = model.StableQuery(s, cov)
		valid := fmt.Sprintf(" and on() ((%s >= 0) and (%s >= %s) and (%s <= 1e18) and ((%s == 0) or (%s >= 1e-9)))", good, total, good, total, total, total)
		good = "(" + good + ")" + valid
		total = "(" + total + ")" + valid
		add("nines:sli_good:window", good, w)
		add("nines:sli_total:window", total, w)
		ratio := fmt.Sprintf("((%s) / (%s)) and on() ((%s) > 0)", good, total, total)
		burn := "(1 - (" + ratio + ")) / " + strconv.FormatFloat(1-s.Target, 'g', -1, 64)
		add("nines:sli:ratio", ratio, w)
		add("nines:slo_burn_rate:ratio", burn, w)
		add("nines:slo_coverage:ratio", cov, w)
		add("nines:slo_budget_remaining:ratio", "1 - ("+burn+")", w)
		burns[w] = burn
		coverages[w] = cov
	}
	for _, p := range s.AlertPolicies() {
		expr := fmt.Sprintf("((%s) >= %g) and on() ((%s) >= %g) and on() ((%s) >= %g) and on() ((%s) >= %g)", burns[p.ShortWindowSeconds], p.BurnRate, burns[p.LongWindowSeconds], p.BurnRate, coverages[p.ShortWindowSeconds], p.MinCoverage, coverages[p.LongWindowSeconds], p.MinCoverage)
		alertLabels := labels(0)
		if p.Name != "" {
			alertLabels["nines_policy"] = p.Name
		}
		if p.Severity != "" {
			alertLabels["severity"] = p.Severity
		}
		g.Rules = append(g.Rules, Rule{Alert: "NinesSLOBurnRate", Expr: expr, For: fmt.Sprintf("%ds", p.ForSeconds), Labels: alertLabels, Annotations: map[string]string{"summary": "SLO " + s.ID + " exceeds both burn-rate thresholds", "description": "Full provider windows at one completed cutoff; requires scrape coverage and nonzero traffic."}})
	}
	return g, nil
}
