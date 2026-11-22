package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/model"
)

// A deprecated resource version warns once on stderr, never on stdout, so
// scripts and agents parsing output are unaffected.
func TestDeprecatedDefinitionWarnsOnceOnStderr(t *testing.T) {
	var stderr bytes.Buffer
	warnings.Lock()
	previous := warnings.out
	warnings.out, warnings.seen = &stderr, map[string]bool{}
	warnings.Unlock()
	t.Cleanup(func() { warnings.Lock(); warnings.out = previous; warnings.Unlock() })
	for _, file := range []string{"../../api/v1beta1/testdata/v1alpha1/request-success.slo.yaml", "../../api/v1beta1/testdata/v1alpha1/request-success.slo.yaml", "../../examples/request-success.slo.yaml"} {
		var out bytes.Buffer
		if err := run([]string{"validate", "--offline", "-f", file, "--agent"}, &out); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "Warning:") {
			t.Fatal("warning written to stdout")
		}
	}
	if got := stderr.String(); got != "Warning: "+model.V1alpha1Deprecation+"\n" {
		t.Fatalf("stderr = %q", got)
	}
}
