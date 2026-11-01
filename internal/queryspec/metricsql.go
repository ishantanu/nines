package queryspec

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/VictoriaMetrics/metricsql"
)

var metricsQLCountPrefix = regexp.MustCompile(`(?i)^\s*sum\s*\(\s*(increase|increase_pure|increase_prometheus)\s*\(`)

// A count expression has exactly two call frames outside quoted label values.
// Bound this before parsing so unsupported nested functions/macros cannot cause
// expansion work before the AST admission checks run.
func boundedCountFrames(query string) bool {
	var quote byte
	escaped := false
	depth, calls := 0, 0
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' && quote != '`' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' || ch == '`' {
			quote = ch
			continue
		}
		switch ch {
		case '(':
			calls++
			depth++
			if calls > 2 || depth > 2 {
				return false
			}
		case ')':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return calls == 2 && depth == 0 && quote == 0
}

// MetricsQLCount uses the official parser, then restricts the AST to one
// aggregated counter increase with an explicit lookbehind equal to the SLO step.
// This prevents implicit windows, rates, offsets and truncated/grouped results.
func MetricsQLCount(query string, step int64) error {
	if len(query) == 0 || len(query) > 16000 || step < 1 || step > 3600 || !metricsQLCountPrefix.MatchString(query) || !boundedCountFrames(query) {
		return fmt.Errorf("MetricsQL SLO counts require sum(increase[_pure|_prometheus](metric[STEP])) with an explicit interval")
	}
	e, err := metricsql.Parse(query)
	if err != nil {
		return fmt.Errorf("invalid MetricsQL syntax")
	}
	aggregate, ok := e.(*metricsql.AggrFuncExpr)
	if !ok || strings.ToLower(aggregate.Name) != "sum" || len(aggregate.Args) != 1 || aggregate.Modifier.Op != "" || aggregate.Limit != 0 {
		return fmt.Errorf("MetricsQL count queries require one sum without grouping or limits")
	}
	fn, ok := aggregate.Args[0].(*metricsql.FuncExpr)
	if !ok || len(fn.Args) != 1 || fn.KeepMetricNames {
		return fmt.Errorf("MetricsQL count query requires one counter-increase function")
	}
	switch strings.ToLower(fn.Name) {
	case "increase", "increase_pure", "increase_prometheus":
	default:
		return fmt.Errorf("unsupported MetricsQL count function")
	}
	rollup, ok := fn.Args[0].(*metricsql.RollupExpr)
	if !ok || rollup.Window == nil || rollup.Offset != nil || rollup.Step != nil || rollup.InheritStep || rollup.At != nil {
		return fmt.Errorf("MetricsQL count queries require a fixed window without offsets, subqueries or timestamp overrides")
	}
	// Evaluate at two possible query steps to reject interval-relative durations.
	if rollup.Window.Duration(step*1000) != step*1000 || rollup.Window.Duration((step+1)*1000) != step*1000 {
		return fmt.Errorf("MetricsQL lookbehind must be fixed and equal to stepSeconds")
	}
	metric, ok := rollup.Expr.(*metricsql.MetricExpr)
	if !ok || len(metric.LabelFilterss) != 1 || len(metric.LabelFilterss[0]) == 0 || len(metric.LabelFilterss[0]) > 33 {
		return fmt.Errorf("MetricsQL requires one metric selector with at most 32 label filters")
	}
	name := metric.LabelFilterss[0][0]
	if name.Label != "__name__" || name.Value == "" || name.IsRegexp || name.IsNegative {
		return fmt.Errorf("MetricsQL requires one explicit metric name")
	}
	for _, label := range metric.LabelFilterss[0] {
		if label.IsRegexp {
			if _, err := regexp.Compile(label.Value); err != nil {
				return fmt.Errorf("invalid MetricsQL label regular expression")
			}
		}
	}
	return nil
}
