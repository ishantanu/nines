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
