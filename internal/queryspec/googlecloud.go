package queryspec

import (
	"fmt"
	"regexp"
	"strings"
)

// GoogleCloudFilter is an exact-match subset of Monitoring filters. Project,
// metrics-scope selection, functions and boolean alternatives belong to no SLO.
var cloudTerm = regexp.MustCompile(`^(metric\.type|resource\.type|metric\.labels\.[a-zA-Z_][a-zA-Z0-9_]*|resource\.labels\.[a-zA-Z_][a-zA-Z0-9_]*)\s*=\s*"([a-zA-Z0-9_./:@ -]{1,200})"$`)

func GoogleCloudFilter(query string, step int64) (map[string]string, error) {
	if len(query) > 8000 || strings.TrimSpace(query) != query || step < 60 || step > 3600 || strings.ContainsAny(query, "\r\n\x00") {
		return nil, fmt.Errorf("google cloud count filters require bounded text and stepSeconds 60–3600")
	}
	// Split outside quotes: label values may legitimately contain the word AND.
	parts := []string{}
	start := 0
	quoted := false
	for i := 0; i < len(query); i++ {
		if query[i] == '"' {
			quoted = !quoted
		}
		if !quoted && strings.HasPrefix(query[i:], " AND ") {
			parts = append(parts, query[start:i])
			i += 4
			start = i + 1
		}
	}
	parts = append(parts, query[start:])
	if len(parts) > 20 {
		return nil, fmt.Errorf("too many Google Cloud filter predicates")
	}
	values := map[string]string{}
	for _, part := range parts {
		m := cloudTerm.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil {
			return nil, fmt.Errorf("google cloud filters require exact metric.type, resource.type and optional label equalities joined by AND")
		}
		if _, ok := values[m[1]]; ok {
			return nil, fmt.Errorf("duplicate Google Cloud filter predicate")
		}
		values[m[1]] = m[2]
	}
	if values["metric.type"] == "" || values["resource.type"] == "" {
		return nil, fmt.Errorf("google cloud filters require metric.type and resource.type")
	}
	return values, nil
}
