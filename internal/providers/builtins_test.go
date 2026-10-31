package providers

import (
	"testing"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/telemetryprovider"
)

func TestBuiltinFactoryRoutingAndQueryValidation(t *testing.T) {
	r := Builtins()
	c, err := r.Open("datadog", telemetryprovider.Configuration{Endpoint: "https://api.datadoghq.eu", Credentials: map[string]string{"apiKey": "synthetic", "applicationKey": "synthetic"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = c.ValidateSLO(model.SLO{GoodQuery: "vector(1)", TotalQuery: "vector(1)", StepSeconds: 60}); err == nil {
		t.Fatal("PromQL accepted by Datadog")
	}
	if _, err = r.Open("datadog", telemetryprovider.Configuration{Endpoint: "https://api.datadoghq.eu", Credentials: map[string]string{"bearerToken": "synthetic"}}); err == nil {
		t.Fatal("wrong credential scheme accepted")
	}
	c, err = r.Open("prometheus", telemetryprovider.Configuration{Endpoint: "https://metrics.example"})
	if err != nil {
		t.Fatal(err)
	}
	if err = c.ValidateSLO(model.SLO{QueryLanguage: "datadog"}); err == nil {
		t.Fatal("Datadog accepted by Prometheus")
	}
	d, ok := r.Describe("datadog")
	if !ok || !d.Capabilities.IntervalCounts || d.Capabilities.Discovery || d.Capabilities.RuleDeployment {
		t.Fatal(d)
	}
}

func TestNewRelicFactoryUsesPinnedAccountAndUserKey(t *testing.T) {
	r := Builtins()
	cfg := telemetryprovider.Configuration{Endpoint: "https://api.eu.newrelic.com/graphql", Options: map[string]string{"accountID": "123456"}, Credentials: map[string]string{"userKey": "synthetic-key"}}
	c, err := r.Open("newrelic", cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := model.SLO{QueryLanguage: "nrql", StepSeconds: 60, GoodQuery: "SELECT count(*) FROM NinesRequest WHERE success = true", TotalQuery: "SELECT count(*) FROM NinesRequest"}
	if err = c.ValidateSLO(s); err != nil {
		t.Fatal(err)
	}
	d, ok := r.Describe("newrelic")
	if !ok || !d.Capabilities.IntervalCounts || d.Capabilities.RuleDeployment || d.QueryLanguages[0] != "nrql" {
		t.Fatal(d)
	}
	for _, bad := range []telemetryprovider.Configuration{
		{Endpoint: cfg.Endpoint, Options: map[string]string{"accountID": "123456", "query": "override"}, Credentials: cfg.Credentials},
		{Endpoint: cfg.Endpoint, Options: cfg.Options, Credentials: map[string]string{"licenseKey": "wrong-kind"}},
		{Endpoint: cfg.Endpoint, Options: cfg.Options, Credentials: cfg.Credentials, Dataset: "other-account"},
	} {
		if _, err = r.Open("newrelic", bad); err == nil {
			t.Fatal("unsafe New Relic routing accepted")
		}
	}
}

func TestVictoriaMetricsFactoryAndCapabilities(t *testing.T) {
	r := Builtins()
	cfg := telemetryprovider.Configuration{Endpoint: "https://vm.example", Options: map[string]string{"accountID": "42", "projectID": "7"}, Credentials: map[string]string{"bearerToken": "synthetic"}}
	c, err := r.Open("victoriametrics", cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := model.SLO{QueryLanguage: "metricsql", StepSeconds: 60, GoodQuery: "sum(increase_pure(requests_total[1m]))", TotalQuery: "sum(increase_pure(requests_total[1m]))"}
	if err = c.ValidateSLO(s); err != nil {
		t.Fatal(err)
	}
	d, ok := r.Describe("victoriametrics")
	if !ok || !d.Capabilities.IntervalCounts || d.Capabilities.RuleDeployment || d.Capabilities.WindowCounts {
		t.Fatal(d)
	}
	cfg.Options["accountID"] = "multitenant"
	if _, err = r.Open("victoriametrics", cfg); err == nil {
		t.Fatal("multitenant routing accepted")
	}
}

func TestDash0FactoryCapabilitiesAndDatasetBoundary(t *testing.T) {
	r := Builtins()
	cfg := telemetryprovider.Configuration{Endpoint: "https://api.eu-west-1.aws.dash0.com", Options: map[string]string{"dataset": "production"}, Credentials: map[string]string{"bearerToken": "synthetic"}}
	c, err := r.Open("dash0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	d, ok := r.Describe("dash0")
	if !ok || !d.Capabilities.NativeSLOPublishing || !d.Capabilities.WindowCounts || d.Capabilities.RuleDeployment {
		t.Fatal(d)
	}
	if _, ok = c.(telemetryprovider.WindowCollector); !ok {
		t.Fatal("window capability lost")
	}
	cfg.Dataset = "other"
	if _, err = r.Open("dash0", cfg); err == nil {
		t.Fatal("legacy tenant field bypassed Dash0 dataset")
	}
}

func TestGoogleCloudFactoryPinsProjectAndExplicitAuth(t *testing.T) {
	r := Builtins()
	cfg := telemetryprovider.Configuration{Endpoint: "https://monitoring.googleapis.com", Options: map[string]string{"projectID": "nines-test", "auth": "access-token"}, Credentials: map[string]string{"accessToken": "synthetic-key"}}
	c, err := r.Open("googlecloud", cfg)
	if err != nil {
		t.Fatal(err)
	}
	d, ok := r.Describe("googlecloud")
	if !ok || !d.Capabilities.IntervalCounts || !d.Capabilities.NativeSLOPublishing || d.Capabilities.RuleDeployment || d.Capabilities.WindowCounts {
		t.Fatal(d)
	}
	if _, ok = c.(telemetryprovider.Prober); !ok {
		t.Fatal("probe capability lost")
	}
	cfg.Dataset = "other"
	if _, err = r.Open("googlecloud", cfg); err == nil {
		t.Fatal("dataset bypass")
	}
	cfg.Dataset = ""
	cfg.Options["auth"] = ""
	if _, err = r.Open("googlecloud", cfg); err == nil {
		t.Fatal("implicit auth accepted")
	}
}
