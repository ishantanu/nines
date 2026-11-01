package queryspec

import (
	"fmt"
	"regexp"
	"sync"

	"github.com/ishantanu/nines/internal/promqlvalidate"
)

type Validator func(string, int64) error

var languageMu sync.RWMutex
var languages = map[string]Validator{
	"promql":      func(query string, _ int64) error { _, err := promqlvalidate.InstantVector(query); return err },
	"datadog":     func(query string, step int64) error { _, err := DatadogCount(query, step); return err },
	"nrql":        func(query string, step int64) error { _, err := NewRelicCount(query, step); return err },
	"metricsql":   MetricsQLCount,
	"googlecloud": func(query string, step int64) error { _, err := GoogleCloudFilter(query, step); return err },
}

// RegisterLanguage is a host startup extension point. Validators must be pure,
// deterministic and bounded; manifests cannot install or replace validators.
func RegisterLanguage(name string, validate Validator) error {
	if !regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`).MatchString(name) || validate == nil {
		return fmt.Errorf("query language requires a valid name and validator")
	}
	languageMu.Lock()
	defer languageMu.Unlock()
	if _, ok := languages[name]; ok {
		return fmt.Errorf("query language already registered")
	}
	languages[name] = validate
	return nil
}
func Supports(name string) bool {
	languageMu.RLock()
	defer languageMu.RUnlock()
	_, ok := languages[name]
	return ok
}
func Validate(name, query string, step int64) error {
	if name == "" {
		name = "promql"
	}
	languageMu.RLock()
	validate, ok := languages[name]
	languageMu.RUnlock()
	if !ok {
		return fmt.Errorf("query language is not installed")
	}
	return validate(query, step)
}
