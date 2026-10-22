package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOperatorRenderRequiresExplicitScopeAndWritesNoPartialOutput(t *testing.T) {
	file := filepath.Join(t.TempDir(), "slo.yaml")
	if e := os.WriteFile(file, []byte("apiVersion: nines.dev/v1alpha1\nkind: ServiceLevelObjective\nmetadata:\n  name: api\n  project: commerce\nspec:\n  displayName: API\n  service: api\n  target: 0.999\n  windowSeconds: 3600\n  stepSeconds: 60\n  goodQuery: sum(increase(requests_total{status=\"200\"}[1m]))\n  totalQuery: sum(increase(requests_total[1m]))\n"), 0600); e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	if e := runOperator([]string{"render", "-f", file, "-namespace", "team-commerce", "-rules-only"}, &out); e == nil || out.Len() != 0 {
		t.Fatal("missing tenant emitted partial artifact", e)
	}
	if e := runOperator([]string{"render", "-f", file, "-namespace", "team-commerce", "-tenant", "commerce", "-rules-only"}, &out); e != nil {
		t.Fatal(e)
	}
	for _, value := range []string{"kind: PrometheusRule", "namespace: team-commerce", "nines_tenant: commerce"} {
		if !strings.Contains(out.String(), value) {
			t.Fatal(value, out.String())
		}
	}
	out.Reset()
	if e := runOperator([]string{"render", "-f", file, "-namespace", "team-commerce"}, &out); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "kind: ServiceLevelObjective") || !strings.Contains(out.String(), "nines.dev/project: commerce") || strings.Contains(out.String(), "kind: PrometheusRule") {
		t.Fatal("CRD mode emitted an unowned rule or lost project metadata", out.String())
	}

}
