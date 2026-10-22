package rules

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGeneratedExpressionsRejectSyntaxAndNonVectors(t *testing.T) {
	for _, expr := range []string{"sum(", "1", `"text"`, "up[5m]"} {
		for _, rule := range []Rule{{Record: "test:record", Expr: expr}, {Alert: "TestAlert", Expr: expr}} {
			if err := validateExpressions(Group{Rules: []Rule{rule}}); err == nil {
				t.Fatalf("accepted invalid generated expression %q", expr)
			}
		}
	}
}

func TestGeneratedRulesSafelyNestCommentedQueries(t *testing.T) {
	s := fixture()
	s.GoodQuery += " # preserve this valid source query"
	s.TotalQuery += " # another comment"
	group, err := Generate(s, "commerce")
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range group.Rules {
		if strings.Contains(rule.Expr, "#") {
			t.Fatal("source comment leaked into nested generated syntax")
		}
	}
	if !strings.Contains(s.GoodQuery, "#") {
		t.Fatal("generation changed the supplied SLO")
	}
}

func TestInvalidPromQLCannotReachRuler(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(200) }))
	defer server.Close()
	s := fixture()
	s.GoodQuery = "sum("
	manager := Manager{Tenant: "commerce", Targets: map[string]Target{"production": {Connection: "production", Kind: "mimir", QueryURL: server.URL, RulerURL: server.URL, Namespace: "nines"}}}
	if _, err := manager.Plan(s); err == nil {
		t.Fatal("invalid query produced a deployment plan")
	}
	if _, err := manager.Apply(context.Background(), s, "reviewed-digest"); err == nil {
		t.Fatal("invalid query reached deployment")
	}
	if calls.Load() != 0 {
		t.Fatal("invalid query made a ruler request")
	}
}
