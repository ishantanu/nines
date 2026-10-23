package operator_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/operator"
	"github.com/ishantanu/nines/internal/rules"
)

// Uses only the agentless API container created by run.sh, never a host context.
// The real controller's ConfigMaps prove selection; no workload pod is scheduled.
func TestRealOperatorRuleSelectors(t *testing.T) {
	container := os.Getenv("NINES_OPERATOR_TEST_CONTAINER")
	if container == "" {
		t.Skip("run make test-operator for a disposable API and real controller")
	}
	if !strings.HasPrefix(container, "nines-operator-acceptance-") {
		t.Fatal("refusing non-fixture container")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	owned, err := exec.CommandContext(ctx, "docker", "inspect", "--format", `{{index .Config.Labels "nines.acceptance"}}`, container).Output()
	if err != nil || strings.TrimSpace(string(owned)) != "operator" {
		t.Fatal("container is not labelled as our disposable operator fixture", err)
	}
	kubectl := func(input []byte, args ...string) []byte {
		t.Helper()
		argv := append([]string{"exec", "-i", container, "kubectl"}, args...)
		cmd := exec.CommandContext(ctx, "docker", argv...)
		cmd.Stdin = bytes.NewReader(input)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture kubectl %v failed: %v: %s", args, err, out)
		}
		return out
	}
	apply := func(value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		kubectl(data, "apply", "-f", "-")
	}
	kubectl(nil, "label", "namespace", "nines-rules", "nines-test-selection=yes", "--overwrite")
	apply(map[string]any{"apiVersion": "monitoring.coreos.com/v1", "kind": "Prometheus", "metadata": map[string]string{"name": "selector-proof", "namespace": "nines-prometheus"}, "spec": map[string]any{
		"replicas": 0, "version": "v3.14.0", "serviceMonitorSelector": map[string]any{}, "podMonitorSelector": map[string]any{},
		"ruleSelector":          map[string]any{"matchLabels": map[string]string{"nines-test-rule": "selected"}},
		"ruleNamespaceSelector": map[string]any{"matchLabels": map[string]string{"nines-test-selection": "yes"}},
	}})
	type selectedRule struct{ id, namespace, label, group string }
	fixtures := []selectedRule{{id: "selected", namespace: "nines-rules", label: "selected"}, {id: "wrong-label", namespace: "nines-rules", label: "other"}, {id: "wrong-namespace", namespace: "nines-unselected", label: "selected"}}
	for i := range fixtures {
		f := &fixtures[i]
		slo := model.SLO{ID: f.id, Source: "production", Project: "acceptance", Service: "api", Name: f.id, Target: .999, WindowSeconds: 60, StepSeconds: 1, GoodQuery: "vector(99)", TotalQuery: "vector(100)", Policy: &model.Policy{ShortWindowSeconds: 2, LongWindowSeconds: 4, BurnRate: 2, ForSeconds: 2, MinCoverage: 1}}
		resource, err := operator.PrometheusRule(model.DefinitionFromSLO(slo), "acceptance", f.namespace, "", map[string]string{"nines-test-rule": f.label}, 3)
		if err != nil {
			t.Fatal(err)
		}
		var file rules.File
		if err := json.Unmarshal(resource.Spec, &file); err != nil {
			t.Fatal(err)
		}
		f.group = file.Groups[0].Name
		apply(resource)
	}
	waitGroups := func(want []string) {
		t.Helper()
		slices.Sort(want)
		var got []string
		var matchingSince time.Time
		for {
			var cms struct {
				Items []struct {
					Data map[string]string `json:"data"`
				} `json:"items"`
			}
			out := kubectl(nil, "get", "configmaps", "-n", "nines-prometheus", "-o", "json")
			if err := json.Unmarshal(out, &cms); err != nil {
				t.Fatal(err)
			}
			got = []string{}
			for _, cm := range cms.Items {
				for key, data := range cm.Data {
					if !strings.HasSuffix(key, ".yaml") {
						continue
					}
					var file rules.File
					if err := yaml.Unmarshal([]byte(data), &file); err != nil {
						t.Fatal(err)
					}
					for _, group := range file.Groups {
						got = append(got, group.Name)
					}
				}
			}
			slices.Sort(got)
			if reflect.DeepEqual(got, want) {
				if matchingSince.IsZero() {
					matchingSince = time.Now()
				}
				if time.Since(matchingSince) >= 2*time.Second {
					t.Logf("Controller selected exactly %v", got)
					return
				}
			} else {
				matchingSince = time.Time{}
			}
			select {
			case <-ctx.Done():
				t.Fatalf("selector reconciliation: got %v, want %v", got, want)
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
	// CRD persistence alone is insufficient: only a matching rule AND namespace is selected.
	waitGroups([]string{fixtures[0].group})
	kubectl(nil, "label", "prometheusrule", operator.RuleName(fixtures[1].id), "-n", fixtures[1].namespace, "nines-test-rule=selected", "--overwrite")
	waitGroups([]string{fixtures[0].group, fixtures[1].group})
	kubectl(nil, "label", "namespace", "nines-unselected", "nines-test-selection=yes", "--overwrite")
	waitGroups([]string{fixtures[0].group, fixtures[1].group, fixtures[2].group})
	kubectl(nil, "label", "namespace", "nines-rules", "nines-test-selection-")
	waitGroups([]string{fixtures[2].group})
	kubectl(nil, "label", "prometheusrule", operator.RuleName(fixtures[2].id), "-n", fixtures[2].namespace, "nines-test-rule-")
	waitGroups([]string{})
	t.Logf("Verified actual Prometheus Operator label/namespace selection and removal across %d generated SLO rule groups; no in-cluster pods were exercised", len(fixtures))
}
