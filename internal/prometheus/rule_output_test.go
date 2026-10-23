package prometheus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

func TestRuleOutputUsesQueryIdentityAndStoredTimestamp(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC()
	identity := model.RuleOutputIdentity{Tenant: "commerce", Source: "production", SLO: "availability", DefinitionDigest: strings.Repeat("a", 64), Revision: 2, StepSeconds: 10}
	for _, test := range []struct {
		name, digest, revision, value, state, code string
		count                                      int
	}{
		{"current", identity.DefinitionDigest, "2", strconv.FormatInt(now.Add(-time.Second).Unix(), 10), "verified", "rule_output_verified", 1},
		{"missing", identity.DefinitionDigest, "2", "0", "not_observed", "rule_output_missing", 0},
		{"other-definition", strings.Repeat("b", 64), "2", "0", "out_of_date", "rule_output_out_of_date", 1},
		{"other-revision", identity.DefinitionDigest, "1", "0", "out_of_date", "rule_output_out_of_date", 1},
		{"old-stored-sample", identity.DefinitionDigest, "2", strconv.FormatInt(now.Add(-5*time.Minute).Unix(), 10), "stale", "rule_output_stale", 1},
		{"future-stored-sample", identity.DefinitionDigest, "2", strconv.FormatInt(now.Add(10*time.Second).Unix(), 10), "stale", "rule_output_stale", 1},
		{"nan", identity.DefinitionDigest, "2", "NaN", "unknown", "rule_output_unavailable", 1},
		{"duplicate", identity.DefinitionDigest, "2", strconv.FormatInt(now.Unix(), 10), "unknown", "rule_output_ambiguous", 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer query-only-key" || r.Header.Get("X-Scope-OrgID") != "query-org" {
					t.Error("query identity was not pinned")
				}
				expression := r.URL.Query().Get("query")
				if !strings.HasPrefix(expression, "timestamp(nines:rule_definition:info{") || !strings.Contains(expression, " == 1)") || r.URL.Query().Get("limit") != "9" {
					t.Error("unbounded or timestamp-unsafe query", expression)
				}
				result := []any{}
				for n := 0; n < test.count; n++ {
					result = append(result, map[string]any{"metric": map[string]string{"nines_managed": "true", "nines_tenant": "commerce", "nines_source": "production", "nines_slo": "availability", "nines_revision": test.revision, "nines_definition_digest": test.digest}, "value": []any{now.Unix(), test.value}})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": result}})
			}))
			defer server.Close()
			client, err := NewMimir(server.URL, "query-org", "query-only-key")
			if err != nil {
				t.Fatal(err)
			}
			out := client.RuleOutput(context.Background(), identity, now)
			if out.State != test.state || out.Code != test.code || out.NextAction == "" {
				t.Fatal(out)
			}
			if out.State == "verified" && (out.ObservedAt == nil || !out.ObservedAt.Equal(now.Add(-time.Second))) {
				t.Fatal("query time substituted for sample time", out)
			}
		})
	}
}
