// Package rules generates tenant/connection-scoped Prometheus recording rules.
package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/promqlvalidate"
	"github.com/ishantanu/nines/internal/tenancy"
)

type Rule struct {
	Record      string            `json:"record,omitempty" yaml:"record,omitempty"`
	Alert       string            `json:"alert,omitempty" yaml:"alert,omitempty"`
	Expr        string            `json:"expr" yaml:"expr"`
	For         string            `json:"for,omitempty" yaml:"for,omitempty"`
	Labels      map[string]string `json:"labels" yaml:"labels"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}
type Group struct {
	Name     string `json:"name" yaml:"name"`
	Interval string `json:"interval" yaml:"interval"`
	Rules    []Rule `json:"rules" yaml:"rules"`
}
type File struct {
	Groups []Group `json:"groups" yaml:"groups"`
}

func ConnectionID(s model.SLO) string {
	if s.Source == "" {
		return "primary"
	}
	return s.Source
}
func Generate(s model.SLO, tenant string) (Group, error) { return generate(s, tenant, "") }

// GenerateOwned isolates an operator resource's intermediate recordings as well
// as its ownership labels, including when a dataset has several ruler namespaces.
func GenerateOwned(s model.SLO, tenant, uid string) (Group, error) {
	if uid == "" || len(uid) > 128 {
		return Group{}, model.Invalid("a bounded resource UID is required")
	}
	return generate(s, tenant, uid)
}
func generate(s model.SLO, tenant, ownerUID string) (Group, error) {
	group, err := compile(s, tenant, ownerUID)
	if err != nil {
		return Group{}, err
	}
	if err := validateExpressions(group); err != nil {
		return Group{}, err
	}
	return group, nil
}

// Validate every final expression, including compiler-produced window and alert
// expressions, before a caller can export or deploy the group.
func validateExpressions(group Group) error {
	for _, rule := range group.Rules {
		name := rule.Record
		if name == "" {
			name = rule.Alert
		}
		if _, err := promqlvalidate.InstantVector(rule.Expr); err != nil {
			return model.Invalid("generated rule %s: %v", name, err)
		}
	}
	return nil
}

func compile(s model.SLO, tenant, ownerUID string) (Group, error) {
	if err := s.Validate(); err != nil {
		return Group{}, err
	}
	if !tenancy.ValidID(tenant) {
		return Group{}, model.Invalid("tenant is required for recording rules")
	}
	if !s.HasQueries() {
		return Group{}, model.Invalid("recording rules require both interval-count queries")
	}
	if s.ProviderWindow != nil {
		return generateWindows(s, tenant, ownerUID)
	}
	connection := ConnectionID(s)
	hash := sha256.Sum256([]byte(tenant + "/" + connection))
	g := Group{Name: "nines-" + s.ID + "-" + hex.EncodeToString(hash[:5]), Interval: fmt.Sprintf("%ds", s.StepSeconds)}
	base := map[string]string{"nines_managed": "true", "nines_tenant": tenant, "nines_source": connection, "nines_slo": s.ID}
	if ownerUID != "" {
		base["nines_resource_uid"] = ownerUID
	}
	labels := func(window int64) map[string]string {
		x := map[string]string{}
		for k, v := range base {
			x[k] = v
		}
		if window > 0 {
			x["window"] = fmt.Sprintf("%ds", window)
		}
		return x
	}
	selector := fmt.Sprintf("nines_tenant=%q,nines_source=%q,nines_slo=%q,nines_resource_uid=%q", tenant, connection, s.ID, ownerUID)
	add := func(name, expr string, window int64) {
		g.Rules = append(g.Rules, Rule{Record: name, Expr: expr, Labels: labels(window)})
	}
	// Both interval series are present only when both input expressions return
	// data. Queries must each return one aggregated interval-count series.
	// Evaluate source counts on the same completed UTC intervals as collection.
	// Group schedules have a fractional phase and can coincide with in-flight
	// scrapes. A one-step delayed, step-aligned subquery avoids inconsistent
	// numerator/denominator guards and keeps each count interval disjoint.
	stable := func(query string) string {
		return fmt.Sprintf("last_over_time((%s)[%ds:%ds] offset %ds)", query, s.StepSeconds, s.StepSeconds, s.StepSeconds)
	}
	// Printing parsed expressions removes comments safely before nesting them in
	// larger expressions. A trailing comment must not swallow generated syntax.
	good, err := promqlvalidate.InstantVector(s.GoodQuery)
	if err != nil {
		return Group{}, model.Invalid("goodQuery: %v", err)
	}
	total, err := promqlvalidate.InstantVector(s.TotalQuery)
	if err != nil {
		return Group{}, model.Invalid("totalQuery: %v", err)
	}
	goodQuery, totalQuery := stable(good.String()), stable(total.String())
	goodValue := "sum((" + goodQuery + "))"
	totalValue := "sum((" + totalQuery + "))"
	validInput := fmt.Sprintf("(count((%s)) == 1) and (count((%s)) == 1) and (%s >= 0) and (%s >= %s) and (%s <= 1e18) and ((%s == 0) or (%s >= 1e-9))", goodQuery, totalQuery, goodValue, totalValue, goodValue, totalValue, totalValue, totalValue)
	goodExpr := goodValue + " and on() (" + validInput + ")"
	totalExpr := totalValue + " and on() (" + validInput + ")"
	add("nines:sli_good:interval", goodExpr, 0)
	add("nines:sli_total:interval", totalExpr, 0)
	add("nines:slo_target:ratio", "vector("+strconv.FormatFloat(s.Target, 'g', -1, 64)+")", 0)
	windows := []int64{s.WindowSeconds}
	for _, p := range s.AlertPolicies() {
		windows = append(windows, p.ShortWindowSeconds, p.LongWindowSeconds)
	}
	seen := map[int64]bool{}
	for _, window := range windows {
		if seen[window] {
			continue
		}
		seen[window] = true
		good := fmt.Sprintf("sum_over_time(nines:sli_good:interval{%s}[%ds] offset %ds)", selector, window, s.StepSeconds)
		total := fmt.Sprintf("sum_over_time(nines:sli_total:interval{%s}[%ds] offset %ds)", selector, window, s.StepSeconds)
		valid := fmt.Sprintf(" and (%s > 0) and (%s >= 0) and (%s <= %s)", total, good, good, total)
		ratio := fmt.Sprintf("(%s / %s)%s", good, total, valid)
		burn := fmt.Sprintf("((%s - %s) / %s / %s)%s", total, good, total, strconv.FormatFloat(1-s.Target, 'g', -1, 64), valid)
		add("nines:sli:ratio", ratio, window)
		add("nines:slo_burn_rate:ratio", burn, window)
		coverage := fmt.Sprintf("count_over_time(nines:sli_good:interval{%s}[%ds] offset %ds) / %d", selector, window, s.StepSeconds, window/s.StepSeconds)
		add("nines:slo_coverage:ratio", coverage, window)
		add("nines:slo_budget_remaining:ratio", "1 - ("+burn+")", window)
	}
	for _, p := range s.AlertPolicies() {
		join := " on(nines_tenant,nines_source,nines_slo) "
		expr := fmt.Sprintf("(nines:slo_burn_rate:ratio{%s,window=%q} >= %g) and%s(nines:slo_burn_rate:ratio{%s,window=%q} >= %g) and%s(nines:slo_coverage:ratio{%s,window=%q} >= %g) and%s(nines:slo_coverage:ratio{%s,window=%q} >= %g)", selector, fmt.Sprintf("%ds", p.ShortWindowSeconds), p.BurnRate, join, selector, fmt.Sprintf("%ds", p.LongWindowSeconds), p.BurnRate, join, selector, fmt.Sprintf("%ds", p.ShortWindowSeconds), p.MinCoverage, join, selector, fmt.Sprintf("%ds", p.LongWindowSeconds), p.MinCoverage)
		alertLabels := labels(0)
		if p.Name != "" {
			alertLabels["nines_policy"] = p.Name
		}
		if p.Severity != "" {
			alertLabels["severity"] = p.Severity
		}
		alertLabels["nines_revision"] = fmt.Sprint(model.ConfigRevision(s))
		g.Rules = append(g.Rules, Rule{Alert: "NinesSLOBurnRate", Expr: expr, For: fmt.Sprintf("%ds", p.ForSeconds), Labels: alertLabels, Annotations: map[string]string{"summary": "SLO " + s.ID + " exceeds both burn-rate thresholds", "description": "Tenant " + tenant + ", connection " + connection + ". Requires sufficient coverage and nonzero traffic."}})
	}
	return g, nil
}
func Owned(g Group, tenant, source, slo string) bool {
	if len(g.Rules) == 0 {
		return false
	}
	for _, r := range g.Rules {
		if r.Labels["nines_managed"] != "true" || r.Labels["nines_tenant"] != tenant || r.Labels["nines_source"] != source || r.Labels["nines_slo"] != slo {
			return false
		}
	}
	return true
}

// DurationSeconds accepts human-friendly d/w plus Go h/m/s notation.
func DurationSeconds(value string) (int64, error) {
	if value == "" {
		return 0, model.Invalid("duration is required")
	}
	var seconds int64
	for len(value) > 0 {
		n := 0
		for n < len(value) && value[n] >= '0' && value[n] <= '9' {
			n++
		}
		if n == 0 || n == len(value) {
			return 0, model.Invalid("use whole durations such as 28d, 1h30m or 60s")
		}
		v, err := strconv.ParseInt(value[:n], 10, 64)
		if err != nil {
			return 0, model.Invalid("invalid duration")
		}
		unit := value[n]
		mult := int64(0)
		switch unit {
		case 'w':
			mult = 604800
		case 'd':
			mult = 86400
		case 'h':
			mult = 3600
		case 'm':
			mult = 60
		case 's':
			mult = 1
		default:
			return 0, model.Invalid("unsupported duration unit")
		}
		if v > int64(365*86400)/mult || seconds > 365*86400-v*mult {
			return 0, model.Invalid("duration exceeds one year")
		}
		seconds += v * mult
		value = value[n+1:]
	}
	return seconds, nil
}
