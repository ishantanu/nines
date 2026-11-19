package migrate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

type pyrraMetric struct {
	Metric string `yaml:"metric"`
}

type pyrraDoc struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name      string            `yaml:"name"`
		Namespace string            `yaml:"namespace"`
		Labels    map[string]string `yaml:"labels"`
	} `yaml:"metadata"`
	Spec struct {
		Target      any    `yaml:"target"`
		Window      string `yaml:"window"`
		Description string `yaml:"description"`
		Indicator   struct {
			Ratio *struct {
				Errors   pyrraMetric `yaml:"errors"`
				Total    pyrraMetric `yaml:"total"`
				Grouping []string    `yaml:"grouping"`
			} `yaml:"ratio"`
			Latency *struct {
				Success  pyrraMetric `yaml:"success"`
				Total    pyrraMetric `yaml:"total"`
				Grouping []string    `yaml:"grouping"`
			} `yaml:"latency"`
			LatencyNative *yaml.Node `yaml:"latencyNative"`
			BoolGauge     *yaml.Node `yaml:"boolGauge"`
		} `yaml:"indicator"`
		Alerting struct {
			Disabled bool `yaml:"disabled"`
		} `yaml:"alerting"`
	} `yaml:"spec"`
}

var durationPart = regexp.MustCompile(`(\d+)([smhdw])`)

// parseWindow reads Pyrra windows such as 4w, 30d or 1d12h.
func parseWindow(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.Join(durationPart.FindAllString(s, -1), "") != s {
		return 0, fmt.Errorf("the window %q is not a duration such as 4w or 30d", s)
	}
	unit := map[string]int64{"s": 1, "m": 60, "h": 3600, "d": 86400, "w": 7 * 86400}
	var total int64
	for _, m := range durationPart.FindAllStringSubmatch(s, -1) {
		n, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil || n > 100000 {
			return 0, fmt.Errorf("the window %q is too large", s)
		}
		total += n * unit[m[2]]
	}
	return total, nil
}

func pyrraTarget(v any) (float64, error) {
	switch t := v.(type) {
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err != nil {
			return 0, fmt.Errorf("the target %q is not a percentage", t)
		}
		return percentToTarget(f)
	case float64:
		return percentToTarget(t)
	case int:
		return percentToTarget(float64(t))
	}
	return 0, fmt.Errorf("the SLO has no target")
}

// pyrraItems reads Pyrra ServiceLevelObjective resources.
func pyrraItems(docs []*yaml.Node, r *v1.MigrationResult, opts Options) ([]item, error) {
	var out []item
	for i, n := range docs {
		var d pyrraDoc
		if err := n.Decode(&d); err != nil {
			return nil, fmt.Errorf("document %d is not a valid Pyrra resource: %v", i+1, err)
		}
		origin := d.Metadata.Name
		if origin == "" {
			origin = fmt.Sprintf("document %d", i+1)
		}
		if !strings.HasPrefix(d.APIVersion, "pyrra.dev/") || d.Kind != "ServiceLevelObjective" {
			r.Skipped = append(r.Skipped, v1.MigrationSkipped{Origin: origin, Reason: fmt.Sprintf("%s/%s is not a Pyrra ServiceLevelObjective", d.APIVersion, d.Kind)})
			continue
		}
		it, reason := pyrraItem(d, opts)
		if reason != "" {
			r.Skipped = append(r.Skipped, v1.MigrationSkipped{Origin: origin, Reason: reason})
			continue
		}
		it.origin = origin
		out = append(out, it)
	}
	return out, nil
}

func pyrraItem(d pyrraDoc, opts Options) (item, string) {
	if d.Metadata.Name == "" {
		return item{}, "the resource has no metadata.name"
	}
	ind := d.Spec.Indicator
	var good, total string
	var notes []v1.MigrationNote
	var grouping []string
	var err error
	switch {
	case ind.LatencyNative != nil:
		return item{}, "latencyNative indicators use native histograms, which Nines cannot count per interval. Use a classic histogram (latency) or define the SLI in Nines directly"
	case ind.BoolGauge != nil:
		return item{}, "boolGauge indicators are gauges of 0 or 1, not event counters, so Nines cannot derive good and total counts from them"
	case ind.Ratio != nil:
		grouping = ind.Ratio.Grouping
		var errExpr string
		if errExpr, err = counterIncrease(ind.Ratio.Errors.Metric, stepText); err != nil {
			return item{}, "errors metric: " + err.Error()
		}
		if total, err = counterIncrease(ind.Ratio.Total.Metric, stepText); err != nil {
			return item{}, "total metric: " + err.Error()
		}
		good = "(" + total + ") - (" + errExpr + ")"
		notes = append(notes, note("info", "indicator.ratio", "Nines needs good events, so goodQuery is total minus errors, computed from the two metrics as increases over the 1 minute measurement step."))
	case ind.Latency != nil:
		grouping = ind.Latency.Grouping
		if good, err = counterIncrease(ind.Latency.Success.Metric, stepText); err != nil {
			return item{}, "success metric: " + err.Error()
		}
		if total, err = counterIncrease(ind.Latency.Total.Metric, stepText); err != nil {
			return item{}, "total metric: " + err.Error()
		}
		notes = append(notes, note("review", "indicator.latency", "The latency threshold is the le bucket in the success metric. Nines counts it as a plain good-over-total ratio; confirm the bucket exists for every series."))
	default:
		return item{}, "the resource has no ratio or latency indicator"
	}
	if len(grouping) > 0 {
		notes = append(notes, note("lossy", "grouping", fmt.Sprintf("Pyrra creates one SLO per value of %s. Nines imports one aggregate SLO. Add quality.cohortLabels after import to inspect the groups.", strings.Join(grouping, ", "))))
	}
	target, err := pyrraTarget(d.Spec.Target)
	if err != nil {
		return item{}, err.Error()
	}
	window, err := parseWindow(d.Spec.Window)
	if err != nil {
		return item{}, err.Error()
	}
	if window < 3600 || window > 31*dayWindow {
		return item{}, fmt.Sprintf("the window %s is outside what Nines supports (1 hour to 31 days)", d.Spec.Window)
	}
	if window%stepSeconds != 0 {
		return item{}, "the window is not a whole number of minutes"
	}
	service := d.Metadata.Labels["service"]
	if service == "" && d.Metadata.Namespace != "" {
		service = d.Metadata.Namespace
		notes = append(notes, note("review", "service", fmt.Sprintf("Pyrra has no service concept, so the namespace %q is the service. Set the service that owns this SLO.", service)))
	}
	if service == "" {
		service = d.Metadata.Name
		notes = append(notes, note("review", "service", "The resource has no service label or namespace, so the service is the SLO name. Set a service in the definition."))
	}
	lbl, ln := labels(d.Metadata.Labels)
	delete(lbl, "service")
	notes = append(notes, ln...)
	if d.Spec.Description != "" {
		notes = append(notes, note("lossy", "description", "Nines SLOs have no description field; it was not carried over."))
	}
	pol, pn := policies(window, !d.Spec.Alerting.Disabled, !d.Spec.Alerting.Disabled)
	notes = append(notes, pn...)
	if d.Spec.Alerting.Disabled {
		notes = append(notes, note("review", "alerting", "Alerting is disabled in Pyrra, so no burn-rate policies were created."))
	} else {
		notes = append(notes, note("review", "policies", "Alerts use the standard multi-window burn-rate policies scaled to the SLO window. Pyrra scales its own factors differently, so compare thresholds before retiring Pyrra's alerts."))
	}
	notes = append(notes, note("info", "windowSeconds", fmt.Sprintf("The window %s was carried over.", d.Spec.Window)))
	body := map[string]any{
		"service":       service,
		"source":        opts.Source,
		"target":        target,
		"windowSeconds": window,
		"stepSeconds":   stepSeconds,
		"goodQuery":     good,
		"totalQuery":    total,
	}
	if opts.Lifecycle != "committed" {
		body["lifecycle"] = opts.Lifecycle
	}
	if len(lbl) > 0 {
		body["labels"] = lbl
	}
	if len(pol) > 0 {
		body["policies"] = pol
	}
	return item{name: d.Metadata.Name, display: humanize("", d.Metadata.Name), spec: body, notes: notes}, ""
}
