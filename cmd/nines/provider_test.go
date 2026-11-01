package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestProviderOfflineValidationDoesNotGeneratePrometheusRules(t *testing.T) {
	for _, file := range []string{"../../examples/googlecloud/api-availability.slo.yaml", "../../examples/datadog/api-availability.slo.yaml", "../../examples/newrelic/api-availability.slo.yaml", "../../examples/newrelic/api-latency.slo.yaml", "../../examples/victoriametrics/api-availability.slo.yaml"} {
		t.Run(file, func(t *testing.T) {
			var out bytes.Buffer
			if err := run([]string{"validate", "--offline", "-f", file, "--agent"}, &out); err != nil {
				t.Fatal(err)
			}
			var result map[string]string
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result["status"] != "valid" || strings.Contains(out.String(), "PromQL types") {
				t.Fatal(out.String())
			}
			out.Reset()
			if err := run([]string{"rules", "generate", "--offline", "--tenant", "production", "-f", file, "--agent"}, &out); err == nil || !strings.Contains(err.Error(), "requires PromQL") || out.Len() != 0 {
				t.Fatal("provider rules were not refused", out.String(), err)
			}
		})
	}
}
