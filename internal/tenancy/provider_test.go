package tenancy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProviderCredentialReferences(t *testing.T) {
	input := `tenants:
- id: test
  name: Test
  access: [{tokenEnv: TEST_OWNER, role: owner}]
  source: {kind: manual}
  connections:
  - name: datadog-prod
    kind: datadog
    url: https://api.datadoghq.eu
    credentialsEnv:
      apiKey: TEST_DD_API
      applicationKey: TEST_DD_APP
`
	lookup := func(name string) string {
		switch name {
		case "TEST_OWNER":
			return "synthetic-owner-credential-24-characters"
		case "TEST_DD_API":
			return "private-api-value"
		case "TEST_DD_APP":
			return "private-app-value"
		}
		return ""
	}
	resolved, err := Parse([]byte(input), lookup)
	if err != nil {
		t.Fatal(err)
	}
	if resolved[0].Connections[0].Credentials["apiKey"] != "private-api-value" {
		t.Fatal("credential not resolved")
	}
	b, err := json.Marshal(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "private-") {
		t.Fatal("credential serialized")
	}
	for _, bad := range []string{strings.Replace(input, "TEST_DD_APP", "MISSING", 1), strings.Replace(input, "    credentialsEnv:", "    bearerTokenEnv: TEST_DD_API\n    credentialsEnv:", 1), strings.Replace(input, "    credentialsEnv:", "    rules: {namespace: test, mode: operator, publisher: test}\n    credentialsEnv:", 1)} {
		if _, err = Parse([]byte(bad), lookup); err == nil {
			t.Fatal("invalid provider config accepted")
		}
	}
}

func TestNewRelicAccountAndCredentialConfiguration(t *testing.T) {
	input := `tenants:
- id: example
  name: Example
  access: [{tokenEnv: OWNER_TOKEN, role: owner}]
  source:
    kind: newrelic
    url: https://api.eu.newrelic.com/graphql
    options: {accountID: "123456"}
    credentialsEnv: {userKey: NEW_RELIC_USER_KEY}
`
	resolved, err := Parse([]byte(input), func(name string) string {
		if name == "OWNER_TOKEN" {
			return "synthetic-owner-token-at-least-24-characters"
		}
		if name == "NEW_RELIC_USER_KEY" {
			return "private-newrelic-credential"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved[0].Config.Source.Options["accountID"] != "123456" || resolved[0].Credentials["userKey"] != "private-newrelic-credential" {
		t.Fatal("configured account or user key lost")
	}
	body, err := json.Marshal(resolved)
	if err != nil || strings.Contains(string(body), "private-newrelic-credential") {
		t.Fatal("resolved key serialized", err)
	}
}

func TestDash0NativePublicationConfiguration(t *testing.T) {
	input := `tenants:
- id: production
  name: Production
  access: [{tokenEnv: OWNER_TOKEN, role: owner}]
  source: {kind: manual}
  connections:
  - name: dash0-production
    kind: dash0
    url: https://api.eu-west-1.aws.dash0.com
    options: {dataset: production}
    credentialsEnv: {bearerToken: DASH0_READ_TOKEN}
    rules:
      mode: native
      namespace: nines-prod
      bearerTokenEnv: DASH0_PUBLISH_TOKEN
`
	lookup := func(string) string { return "synthetic-private-value-at-least-24-characters" }
	resolved, err := Parse([]byte(input), lookup)
	if err != nil {
		t.Fatal(err)
	}
	if resolved[0].Connections[0].RulesBearerToken == "" || resolved[0].Connections[0].Config.Options["dataset"] != "production" {
		t.Fatal("native credentials or dataset lost")
	}
	for _, bad := range []string{strings.Replace(input, "mode: native", "mode: operator", 1), strings.Replace(input, "mode: native", "mode: native\n      autoDeploy: true", 1), strings.Replace(input, "      bearerTokenEnv: DASH0_PUBLISH_TOKEN\n", "", 1), strings.Replace(input, "mode: native", "mode: native\n      rulerURL: https://other.example", 1)} {
		if _, err = Parse([]byte(bad), lookup); err == nil {
			t.Fatal("unsafe native configuration accepted")
		}
	}
	b, err := json.Marshal(resolved)
	if err != nil || strings.Contains(string(b), "synthetic-private") {
		t.Fatal("native credential serialized", err)
	}
}

func TestGoogleCloudNativeConfigurationIsExplicit(t *testing.T) {
	input := `tenants:
- id: production
  name: Production
  access: [{tokenEnv: OWNER_TOKEN, role: owner}]
  source: {kind: manual}
  connections:
  - name: google-checkout
    kind: googlecloud
    url: https://monitoring.googleapis.com
    options: {projectID: nines-test, serviceID: checkout, auth: access-token}
    credentialsEnv: {accessToken: GCM_READ_TOKEN}
    rules: {mode: native, namespace: nines-prod, bearerTokenEnv: GCM_PUBLISH_TOKEN}
`
	lookup := func(string) string { return "synthetic-credential-at-least-24-characters" }
	if _, err := Parse([]byte(input), lookup); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{strings.Replace(input, "mode: native", "mode: native, autoDeploy: true", 1), strings.Replace(input, "serviceID: checkout, ", "", 1), strings.Replace(input, ", bearerTokenEnv: GCM_PUBLISH_TOKEN", "", 1), strings.Replace(input, "mode: native", "mode: operator", 1)} {
		if _, err := Parse([]byte(bad), lookup); err == nil {
			t.Fatal("unsafe Google configuration accepted")
		}
	}
}

func TestCollectionModeValidation(t *testing.T) {
	lookup := func(string) string { return "synthetic-credential-at-least-24-characters" }
	for _, mode := range []string{"automatic", "manual", "invalid"} {
		input := strings.Replace(validConfig, "  source:\n", "  source:\n    collectionMode: "+mode+"\n", 1)
		configs, err := Parse([]byte(input), lookup)
		if mode == "invalid" {
			if err == nil {
				t.Fatal("invalid mode accepted")
			}
			continue
		}
		if err != nil || configs[0].Config.Source.CollectionMode != mode {
			t.Fatal(mode, configs, err)
		}
	}
}

func TestDatadogNativePublicationConfiguration(t *testing.T) {
	input := `tenants:
- id: example
  name: Example
  access: [{tokenEnv: OWNER_TOKEN, role: owner}]
  source: {kind: manual}
  connections:
  - name: dd
    kind: datadog
    url: https://api.uk1.datadoghq.com
    credentialsEnv: {apiKey: DD_API_KEY, applicationKey: DD_APP_KEY}
    rules: {mode: native, namespace: production}
`
	lookup := func(string) string { return "synthetic-credential-at-least-24-characters" }
	if _, err := Parse([]byte(input), lookup); err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{", autoDeploy: true", ", reload: true", ", bearerTokenEnv: DD_API_KEY", ", directory: /tmp/rules", ", rulerURL: https://example.com", ", publisher: controller"} {
		if _, err := Parse([]byte(strings.Replace(input, "namespace: production}", "namespace: production"+extra+"}", 1)), lookup); err == nil {
			t.Fatal("unsafe native config", extra)
		}
	}
}

func TestNewRelicNativeEntityConfiguration(t *testing.T) {
	input := `tenants:
- id: example
  name: Example
  access: [{tokenEnv: OWNER_TOKEN, role: owner}]
  managedProviderRoutes: {newrelic: ["123456"], newrelic-entity: ["synthetic-entity"]}
  source: {kind: manual}
  connections:
  - name: nr
    kind: newrelic
    url: https://api.eu.newrelic.com/graphql
    options: {accountID: "123456"}
    credentialsEnv: {userKey: NR_USER_KEY}
    rules: {mode: native, namespace: production, entityGUID: synthetic-entity}
`
	lookup := func(string) string { return "synthetic-credential-at-least-24-characters" }
	configs, err := Parse([]byte(input), lookup)
	if err != nil || configs[0].Connections[0].Config.Rules.EntityGUID != "synthetic-entity" {
		t.Fatal(configs, err)
	}
	for _, bad := range []string{strings.Replace(input, ", entityGUID: synthetic-entity", "", 1), strings.Replace(input, "mode: native", "mode: operator", 1), strings.Replace(input, "namespace: production,", "namespace: production, autoDeploy: true,", 1), strings.Replace(input, "namespace: production,", "namespace: production, bearerTokenEnv: NR_USER_KEY,", 1), strings.Replace(input, "entityGUID: synthetic-entity", "entityGUID: 'invalid entity'", 1)} {
		if _, err := Parse([]byte(bad), lookup); err == nil {
			t.Fatal("invalid native New Relic configuration accepted")
		}
	}
}

func TestManagedVictoriaMetricsRoutePolicy(t *testing.T) {
	for _, route := range []string{"0", "42", "42:7", "4294967295:0"} {
		if !ValidVictoriaMetricsRoute(route) {
			t.Fatal("valid route rejected", route)
		}
		input := "tenants:\n- id: example\n  name: Example\n  source: {kind: manual}\n  access: [{tokenEnv: OWNER_TOKEN, role: owner}]\n  managedProviderRoutes: {victoriametrics: [\"" + route + "\"]}\n"
		if _, err := Parse([]byte(input), func(string) string { return "synthetic-credential-at-least-24-characters" }); err != nil {
			t.Fatal(err)
		}
	}
	for _, route := range []string{"", "multitenant", "*", "01", "42:", ":7", "42:07", "42:7:8", "4294967296", "-1", "42/7"} {
		if ValidVictoriaMetricsRoute(route) {
			t.Fatal("invalid route accepted", route)
		}
	}
}
