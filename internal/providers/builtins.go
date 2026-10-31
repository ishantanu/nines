// Package providers wires built-in Go adapters into the provider extension
// registry. New adapters register here; the collection engine uses interfaces.
package providers

import (
	"fmt"

	"github.com/ishantanu/nines/internal/dash0"
	"github.com/ishantanu/nines/internal/datadog"
	"github.com/ishantanu/nines/internal/googlecloud"
	"github.com/ishantanu/nines/internal/newrelic"
	"github.com/ishantanu/nines/internal/prometheus"
	"github.com/ishantanu/nines/internal/telemetryprovider"
	"github.com/ishantanu/nines/internal/victoriametrics"
)

func Builtins() *telemetryprovider.Registry {
	r := telemetryprovider.NewRegistry()
	for _, kind := range []string{"prometheus", "mimir"} {
		d := telemetryprovider.Descriptor{Kind: kind, QueryLanguages: []string{"promql"}, Capabilities: telemetryprovider.Capabilities{IntervalCounts: true, WindowCounts: true, Discovery: true, Dependencies: true, Cohorts: true, RuleDeployment: true}}
		if err := r.Register(d, func(cfg telemetryprovider.Configuration) (telemetryprovider.Collector, error) {
			if len(cfg.Options) > 0 {
				return nil, fmt.Errorf("provider does not support additional options")
			}
			if err := credentials(cfg.Credentials, "bearerToken", "basicUsername", "basicPassword"); err != nil {
				return nil, err
			}
			var c *prometheus.Client
			var err error
			if kind == "mimir" {
				c, err = prometheus.NewMimir(cfg.Endpoint, cfg.Dataset, cfg.Credentials["bearerToken"])
			} else {
				if cfg.Dataset != "" {
					return nil, fmt.Errorf("prometheus does not accept an upstream tenant")
				}
				c, err = prometheus.New(cfg.Endpoint, cfg.Credentials["bearerToken"])
			}
			if err == nil && (cfg.Credentials["basicUsername"] != "" || cfg.Credentials["basicPassword"] != "") {
				err = c.SetBasicAuth(cfg.Credentials["basicUsername"], cfg.Credentials["basicPassword"])
			}
			if err != nil {
				return nil, err
			}
			return c, nil
		}); err != nil {
			panic(err)
		}
	}
	if err := r.Register(telemetryprovider.Descriptor{Kind: "datadog", QueryLanguages: []string{"datadog"}, Capabilities: telemetryprovider.Capabilities{IntervalCounts: true, NativeSLOPublishing: true, NativeSLOResults: true, NativeSLOCatalog: true}}, func(cfg telemetryprovider.Configuration) (telemetryprovider.Collector, error) {
		if cfg.Dataset != "" {
			return nil, fmt.Errorf("datadog account routing comes from its credentials; dataset overrides are unsupported")
		}
		if len(cfg.Options) > 0 {
			return nil, fmt.Errorf("provider does not support additional options")
		}
		if err := credentials(cfg.Credentials, "apiKey", "applicationKey"); err != nil {
			return nil, err
		}
		return datadog.New(cfg.Endpoint, cfg.Credentials["apiKey"], cfg.Credentials["applicationKey"])
	}); err != nil {
		panic(err)
	}

	if err := r.Register(telemetryprovider.Descriptor{Kind: "newrelic", QueryLanguages: []string{"nrql"}, Capabilities: telemetryprovider.Capabilities{IntervalCounts: true, NativeSLOPublishing: true, NativeSLOResults: true, NativeSLOCatalog: true}}, func(cfg telemetryprovider.Configuration) (telemetryprovider.Collector, error) {
		if cfg.Dataset != "" {
			return nil, fmt.Errorf("new relic uses a configured accountID, without dataset overrides")
		}
		for key := range cfg.Options {
			if key != "accountID" {
				return nil, fmt.Errorf("unsupported New Relic option")
			}
		}
		if err := credentials(cfg.Credentials, "userKey"); err != nil {
			return nil, err
		}
		return newrelic.New(cfg.Endpoint, cfg.Options["accountID"], cfg.Credentials["userKey"])
	}); err != nil {
		panic(err)
	}

	if err := r.Register(telemetryprovider.Descriptor{Kind: "victoriametrics", QueryLanguages: []string{"promql", "metricsql"}, Capabilities: telemetryprovider.Capabilities{IntervalCounts: true}}, func(cfg telemetryprovider.Configuration) (telemetryprovider.Collector, error) {
		if cfg.Dataset != "" {
			return nil, fmt.Errorf("victoriametrics tenant routing requires accountID/projectID options, without dataset overrides")
		}
		for key := range cfg.Options {
			if key != "accountID" && key != "projectID" {
				return nil, fmt.Errorf("unsupported VictoriaMetrics option")
			}
		}
		if err := credentials(cfg.Credentials, "bearerToken", "basicUsername", "basicPassword"); err != nil {
			return nil, err
		}
		return victoriametrics.New(cfg.Endpoint, cfg.Options["accountID"], cfg.Options["projectID"], cfg.Credentials["bearerToken"], cfg.Credentials["basicUsername"], cfg.Credentials["basicPassword"])
	}); err != nil {
		panic(err)
	}
	if err := r.Register(telemetryprovider.Descriptor{Kind: "dash0", QueryLanguages: []string{"promql"}, Capabilities: telemetryprovider.Capabilities{IntervalCounts: true, WindowCounts: true, NativeSLOPublishing: true}}, func(cfg telemetryprovider.Configuration) (telemetryprovider.Collector, error) {
		if cfg.Dataset != "" {
			return nil, fmt.Errorf("Dash0 dataset must be set in options.dataset")
		}
		for key := range cfg.Options {
			if key != "dataset" {
				return nil, fmt.Errorf("unsupported Dash0 option")
			}
		}
		if err := credentials(cfg.Credentials, "bearerToken"); err != nil {
			return nil, err
		}
		return dash0.New(cfg.Endpoint, cfg.Options["dataset"], cfg.Credentials["bearerToken"])
	}); err != nil {
		panic(err)
	}
	if err := r.Register(telemetryprovider.Descriptor{Kind: "googlecloud", QueryLanguages: []string{"googlecloud"}, Capabilities: telemetryprovider.Capabilities{IntervalCounts: true, NativeSLOPublishing: true}}, func(cfg telemetryprovider.Configuration) (telemetryprovider.Collector, error) {
		if cfg.Dataset != "" {
			return nil, fmt.Errorf("google cloud project must be configured through options.projectID")
		}
		for key := range cfg.Options {
			if key != "projectID" && key != "serviceID" && key != "auth" {
				return nil, fmt.Errorf("unsupported Google Cloud option")
			}
		}
		if err := credentials(cfg.Credentials, "accessToken"); err != nil {
			return nil, err
		}
		return googlecloud.New(cfg.Endpoint, cfg.Options["projectID"], cfg.Options["serviceID"], cfg.Options["auth"], cfg.Credentials["accessToken"])
	}); err != nil {
		panic(err)
	}
	return r
}
func credentials(values map[string]string, allowed ...string) error {
	for key := range values {
		found := false
		for _, name := range allowed {
			if key == name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("provider does not support the configured credential fields")
		}
	}
	return nil
}

// Default is the host's compiled provider registry. Register additional adapters
// during startup, before accepting traffic. Tenant configuration cannot register.
var Default = Builtins()
