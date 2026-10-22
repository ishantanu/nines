package tenancy

import (
	"strings"
	"testing"
)

func TestNamedConnectionsResolveSeparateQueryAndRuleCredentials(t *testing.T) {
	body := `tenants:
- id: commerce
  name: Commerce
  access:
  - tokenEnv: OWNER_TOKEN
    role: owner
  source:
    kind: manual
  connections:
  - name: production
    kind: mimir
    url: https://query.example/prometheus
    mimirOrgID: production-org
    bearerTokenEnv: QUERY_TOKEN
    rules:
      namespace: nines
      rulerURL: https://ruler.example/prometheus
      bearerTokenEnv: RULES_TOKEN
  - name: staging
    kind: prometheus
    url: https://staging.example
    rules:
      namespace: nines
      directory: /etc/prometheus/rules
`
	lookup := func(key string) string {
		return map[string]string{"OWNER_TOKEN": strings.Repeat("o", 24), "QUERY_TOKEN": "read-secret", "RULES_TOKEN": "rules-secret"}[key]
	}
	c, err := Parse([]byte(body), lookup)
	if err != nil {
		t.Fatal(err)
	}
	if len(c[0].Connections) != 2 || c[0].Connections[0].BearerToken != "read-secret" || c[0].Connections[0].RulesBearerToken != "rules-secret" || c[0].Connections[0].Config.MimirOrgID != "production-org" {
		t.Fatal(c)
	}
	for _, bad := range []string{strings.Replace(body, "name: production", "name: primary", 1), strings.Replace(body, "namespace: nines", "namespace: ../other", 1), strings.Replace(body, "directory: /etc/prometheus/rules", "directory: relative", 1), strings.Replace(body, "rulerURL: https://ruler.example/prometheus", "rulerURL: https://token:secret@ruler.example", 1)} {
		if _, err := Parse([]byte(bad), lookup); err == nil {
			t.Fatal("unsafe route accepted")
		}
	}
}
