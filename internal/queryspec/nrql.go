package queryspec

import (
	"fmt"
	"regexp"
	"strings"
)

// This is a deliberately small event-count subset, not a general NRQL parser.
// Time bounds, account routing and the output alias belong to the adapter.
const nrAttribute = `[a-zA-Z_][a-zA-Z0-9_.]*`
const nrLiteral = `('[a-zA-Z0-9 _.:/@-]{0,200}'|true|false|-?[0-9]{1,18}(\.[0-9]{1,12})?)`
const nrPredicate = nrAttribute + `\s*(=|!=|<=|>=|<|>)\s*` + nrLiteral

var nrCount = regexp.MustCompile(`(?i)^SELECT\s+count\(\*\)\s+FROM\s+([a-zA-Z_][a-zA-Z0-9_]*)(\s+WHERE\s+` + nrPredicate + `(\s+AND\s+` + nrPredicate + `)*)?$`)

var nrAnd = regexp.MustCompile(`(?i)\s+AND\s+`)

type EventCountQuery struct{ EventType, Where, NRQL string }

func NewRelicCount(query string, step int64) (EventCountQuery, error) {
	if len(query) > 16000 || strings.TrimSpace(query) != query || strings.ContainsAny(query, "\r\n\x00") || step < 1 || step > 3600 {
		return EventCountQuery{}, fmt.Errorf("NRQL count query requires bounded text and stepSeconds from 1 to 3600")
	}
	m := nrCount.FindStringSubmatch(query)
	if m == nil || len(nrAnd.FindAllStringIndex(query, -1)) > 19 {
		return EventCountQuery{}, fmt.Errorf("NRQL requires SELECT count(*) FROM EventType with optional AND-connected attribute comparisons; omit time, grouping, aliases and functions")
	}
	if strings.EqualFold(m[1], "Metric") {
		return EventCountQuery{}, fmt.Errorf("NRQL event counts cannot count Metric rows; use an event population")
	}
	where := strings.TrimSpace(m[2])
	if where != "" {
		where = strings.TrimSpace(where[len("WHERE"):])
	}
	return EventCountQuery{EventType: m[1], Where: where, NRQL: "SELECT count(*) AS nines_count FROM " + m[1] + m[2]}, nil
}
