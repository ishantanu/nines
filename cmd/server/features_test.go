package main

import (
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/features"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestServerFeatureDefaults(t *testing.T) {
	if s, err := serverFeatures("", true, false); err != nil || len(s.List()) != len(features.Gated()) {
		t.Fatal("the demo does not show every feature", s.List(), err)
	}
	if s, err := serverFeatures("none", true, true); err != nil || len(s.List()) != 0 {
		t.Fatal("an explicit list does not override the demo default", s.List(), err)
	}
	if s, err := serverFeatures("", false, false); err != nil || len(s.List()) != 0 {
		t.Fatal("a server turns experimental features on by default", s.List(), err)
	}
	if _, err := serverFeatures("replay", false, true); err == nil {
		t.Fatal("an unknown feature was accepted")
	}
}

// A configured connection to a provider that is turned off stops startup with
// the fix, instead of silently ignoring the connection.
func TestStartupRefusesTurnedOffProviders(t *testing.T) {
	runtimes := []tenantRuntime{{Workspace: httpapi.Workspace{ID: "payments", Source: tenancy.Source{Kind: "prometheus"}, Connections: []tenancy.Connection{{Name: "dd", Source: tenancy.Source{Kind: "datadog"}}}}}}
	err := checkProviderGates(runtimes, features.Set{})
	if err == nil || !strings.Contains(err.Error(), "tenant payments uses the datadog provider") || !strings.Contains(err.Error(), "NINES_FEATURES=datadog") {
		t.Fatal(err)
	}
	on, _ := features.Parse("datadog")
	if err := checkProviderGates(runtimes, on); err != nil {
		t.Fatal(err)
	}
}
