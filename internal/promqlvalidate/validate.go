// Package promqlvalidate provides offline PromQL checks shared by SLO admission
// and rule generation. It does not query providers or verify metric semantics.
package promqlvalidate

import (
	"fmt"

	"github.com/prometheus/prometheus/promql/parser"
)

// InstantVector parses a complete expression using the pinned Prometheus grammar
// and type checker. Nines interval inputs and generated rules require vectors.
func InstantVector(query string) (parser.Expr, error) {
	expr, err := parser.NewParser(parser.Options{}).ParseExpr(query)
	if err != nil {
		return nil, fmt.Errorf("invalid PromQL: %w", err)
	}
	if expr.Type() != parser.ValueTypeVector {
		return nil, fmt.Errorf("expected an instant vector, got %s", expr.Type())
	}
	return expr, nil
}
