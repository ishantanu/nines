package migrate

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

type slothAlert struct {
	Disabled    bool              `yaml:"disabled"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}

type slothSLO struct {
	Name        string            `yaml:"name"`
	Objective   float64           `yaml:"objective"`
	Description string            `yaml:"description"`
	Labels      map[string]string `yaml:"labels"`
	SLI         struct {
		Events *struct {
			ErrorQuery string `yaml:"error_query"`
			TotalQuery string `yaml:"total_query"`
		} `yaml:"events"`
		Raw *struct {
			ErrorRatioQuery string `yaml:"error_ratio_query"`
		} `yaml:"raw"`
		Plugin *struct {
			ID string `yaml:"id"`
		} `yaml:"plugin"`
	} `yaml:"sli"`
	Alerting struct {
		Name        string            `yaml:"name"`
		Labels      map[string]string `yaml:"labels"`
		Annotations map[string]string `yaml:"annotations"`
		PageAlert   slothAlert        `yaml:"page_alert"`
		TicketAlert slothAlert        `yaml:"ticket_alert"`
	} `yaml:"alerting"`
}

type slothSpec struct {
	Version string            `yaml:"version"`
	Service string            `yaml:"service"`
	Labels  map[string]string `yaml:"labels"`
	SLOs    []slothSLO        `yaml:"slos"`
}

type slothCRD struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec slothSpec `yaml:"spec"`
}

// slothItems reads the Sloth `prometheus/v1` spec and the Kubernetes
// PrometheusServiceLevel resource.
func slothItems(docs []*yaml.Node, r *v1.MigrationResult, opts Options) ([]item, error) {
	var out []item
	for i, n := range docs {
		var probe struct {
			Kind string `yaml:"kind"`
		}
		_ = n.Decode(&probe)
		var spec slothSpec
		origin := fmt.Sprintf("document %d", i+1)
		if probe.Kind != "" {
			var crd slothCRD
			if err := n.Decode(&crd); err != nil {
				return nil, fmt.Errorf("%s is not a valid Sloth resource: %v", origin, err)
			}
			if crd.Kind != "PrometheusServiceLevel" || !strings.HasPrefix(crd.APIVersion, "sloth.slok.dev/") {
				r.Skipped = append(r.Skipped, v1.MigrationSkipped{Origin: origin, Reason: fmt.Sprintf("%s/%s is not a Sloth PrometheusServiceLevel", crd.APIVersion, crd.Kind)})
				continue
			}
			spec = crd.Spec
			spec.Version = "prometheus/v1"
		} else if err := n.Decode(&spec); err != nil {
			return nil, fmt.Errorf("%s is not a valid Sloth spec: %v", origin, err)
		}
		if spec.Version != "prometheus/v1" {
			r.Skipped = append(r.Skipped, v1.MigrationSkipped{Origin: origin, Reason: fmt.Sprintf("unsupported Sloth version %q; only prometheus/v1 is converted", spec.Version)})
			continue
		}
		if strings.TrimSpace(spec.Service) == "" {
			r.Skipped = append(r.Skipped, v1.MigrationSkipped{Origin: origin, Reason: "the spec has no service"})
			continue
		}
		for _, s := range spec.SLOs {
			origin := spec.Service + "/" + s.Name
			it, reason := slothSLOItem(spec, s, opts)
			if reason != "" {
				r.Skipped = append(r.Skipped, v1.MigrationSkipped{Origin: origin, Reason: reason})
				continue
			}
			it.origin = origin
			out = append(out, it)
		}
	}
	return out, nil
}

func slothSLOItem(spec slothSpec, s slothSLO, opts Options) (item, string) {
	if strings.TrimSpace(s.Name) == "" {
		return item{}, "the SLO has no name"
	}
	switch {
	case s.SLI.Plugin != nil:
		return item{}, fmt.Sprintf("SLI plugin %q cannot be converted: a plugin generates its query at runtime. Write the SLI as error and total counters (sli.events) or define it in Nines directly", s.SLI.Plugin.ID)
	case s.SLI.Raw != nil:
		return item{}, "a raw error-ratio query has no event counts, so Nines cannot derive good and total counts from it. Rewrite it as error and total counters (sli.events) or define the SLI in Nines directly"
	case s.SLI.Events == nil:
		return item{}, "the SLO has no sli.events definition"
	}
	target, err := percentToTarget(s.Objective)
	if err != nil {
		return item{}, err.Error()
	}
	errQ, err := stepCount(s.SLI.Events.ErrorQuery, stepText)
	if err != nil {
		return item{}, "error_query: " + err.Error()
	}
	totQ, err := stepCount(s.SLI.Events.TotalQuery, stepText)
	if err != nil {
		return item{}, "total_query: " + err.Error()
	}
	notes := []v1.MigrationNote{
		note("review", "sli.events", "rate() over {{.window}} became increase() over the 1 minute measurement step. Verify the good and total counts against your dashboards."),
		note("info", "sli.events", "Nines needs good events, so goodQuery is total minus errors, computed from your two queries."),
	}
	lbl, ln := labels(spec.Labels, s.Labels)
	notes = append(notes, ln...)
	if s.Description != "" {
		notes = append(notes, note("lossy", "description", "Nines SLOs have no description field; it was not carried over."))
	}
	page, ticket := !s.Alerting.PageAlert.Disabled, !s.Alerting.TicketAlert.Disabled
	if s.Alerting.Name != "" || len(s.Alerting.Labels) > 0 || len(s.Alerting.Annotations) > 0 || len(s.Alerting.PageAlert.Labels) > 0 || len(s.Alerting.TicketAlert.Labels) > 0 || len(s.Alerting.PageAlert.Annotations) > 0 || len(s.Alerting.TicketAlert.Annotations) > 0 {
		notes = append(notes, note("lossy", "alerting", "Custom alert names, labels and annotations are not carried over; Nines generates its own alert rules from the burn-rate policies."))
	}
	pol, pn := policies(opts.WindowSeconds, page, ticket)
	notes = append(notes, pn...)
	if !page && !ticket {
		notes = append(notes, note("review", "alerting", "Both page and ticket alerts are disabled, so no burn-rate policies were created."))
	} else {
		notes = append(notes, note("review", "policies", "Alerts use the standard multi-window burn-rate policies scaled to the SLO window. Sloth derives its thresholds the same way, but compare them before retiring your Sloth alerts."))
	}
	notes = append(notes, note("review", "windowSeconds", fmt.Sprintf("Sloth takes the SLO period from its generator setting. The window is %s; change it with the window option if yours differs.", duration(opts.WindowSeconds))))
	body := map[string]any{
		"service":       spec.Service,
		"source":        opts.Source,
		"target":        target,
		"windowSeconds": opts.WindowSeconds,
		"stepSeconds":   stepSeconds,
		"goodQuery":     "(" + totQ + ") - (" + errQ + ")",
		"totalQuery":    totQ,
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
	return item{name: spec.Service + "-" + s.Name, display: humanize(spec.Service, s.Name), spec: body, notes: notes}, ""
}

func humanize(service, name string) string {
	return strings.TrimSpace(service + " " + strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " "))
}
