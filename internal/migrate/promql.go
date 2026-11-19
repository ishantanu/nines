package migrate

import (
	"fmt"
	"regexp"
	"strings"
)

// Sloth and Pyrra describe an SLI as rates over a window. Nines evaluates good
// and total event counts per measurement step, so a rate becomes an increase
// over the step. Only this simple, count-preserving shape is converted: any
// other expression is refused instead of silently changed.

var (
	windowPlaceholder = regexp.MustCompile(`\{\{\s*\.window\s*\}\}`)
	rateCall          = regexp.MustCompile(`(^|[^A-Za-z0-9_:])rate\(`)
	functionCall      = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_:]*)\s*\(`)
	counterSelector   = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*(\{\s*([a-zA-Z_][a-zA-Z0-9_]*\s*(=|!=|=~|!~)\s*"([^"\\]|\\.)*"\s*(,\s*[a-zA-Z_][a-zA-Z0-9_]*\s*(=|!=|=~|!~)\s*"([^"\\]|\\.)*"\s*)*,?\s*)?\})?$`)
)

// allowedFunctions are the only call names permitted after conversion. "by"
// and "without" are aggregation modifiers, and "vector" appears in the common
// `or vector(0)` idiom for series that may not exist yet.
var allowedFunctions = map[string]bool{"sum": true, "increase": true, "by": true, "without": true, "vector": true}

// stripLiterals removes quoted strings, label matcher bodies and range
// selectors so that only function and keyword names remain.
func stripLiterals(q string) string {
	var out strings.Builder
	depthBrace, depthBracket := 0, 0
	var quote rune
	escaped := false
	for _, r := range q {
		switch {
		case quote != 0:
			if escaped {
				escaped = false
			} else if r == '\\' {
				escaped = true
			} else if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'' || r == '`':
			quote = r
		case r == '{':
			depthBrace++
		case r == '}':
			if depthBrace > 0 {
				depthBrace--
			}
		case r == '[':
			depthBracket++
		case r == ']':
			if depthBracket > 0 {
				depthBracket--
			}
		case depthBrace == 0 && depthBracket == 0:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func balanced(q string) bool {
	depth := 0
	var quote rune
	escaped := false
	for _, r := range q {
		switch {
		case quote != 0:
			if escaped {
				escaped = false
			} else if r == '\\' {
				escaped = true
			} else if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'' || r == '`':
			quote = r
		case r == '(':
			depth++
		case r == ')':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0 && quote == 0
}

// stepCount converts a Sloth `{{.window}}` rate query into a per-step event
// count query. step is a PromQL duration such as "1m".
func stepCount(q, step string) (string, error) {
	q = strings.TrimSpace(q)
	if q == "" || len(q) > 4000 {
		return "", fmt.Errorf("the query is empty or longer than 4000 characters")
	}
	if !windowPlaceholder.MatchString(q) {
		return "", fmt.Errorf("the query does not use the {{.window}} placeholder, so it cannot be turned into a per-interval event count")
	}
	out := windowPlaceholder.ReplaceAllString(q, step)
	if strings.Contains(out, "{{") || strings.Contains(out, "}}") {
		return "", fmt.Errorf("the query uses a template variable other than {{.window}}")
	}
	if strings.Contains(out, "irate(") {
		return "", fmt.Errorf("irate() reports an instantaneous rate, not an event count")
	}
	out = rateCall.ReplaceAllString(out, "${1}increase(")
	return checkCount(out)
}

// checkCount verifies that an expression is made only of sum(increase(...))
// terms, optionally with by/without modifiers and `or vector(0)`.
func checkCount(q string) (string, error) {
	if !balanced(q) {
		return "", fmt.Errorf("the query has unbalanced parentheses or quotes")
	}
	for _, m := range functionCall.FindAllStringSubmatch(stripLiterals(q), -1) {
		if !allowedFunctions[strings.ToLower(m[1])] {
			return "", fmt.Errorf("the query uses %s(), which cannot be converted into an event count; only sum() over counter increases is supported", m[1])
		}
	}
	if !strings.Contains(q, "increase(") {
		return "", fmt.Errorf("the query does not count events over time")
	}
	return q, nil
}

// counterIncrease builds sum(increase(selector[step])) for a counter selector.
func counterIncrease(selector, step string) (string, error) {
	selector = strings.TrimSpace(selector)
	if !counterSelector.MatchString(selector) || len(selector) > 2000 {
		return "", fmt.Errorf("%q is not a metric name with optional label matchers", truncate(selector, 80))
	}
	return "sum(increase(" + selector + "[" + step + "]))", nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
