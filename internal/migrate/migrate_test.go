package migrate

import (
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/model"
)

const slothSpecYAML = `version: "prometheus/v1"
service: "myservice"
labels:
  owner: "myteam"
  repo: "myorg/myservice"
  tier: "2"
slos:
  - name: "requests-availability"
    objective: 99.9
    description: "Common SLO based on availability for HTTP request responses."
    sli:
      events:
        error_query: sum(rate(http_request_duration_seconds_count{job="myservice",code=~"(5..|429)"}[{{.window}}]))
        total_query: sum(rate(http_request_duration_seconds_count{job="myservice"}[{{.window}}]))
    alerting:
      name: MyServiceHighErrorRate
      labels:
        category: "availability"
      annotations:
        summary: "High error rate on 'myservice' requests responses"
      page_alert:
        labels:
          routing_key: myteam-pager
      ticket_alert:
        labels:
          slack_channel: "#alerts-myteam"
`

func definition(t *testing.T, y string) *model.SLO {
	t.Helper()
	d, err := model.ParseDefinition([]byte(y))
	if err != nil || d.SLO == nil {
		t.Fatalf("%v\n%s", err, y)
	}
	s := d.SLO.SLO()
	return &s
}

func TestSlothEventsBecomePerStepCountsAndEveryApproximationIsReported(t *testing.T) {
	r, err := Convert("sloth", []byte(slothSpecYAML), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Definitions) != 1 || len(r.Skipped) != 0 {
		t.Fatalf("%+v", r)
	}
	def := r.Definitions[0]
	if def.Name != "myservice-requests-availability" || def.Origin != "myservice/requests-availability" {
		t.Fatalf("%+v", def)
	}
	slo := definition(t, def.YAML)
	if slo.Target != 0.999 || slo.WindowSeconds != 30*86400 || slo.StepSeconds != 60 || slo.Service != "myservice" {
		t.Fatalf("%+v", slo)
	}
	// rate over {{.window}} became increase over the 1 minute step; good = total - errors.
	if !strings.Contains(slo.TotalQuery, `sum(increase(http_request_duration_seconds_count{job="myservice"}[1m]))`) || strings.Contains(slo.TotalQuery, "{{") {
		t.Fatalf("total: %s", slo.TotalQuery)
	}
	if !strings.HasPrefix(slo.GoodQuery, "(") || !strings.Contains(slo.GoodQuery, `) - (sum(increase(http_request_duration_seconds_count{job="myservice",code=~"(5..|429)"}[1m])))`) {
		t.Fatalf("good: %s", slo.GoodQuery)
	}
	// Imports never page by default.
	if string(slo.Lifecycle) != "experimental" || slo.AlertsEnabled() {
		t.Fatalf("an import must start experimental, got %q", slo.Lifecycle)
	}
	// The standard 30 day thresholds are reproduced, with no `for` delay.
	var burns []float64
	for _, p := range slo.Policies {
		burns = append(burns, p.BurnRate)
	}
	if len(burns) != 4 || burns[0] != 14.4 || burns[1] != 6 || burns[2] != 3 || burns[3] != 1 {
		t.Fatalf("burn rates %v", burns)
	}
	if slo.Labels["owner"] != "myteam" || slo.Labels["tier"] != "2" || slo.Labels["repo"] != "myorg/myservice" {
		t.Fatalf("%v", slo.Labels)
	}
	// What was not carried over is stated.
	all := ""
	for _, n := range def.Notes {
		all += n.Level + ":" + n.Field + ":" + n.Message + "\n"
	}
	for _, want := range []string{"lossy:description", "lossy:alerting", "review:sli.events", "review:policies", "review:windowSeconds"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing note %s in\n%s", want, all)
		}
	}
	if len(r.Limitations) == 0 || !strings.Contains(r.Notes[0].Message, "experimental") {
		t.Fatalf("%+v", r.Notes)
	}
}

func TestSlothQueriesThatCannotBeCountsAreSkippedNeverRewritten(t *testing.T) {
	cases := map[string]string{
		"raw ratio": `
    sli:
      raw:
        error_ratio_query: sum(rate(errors[{{.window}}])) / sum(rate(all[{{.window}}]))`,
		"plugin": `
    sli:
      plugin:
        id: "sloth-common/kubernetes/apiserver/availability"`,
		"no window placeholder": `
    sli:
      events:
        error_query: sum(rate(errors[5m]))
        total_query: sum(rate(all[{{.window}}]))`,
		"quantile": `
    sli:
      events:
        error_query: histogram_quantile(0.99, sum(rate(lat_bucket[{{.window}}])) by (le))
        total_query: sum(rate(all[{{.window}}]))`,
		"irate": `
    sli:
      events:
        error_query: sum(irate(errors[{{.window}}]))
        total_query: sum(rate(all[{{.window}}]))`,
		"unbalanced": `
    sli:
      events:
        error_query: sum(rate(errors[{{.window}}])
        total_query: sum(rate(all[{{.window}}]))`,
		"other template": `
    sli:
      events:
        error_query: sum(rate(errors{job="{{.job}}"}[{{.window}}]))
        total_query: sum(rate(all[{{.window}}]))`,
	}
	for name, sli := range cases {
		doc := "version: prometheus/v1\nservice: svc\nslos:\n  - name: x\n    objective: 99" + sli + "\n"
		r, err := Convert("sloth", []byte(doc), Options{})
		if err != nil || len(r.Definitions) != 0 || len(r.Skipped) != 1 || r.Skipped[0].Reason == "" {
			t.Errorf("%s: %+v %v", name, r, err)
		}
	}
	// The reason names the unsupported function.
	r, _ := Convert("sloth", []byte("version: prometheus/v1\nservice: s\nslos:\n  - name: q\n    objective: 99\n    sli:\n      events:\n        error_query: histogram_quantile(0.9, sum(rate(b[{{.window}}])) by (le))\n        total_query: sum(rate(a[{{.window}}]))\n"), Options{})
	if !strings.Contains(r.Skipped[0].Reason, "histogram_quantile") {
		t.Fatal(r.Skipped[0].Reason)
	}
}

func TestSlothIdiomsAndAlertSwitches(t *testing.T) {
	doc := `version: prometheus/v1
service: svc
slos:
  - name: a
    objective: 99.5
    sli:
      events:
        error_query: sum(rate(errs{job="a"}[{{ .window }}])) or vector(0)
        total_query: sum by (job) (rate(all{job="a"}[{{.window}}]))
    alerting:
      page_alert:
        disabled: true
  - name: b
    objective: 99
    sli:
      events:
        error_query: sum(rate(e[{{.window}}]))
        total_query: sum(rate(t[{{.window}}]))
    alerting:
      page_alert: {disabled: true}
      ticket_alert: {disabled: true}
`
	r, err := Convert("sloth", []byte(doc), Options{Lifecycle: "committed", WindowSeconds: 7 * 86400})
	if err != nil || len(r.Definitions) != 2 {
		t.Fatalf("%+v %v", r, err)
	}
	a := definition(t, r.Definitions[0].YAML)
	if string(a.Lifecycle) == "experimental" || !strings.Contains(a.GoodQuery, "or vector(0)") || a.Target != 0.995 {
		t.Fatalf("%+v", a)
	}
	for _, p := range a.Policies {
		if p.Severity == "critical" {
			t.Fatal("a disabled page alert must not create critical policies")
		}
	}
	// ticket-slow looks back 3 days, which fits a 7 day window.
	if len(a.Policies) != 2 {
		t.Fatalf("%+v", a.Policies)
	}
	if b := definition(t, r.Definitions[1].YAML); len(b.Policies) != 0 {
		t.Fatalf("%+v", b.Policies)
	}
	// A window shorter than an alert's lookback leaves that alert out, with a note.
	short, err := Convert("sloth", []byte(slothSpecYAML), Options{WindowSeconds: 12 * 3600})
	if err != nil || len(short.Definitions) != 1 {
		t.Fatalf("%+v %v", short, err)
	}
	if n := len(definition(t, short.Definitions[0].YAML).Policies); n != 2 {
		t.Fatalf("a 12 hour window keeps only the page alerts, got %d", n)
	}
	var left bool
	for _, n := range short.Definitions[0].Notes {
		left = left || strings.Contains(n.Message, "left out")
	}
	if !left {
		t.Fatal("dropped alerts must be reported")
	}
}

const pyrraYAML = `apiVersion: pyrra.dev/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: pyrra-apiserver-read-resource-errors
  namespace: monitoring
  labels:
    prometheus: k8s
    role: alert-rules
    Team/Name: platform
spec:
  target: '99'
  window: 2w
  description: Pyrra example
  indicator:
    ratio:
      errors:
        metric: apiserver_request_total{verb=~"LIST|GET",code=~"5.."}
      total:
        metric: apiserver_request_total{verb=~"LIST|GET"}
      grouping:
        - resource
---
apiVersion: pyrra.dev/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: latency
  labels:
    service: checkout
spec:
  target: "95"
  window: 4w
  indicator:
    latency:
      success:
        metric: http_request_duration_seconds_bucket{job="checkout",le="0.1"}
      total:
        metric: http_request_duration_seconds_count{job="checkout"}
  alerting:
    disabled: true
`

func TestPyrraRatioAndLatencyConvertWithTheirWindowsAndGroupingIsReported(t *testing.T) {
	r, err := Convert("pyrra", []byte(pyrraYAML), Options{Project: "platform", Source: "mimir"})
	if err != nil || len(r.Definitions) != 2 || len(r.Skipped) != 0 {
		t.Fatalf("%+v %v", r, err)
	}
	ratio := definition(t, r.Definitions[0].YAML)
	if ratio.Target != 0.99 || ratio.WindowSeconds != 14*86400 || ratio.Service != "monitoring" || ratio.Source != "mimir" || ratio.Project != "platform" {
		t.Fatalf("%+v", ratio)
	}
	if ratio.TotalQuery != `sum(increase(apiserver_request_total{verb=~"LIST|GET"}[1m]))` || !strings.Contains(ratio.GoodQuery, `code=~"5.."`) {
		t.Fatalf("%s | %s", ratio.GoodQuery, ratio.TotalQuery)
	}
	if ratio.Labels["prometheus"] != "k8s" || ratio.Labels["team_name"] != "platform" {
		t.Fatalf("labels must be sanitized and renamed with a note: %v", ratio.Labels)
	}
	notes := ""
	for _, n := range r.Definitions[0].Notes {
		notes += n.Level + ":" + n.Field + "|"
	}
	for _, want := range []string{"lossy:grouping", "review:labels.Team/Name", "lossy:description"} {
		if !strings.Contains(notes, want) {
			t.Errorf("missing %s in %s", want, notes)
		}
	}
	latency := definition(t, r.Definitions[1].YAML)
	if latency.Service != "checkout" || latency.WindowSeconds != 28*86400 || latency.GoodQuery != `sum(increase(http_request_duration_seconds_bucket{job="checkout",le="0.1"}[1m]))` || len(latency.Policies) != 0 {
		t.Fatalf("%+v", latency)
	}
}

func TestPyrraUnsupportedShapesAreSkippedWithAReason(t *testing.T) {
	base := "apiVersion: pyrra.dev/v1alpha1\nkind: ServiceLevelObjective\nmetadata:\n  name: x\nspec:\n  target: '99'\n  window: 4w\n  indicator:\n"
	for name, doc := range map[string]string{
		"native histogram": base + "    latencyNative:\n      latency: 100ms\n      total:\n        metric: m\n",
		"bool gauge":       base + "    boolGauge:\n      metric: up\n",
		"no indicator":     base + "    {}\n",
		"function metric":  base + "    ratio:\n      errors:\n        metric: sum(rate(a[5m]))\n      total:\n        metric: b\n",
		"long window":      strings.Replace(base, "window: 4w", "window: 12w", 1) + "    ratio:\n      errors:\n        metric: e\n      total:\n        metric: t\n",
		"bad window":       strings.Replace(base, "window: 4w", "window: soon", 1) + "    ratio:\n      errors:\n        metric: e\n      total:\n        metric: t\n",
		"bad target":       strings.Replace(base, "target: '99'", "target: '100'", 1) + "    ratio:\n      errors:\n        metric: e\n      total:\n        metric: t\n",
		"wrong kind":       strings.Replace(base, "kind: ServiceLevelObjective", "kind: Other", 1) + "    {}\n",
	} {
		r, err := Convert("pyrra", []byte(doc), Options{})
		if err != nil || len(r.Definitions) != 0 || len(r.Skipped) != 1 || r.Skipped[0].Reason == "" {
			t.Errorf("%s: %+v %v", name, r, err)
		}
	}
}

func TestNamesAreSafeUniqueAndInputIsBounded(t *testing.T) {
	doc := `version: prometheus/v1
service: "My Service!"
slos:
  - {name: "A_B", objective: 99, sli: {events: {error_query: "sum(rate(e[{{.window}}]))", total_query: "sum(rate(t[{{.window}}]))"}}}
  - {name: "a-b", objective: 99, sli: {events: {error_query: "sum(rate(e[{{.window}}]))", total_query: "sum(rate(t[{{.window}}]))"}}}
`
	r, err := Convert("sloth", []byte(doc), Options{})
	if err != nil || len(r.Definitions) != 2 || r.Definitions[0].Name != "my-service-a-b" || r.Definitions[1].Name != "my-service-a-b-2" {
		t.Fatalf("%+v %v", r.Definitions, err)
	}
	for _, bad := range []struct {
		tool, doc string
		opts      Options
	}{
		{"sloth", "", Options{}},
		{"sloth", strings.Repeat("a", MaxInputBytes+1), Options{}},
		{"datadog", "x: 1", Options{}},
		{"sloth", slothSpecYAML, Options{WindowSeconds: 60}},
		{"sloth", slothSpecYAML, Options{WindowSeconds: 40 * 86400}},
		{"sloth", slothSpecYAML, Options{Lifecycle: "live"}},
		{"sloth", "a: [", Options{}},
		{"sloth", strings.Repeat("---\nversion: prometheus/v1\nservice: s\n", MaxDocuments+1), Options{}},
	} {
		if _, err := Convert(bad.tool, []byte(bad.doc), bad.opts); err == nil {
			t.Errorf("accepted %s %q %+v", bad.tool, truncate(bad.doc, 20), bad.opts)
		}
	}
	// Neither tool's resource is accepted by the other, and a Kubernetes Sloth resource works.
	if r, _ := Convert("sloth", []byte(pyrraYAML), Options{}); len(r.Definitions) != 0 || len(r.Skipped) == 0 {
		t.Fatalf("%+v", r)
	}
	crd := "apiVersion: sloth.slok.dev/v1\nkind: PrometheusServiceLevel\nmetadata:\n  name: x\nspec:\n  service: svc\n  slos:\n    - name: a\n      objective: 99\n      sli:\n        events:\n          error_query: sum(rate(e[{{.window}}]))\n          total_query: sum(rate(t[{{.window}}]))\n"
	if r, err := Convert("sloth", []byte(crd), Options{}); err != nil || len(r.Definitions) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
}
