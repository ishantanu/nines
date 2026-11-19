package operator_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/operator"
	"github.com/ishantanu/nines/internal/rules"
)

// The Prometheus branch consumes the actual ConfigMap selected by the upstream
// Prometheus Operator. Only the last file-mount/reload hop is provided by this
// agentless fixture; no in-cluster Prometheus Pod or production traffic is used.
func TestRealOperatorIndependentSeverityEvaluation(t *testing.T) {
	f := realCluster(t)
	f.kubectl(nil, "label", "namespace", "team-commerce", "nines-severity-test=yes", "--overwrite")
	f.apply(map[string]any{"apiVersion": "monitoring.coreos.com/v1", "kind": "Prometheus", "metadata": map[string]string{"name": "severity-proof", "namespace": "nines-prometheus"}, "spec": map[string]any{
		"replicas": 0, "version": "v3.14.0", "serviceMonitorSelector": map[string]any{}, "podMonitorSelector": map[string]any{},
		"ruleSelector":          map[string]any{"matchLabels": map[string]string{"nines-severity-test": "selected"}},
		"ruleNamespaceSelector": map[string]any{"matchLabels": map[string]string{"nines-severity-test": "yes"}},
	}})
	for _, kind := range []string{"prometheus-operator", "mimir"} {
		t.Run(kind, func(t *testing.T) { t.Parallel(); operatorSeverity(t, kind) })
	}
}
func operatorSeverity(t *testing.T, kind string) {
	f := realCluster(t)
	queryURL, org := os.Getenv("NINES_OPERATOR_TEST_PROMETHEUS"), ""
	source := operator.Source{Kind: kind, QueryURL: queryURL, PrometheusLabels: map[string]string{"nines-severity-test": "selected"}}
	id := "severity-prometheus"
	if kind == "mimir" {
		id = "severity-mimir"
		org = "operator-severity"
		queryURL = f.mimir + "/prometheus"
		source = operator.Source{Kind: kind, QueryURL: queryURL, RulerURL: queryURL, MimirOrgID: org, RulerNamespace: "severity-proof"}
	}
	endpoint, e := url.Parse(queryURL)
	if e != nil || endpoint.Hostname() != "127.0.0.1" {
		t.Fatal("missing disposable evaluator")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	request := func(method, path string, want int, payload ...[]byte) []byte {
		t.Helper()
		var bodyReader io.Reader
		if len(payload) != 0 {
			bodyReader = bytes.NewReader(payload[0])
		}
		r, e := http.NewRequestWithContext(f.ctx, method, queryURL+path, bodyReader)
		if e != nil {
			t.Fatal(e)
		}
		if org != "" {
			r.Header.Set("X-Scope-OrgID", org)
		}
		if bodyReader != nil {
			r.Header.Set("Content-Type", "application/yaml")
		}
		response, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer func() { _ = response.Body.Close() }()
		body, e := io.ReadAll(io.LimitReader(response.Body, 4<<20))
		if e != nil || response.StatusCode != want {
			t.Fatalf("fixture evaluator %s returned %d (%v): %.1024s", path, response.StatusCode, e, body)
		}
		return body
	}
	// Change a separate synthetic input group, never the reviewed SLO group.
	// Holding each phase avoids losing the independent-recovery assertion when
	// a valid missing-data interval makes both alerts inactive at once.
	setGood := func(good float64) {
		t.Helper()
		input := rules.Group{Name: "synthetic-" + id, Interval: "1s", Rules: []rules.Rule{
			{Record: "fixture:nines_operator_good:interval", Expr: fmt.Sprintf("vector(%g)", good), Labels: map[string]string{"run": id}},
			{Record: "fixture:nines_operator_total:interval", Expr: "vector(100)", Labels: map[string]string{"run": id}},
		}}
		if kind == "mimir" {
			body, err := yaml.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			request(http.MethodPost, "/config/v1/rules/severity-input", http.StatusAccepted, body)
		} else {
			directory := os.Getenv("NINES_OPERATOR_TEST_RULE_DIRECTORY")
			if !filepath.IsAbs(directory) {
				t.Fatal("missing private shared rule directory")
			}
			body, err := yaml.Marshal(rules.File{Groups: []rules.Group{input}})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, "synthetic-"+id+".yaml")
			if err = os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			if err = os.Chmod(path, 0644); err != nil {
				t.Fatal(err)
			}
			request(http.MethodPost, "/-/reload", http.StatusOK)
		}
		t.Logf("Synthetic input now requests %g/100 good events; SLO generation is unchanged", good)
	}
	setGood(100)
	good := fmt.Sprintf(`sum(fixture:nines_operator_good:interval{run=%q})`, id)
	total := fmt.Sprintf(`sum(fixture:nines_operator_total:interval{run=%q})`, id)
	slo := model.SLO{ID: id, Source: "primary", Project: "commerce", Service: "checkout", Name: id, Target: .999, WindowSeconds: 60, StepSeconds: 15, GoodQuery: good, TotalQuery: total, Policies: []model.Policy{
		{Name: "critical", Severity: "critical", ShortWindowSeconds: 15, LongWindowSeconds: 30, BurnRate: 8, ForSeconds: 15, MinCoverage: 1},
		{Name: "warning", Severity: "warning", ShortWindowSeconds: 30, LongWindowSeconds: 60, BurnRate: 2, ForSeconds: 30, MinCoverage: 1},
	}}
	definition := model.DefinitionFromSLO(slo)
	resource, e := operator.FromDefinition(model.Definition{SLO: &definition}, "team-commerce")
	if e != nil {
		t.Fatal(e)
	}
	f.apply(resource)
	controller := &operator.Controller{API: f.api, Config: operator.Config{Namespace: "team-commerce", Tenant: "commerce", Projects: []string{"commerce"}, Sources: map[string]operator.Source{"primary": source}}}
	reason := "RulesConfigured"
	if kind == "mimir" {
		reason = "RulesAccepted"
	}
	f.wait("operator configured current generation", func() bool {
		_ = controller.Reconcile(f.ctx, f.get("slo", id))
		return isReady(f.get("slo", id), reason)
	})
	parent := f.get("slo", id)
	native, e := operator.Native(parent)
	if e != nil {
		t.Fatal(e)
	}
	revision, e := operator.RuleRevision(parent, "commerce")
	if e != nil {
		t.Fatal(e)
	}
	desired := native.SLO.SLO()
	desired.Revision = revision
	group, e := rules.GenerateOwned(desired, "commerce", parent.Metadata.UID)
	if e != nil {
		t.Fatal(e)
	}
	if kind == "prometheus-operator" {
		f.wait("upstream Prometheus Operator selected the exact owned rule group", func() bool {
			var cms struct {
				Items []struct {
					Data map[string]string `json:"data"`
				} `json:"items"`
			}
			if json.Unmarshal(f.kubectl(nil, "get", "configmaps", "-n", "nines-prometheus", "-o", "json"), &cms) != nil {
				t.Fatal("invalid ConfigMaps")
			}
			for _, cm := range cms.Items {
				for _, value := range cm.Data {
					var file rules.File
					if yaml.Unmarshal([]byte(value), &file) != nil {
						continue
					}
					for _, actual := range file.Groups {
						if actual.Name == group.Name {
							got, _ := json.Marshal(actual)
							want, _ := json.Marshal(group)
							if operator.CanonicalDigest(got) != operator.CanonicalDigest(want) {
								t.Fatal("selected ConfigMap changed the compiled rule group")
							}
							body, e := yaml.Marshal(rules.File{Groups: []rules.Group{actual}})
							if e != nil {
								t.Fatal(e)
							}
							directory := os.Getenv("NINES_OPERATOR_TEST_RULE_DIRECTORY")
							if !filepath.IsAbs(directory) {
								t.Fatal("missing private shared rule directory")
							}
							path := filepath.Join(directory, id+".yaml")
							if e = os.WriteFile(path, body, 0600); e != nil {
								t.Fatal(e)
							}
							// The fixture deliberately uses umask 077 for credentials.
							// Only synthetic rules must be readable by Prometheus's
							// nonroot UID across a Linux bind mount; creation mode
							// alone is masked by the inherited umask.
							if e = os.Chmod(path, 0644); e != nil {
								t.Fatal(e)
							}
							request(http.MethodPost, "/-/reload", http.StatusOK)
							return true
						}
					}
				}
			}
			return false
		})
	}
	type liveRule struct {
		State          string            `json:"state"`
		Health         string            `json:"health"`
		LastError      string            `json:"lastError"`
		LastEvaluation time.Time         `json:"lastEvaluation"`
		Duration       float64           `json:"duration"`
		Labels         map[string]string `json:"labels"`
		Alerts         []struct {
			ActiveAt time.Time `json:"activeAt"`
		} `json:"alerts"`
	}
	pending := map[string]time.Time{}
	fired := map[string]bool{}
	read := func() map[string]liveRule {
		t.Helper()
		var envelope struct {
			Status string `json:"status"`
			Data   struct {
				Groups []struct {
					Name  string     `json:"name"`
					Rules []liveRule `json:"rules"`
				} `json:"groups"`
			} `json:"data"`
		}
		if json.Unmarshal(request(http.MethodGet, "/api/v1/rules", http.StatusOK), &envelope) != nil || envelope.Status != "success" {
			t.Fatal("invalid runtime rules")
		}
		out := map[string]liveRule{}
		for _, g := range envelope.Data.Groups {
			if g.Name != group.Name {
				continue
			}
			for _, r := range g.Rules {
				if r.Health == "err" || r.LastError != "" {
					t.Fatal("compiled operator rules failed evaluation", r.LastError)
				}
				policy := r.Labels["nines_policy"]
				if policy == "" {
					continue
				}
				if r.Labels["nines_revision"] != strconv.FormatInt(revision, 10) || r.Labels["severity"] != policy || r.Labels["nines_resource_uid"] != parent.Metadata.UID {
					t.Fatal("runtime rule lost revision, severity or owner UID", r.Labels)
				}
				out[policy] = r
				if r.State == "pending" {
					if len(r.Alerts) != 1 || r.Alerts[0].ActiveAt.IsZero() {
						t.Fatal("pending receipt missing")
					}
					pending[policy] = r.Alerts[0].ActiveAt
				}
				if r.State == "firing" {
					if pending[policy].IsZero() || len(r.Alerts) != 1 || !pending[policy].Equal(r.Alerts[0].ActiveAt) || r.LastEvaluation.Sub(pending[policy]) < time.Duration(r.Duration*float64(time.Second))-100*time.Millisecond {
						t.Fatal("firing without the configured continuous pending period", policy)
					}
					fired[policy] = true
				}
			}
		}
		return out
	}
	fullCoverage := func() bool {
		expression := fmt.Sprintf(`nines:slo_coverage:ratio{nines_slo=%q,window="60s"} == 1`, id)
		var result struct {
			Data struct {
				Result []json.RawMessage `json:"result"`
			} `json:"data"`
		}
		if err := json.Unmarshal(request(http.MethodGet, "/api/v1/query?query="+url.QueryEscape(expression), http.StatusOK), &result); err != nil {
			t.Fatal(err)
		}
		return len(result.Data.Result) == 1
	}
	f.wait("inactive baseline with complete long-window coverage", func() bool {
		r := read()
		return len(r) == 2 && r["critical"].State == "inactive" && r["warning"].State == "inactive" && fullCoverage()
	})
	setGood(90)
	criticalFirst := false
	f.wait("critical and warning fire independently under the same generation", func() bool {
		r := read()
		if r["critical"].State == "firing" && r["warning"].State == "pending" {
			criticalFirst = true
		}
		return len(r) == 2 && r["critical"].State == "firing" && r["warning"].State == "firing"
	})
	if !criticalFirst || !fired["critical"] || !fired["warning"] {
		t.Fatal("independent severity transition not observed")
	}
	setGood(99.5) // Stable 5x burn: below critical 8x, above warning 2x.
	var independentSince time.Time
	f.wait("critical recovers while warning remains firing across evaluations", func() bool {
		r := read()
		if r["critical"].State != "inactive" || r["warning"].State != "firing" {
			independentSince = time.Time{}
			return false
		}
		if independentSince.IsZero() {
			independentSince = r["critical"].LastEvaluation
		}
		return r["critical"].LastEvaluation.Sub(independentSince) >= time.Duration(slo.StepSeconds)*time.Second
	})
	setGood(100)
	f.wait("warning also recovers without a generation or SLO rule rewrite", func() bool {
		r := read()
		return len(r) == 2 && r["critical"].State == "inactive" && r["warning"].State == "inactive"
	})
	f.wait("healthy recovery with complete long-window coverage", func() bool {
		r := read()
		if len(r) != 2 || r["critical"].State != "inactive" || r["warning"].State != "inactive" {
			return false
		}
		return fullCoverage()
	})
	if current := f.get("slo", id); current.Metadata.UID != parent.Metadata.UID || current.Metadata.Generation != parent.Metadata.Generation {
		t.Fatal("test changed the SLO identity or generation")
	}
	_ = controller.Reconcile(f.ctx, f.get("slo", id))
	final := f.get("slo", id)
	if final.Status.Evaluation == nil || final.Status.Evaluation.State != "evaluating" || final.Status.RuleOutput == nil || final.Status.RuleOutput.State != "verified" {
		t.Fatal("operator did not report current runtime/output evidence", final.Status)
	}
	t.Log("Verified actual operator output, upstream selection, healthy coverage, independent pending/firing/recovery and unchanged generation in", kind)
}
