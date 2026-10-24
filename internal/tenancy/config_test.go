package tenancy

import (
	"encoding/json"
	"strings"
	"testing"
)

const validConfig = `tenants:
- id: commerce
  name: Commerce
  access:
  - tokenEnv: COMMERCE_TOKEN
    role: owner
  source:
    kind: mimir
    url: https://metrics.example/prometheus
    mimirOrgID: commerce-production
    bearerTokenEnv: METRICS_TOKEN
`

func TestTenantConfigValidationAndSecrets(t *testing.T) {
	lookup := func(name string) string {
		if name == "COMMERCE_TOKEN" {
			return "tenant-credential-at-least-24-characters"
		}
		if name == "METRICS_TOKEN" {
			return "upstream-private-secret"
		}
		return ""
	}
	r, err := Parse([]byte(validConfig), lookup)
	if err != nil || len(r) != 1 {
		t.Fatal(r, err)
	}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "tenant-credential-at-least-24-characters") || strings.Contains(string(b), "upstream-private-secret") {
		t.Fatal("resolved credential serializes")
	}
	for name, text := range map[string]string{
		"unknown key":        strings.Replace(validConfig, "  source:", "  unknown: true\n  source:", 1),
		"duplicate key":      strings.Replace(validConfig, "    kind: mimir", "    kind: mimir\n    kind: manual", 1),
		"multiple documents": validConfig + "\n---\ntenants: []",
		"unknown role":       strings.Replace(validConfig, "role: owner", "role: admin", 1),
		"missing secret":     strings.Replace(validConfig, "COMMERCE_TOKEN", "MISSING_TOKEN", 1),
		"federation":         strings.Replace(validConfig, "commerce-production", "commerce|other", 1),
		"header injection":   strings.Replace(validConfig, "commerce-production", `"commerce\r\nX-Override: bad"`, 1),
		"conflicting auth":   strings.Replace(validConfig, "    bearerTokenEnv: METRICS_TOKEN", "    bearerTokenEnv: METRICS_TOKEN\n    basicUsername: user\n    basicPasswordEnv: METRICS_TOKEN", 1),
		"manual upstream":    strings.Replace(validConfig, "kind: mimir", "kind: manual", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(text), lookup); err == nil {
				t.Fatal("accepted invalid tenant config")
			}
		})
	}
	if _, err = Parse([]byte(validConfig), func(string) string { return "short" }); err == nil {
		t.Fatal("short tenant token accepted")
	}
}

func TestTokenlessTenantsRequireIdentityOptIn(t *testing.T) {
	body := []byte("tenants:\n- id: commerce\n  name: Commerce\n  source: {kind: manual}\n")
	if _, err := Parse(body, func(string) string { return "" }); err == nil {
		t.Fatal("tokenless production tenant accepted without identity")
	}
	result, err := parse(body, func(string) string { return "" }, true)
	if err != nil || len(result) != 1 || len(result[0].Grants) != 0 {
		t.Fatal(result, err)
	}
}

func TestTenantIntentModelCredentialsStayServerOwned(t *testing.T) {
	body := strings.Replace(validConfig, "  source:", "  intentModel:\n    endpoint: https://models.example/v1/chat/completions\n    model: structured-model\n    apiKeyEnv: MODEL_KEY\n  source:", 1)
	lookup := func(name string) string {
		switch name {
		case "COMMERCE_TOKEN":
			return "tenant-credential-at-least-24-characters"
		case "METRICS_TOKEN":
			return "metrics-secret"
		case "MODEL_KEY":
			return "private-model-key"
		}
		return ""
	}
	tenants, err := Parse([]byte(body), lookup)
	if err != nil || tenants[0].IntentModelKey != "private-model-key" {
		t.Fatal(tenants, err)
	}
	encoded, _ := json.Marshal(tenants)
	if strings.Contains(string(encoded), "private-model-key") {
		t.Fatal("model secret serialized")
	}
	if _, err = Parse([]byte(strings.Replace(body, "MODEL_KEY", "MISSING", 1)), lookup); err == nil {
		t.Fatal("missing model credential accepted")
	}
}

func TestStaticRuleReloadIsPrometheusOnly(t *testing.T) {
	base := `tenants:
- id: test
  name: Test
  access:
  - tokenEnv: TEST_TOKEN
    role: owner
  source:
    kind: prometheus
    url: https://metrics.test
    rules:
      namespace: nines
      directory: /rules
      reload: true
      autoDeploy: true
`
	lookup := func(string) string { return "synthetic-token-at-least-24-characters" }
	resolved, err := Parse([]byte(base), lookup)
	if err != nil || !resolved[0].Config.Source.Rules.Reload || !resolved[0].Config.Source.Rules.AutoDeploy {
		t.Fatal(resolved, err)
	}
	if _, err := Parse([]byte(strings.Replace(base, "reload: true", "reload: false", 1)), lookup); err == nil {
		t.Fatal("automatic Prometheus deployment without reload accepted")
	}
	mimir := strings.Replace(base, "kind: prometheus", "kind: mimir\n    mimirOrgID: alpha", 1)
	mimir = strings.Replace(mimir, "directory: /rules", "rulerURL: https://metrics.test/prometheus", 1)
	if _, err := Parse([]byte(mimir), lookup); err == nil {
		t.Fatal("Mimir reload accepted")
	}
}

func TestOperatorDestinationRequiresTrustedNamespaceAndPublisher(t *testing.T) {
	base := `tenants:
- id: commerce
  name: Commerce
  access: [{tokenEnv: TOKEN, role: owner}]
  source:
    kind: prometheus
    url: https://metrics.test
    rules: {mode: operator, namespace: team-commerce, publisher: portal, autoDeploy: true}
`
	lookup := func(string) string { return "synthetic-token-at-least-24-characters" }
	resolved, err := Parse([]byte(base), lookup)
	if err != nil || resolved[0].Config.Source.Rules.Mode != "operator" || !resolved[0].Config.Source.Rules.AutoDeploy {
		t.Fatal(resolved, err)
	}
	for _, replacement := range []string{"publisher: ''", "publisher: portal, directory: /rules", "publisher: portal, reload: true", "publisher: portal, rulerURL: https://ruler.test", "publisher: portal, bearerTokenEnv: TOKEN"} {
		if _, err := Parse([]byte(strings.Replace(base, "publisher: portal", replacement, 1)), lookup); err == nil {
			t.Fatal("invalid operator destination accepted", replacement)
		}
	}
	duplicate := base + strings.Replace(strings.TrimPrefix(base, "tenants:\n"), "id: commerce", "id: another", 1)
	if _, err := Parse([]byte(duplicate), lookup); err == nil {
		t.Fatal("cross-tenant namespace reuse accepted")
	}
}

func TestSSOOnlyTenantCanUseNamedOperatorConnection(t *testing.T) {
	body := []byte(`tenants:
- id: commerce
  name: Commerce
  source: {kind: manual}
  connections:
  - name: production
    kind: prometheus
    url: https://metrics.test
    rules: {mode: operator, namespace: team-commerce, publisher: portal}
`)
	if _, err := Parse(body, func(string) string { return "" }); err == nil {
		t.Fatal("identity-disabled tokenless tenant accepted")
	}
	resolved, err := parse(body, func(string) string { return "" }, true)
	if err != nil || len(resolved[0].Connections) != 1 || resolved[0].Connections[0].Config.Rules.Mode != "operator" {
		t.Fatal(resolved, err)
	}
}

func TestTenantCompatibleAuditArchiveIsOptionalAndValidated(t *testing.T) {
	lookup := func(string) string { return "synthetic-token-at-least-24-characters" }
	withoutArchive, err := Parse([]byte(validConfig), lookup)
	if err != nil || withoutArchive[0].Config.AuditArchive != nil {
		t.Fatal("tenant requires no external archive by default", err)
	}
	body := validConfig + `  auditArchive:
    provider: s3-compatible
    endpoint: https://rgw.example.com
    bucket: nines-audit
    region: us-east-1
    prefix: nines/audit
    retentionDays: 365
`
	resolved, err := Parse([]byte(body), lookup)
	if err != nil {
		t.Fatal(err)
	}
	archive := resolved[0].Config.AuditArchive
	if archive == nil || archive.Provider != "s3-compatible" || archive.Endpoint != "https://rgw.example.com" || archive.ExpectedBucketOwner != "" {
		t.Fatal("gateway configuration was not retained", archive)
	}
	for name, invalid := range map[string]string{
		"AWS owner required": strings.Replace(body, "provider: s3-compatible", "provider: s3", 1),
		"obsolete provider":  strings.Replace(body, "provider: s3-compatible", "provider: minio", 1),
		"missing endpoint":   strings.Replace(body, "    endpoint: https://rgw.example.com\n", "", 1),
		"insecure endpoint":  strings.Replace(body, "https://rgw.example.com", "http://rgw.example.com", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(invalid), lookup); err == nil {
				t.Fatal("accepted invalid archive configuration")
			}
		})
	}
}
