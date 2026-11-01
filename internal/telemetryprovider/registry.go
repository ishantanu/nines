// Package telemetryprovider defines compiled provider extensions. Factories are
// explicitly registered by the host; tenant input never loads executable code.
package telemetryprovider

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/queryspec"
)

type Collector interface {
	ValidateSLO(model.SLO) error
	SourceFingerprint() string
	Collect(context.Context, model.SLO, time.Time, time.Time) ([]model.Point, error)
}
type Capabilities struct {
	NativeSLOResults    bool `json:"nativeSLOResults"`
	NativeSLOCatalog    bool `json:"nativeSLOCatalog"`
	NativeSLOPublishing bool `json:"nativeSLOPublishing"`
	IntervalCounts      bool `json:"intervalCounts"`
	WindowCounts        bool `json:"windowCounts"`
	Discovery           bool `json:"discovery"`
	Dependencies        bool `json:"dependencies"`
	Cohorts             bool `json:"cohorts"`
	RuleDeployment      bool `json:"ruleDeployment"`
}
type Descriptor struct {
	APIVersion     string       `json:"apiVersion"`
	Kind           string       `json:"kind"`
	QueryLanguages []string     `json:"queryLanguages"`
	Capabilities   Capabilities `json:"capabilities"`
}

// Configuration contains only server-authorized routing. Credentials must not be
// serialized, logged or selected by an SLO or model.
type Configuration struct {
	Options     map[string]string
	Endpoint    string
	Dataset     string
	Credentials map[string]string `json:"-" yaml:"-"`
}
type Factory func(Configuration) (Collector, error)
type registration struct {
	descriptor Descriptor
	factory    Factory
}
type Registry struct {
	mu      sync.RWMutex
	entries map[string]registration
}

func NewRegistry() *Registry { return &Registry{entries: map[string]registration{}} }
func (r *Registry) Register(d Descriptor, f Factory) error {
	if !regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`).MatchString(d.Kind) || f == nil || len(d.QueryLanguages) == 0 {
		return fmt.Errorf("provider registration requires a kind, query language and factory")
	}
	for _, language := range d.QueryLanguages {
		if !queryspec.Supports(language) {
			return fmt.Errorf("provider query language is not installed")
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.entries[d.Kind]; ok {
		return fmt.Errorf("provider already registered")
	}
	if d.APIVersion != "" && d.APIVersion != "nines.dev/provider/v1alpha1" {
		return fmt.Errorf("unsupported provider contract version")
	}
	d.APIVersion = "nines.dev/provider/v1alpha1"
	d.QueryLanguages = append([]string(nil), d.QueryLanguages...)
	r.entries[d.Kind] = registration{d, f}
	return nil
}
func (r *Registry) Open(kind string, c Configuration) (Collector, error) {
	r.mu.RLock()
	e, ok := r.entries[kind]
	r.mu.RUnlock()
	if !ok {
		return nil, model.Invalid("telemetry provider %q is not installed", kind)
	}
	c.Options = clone(c.Options)
	c.Credentials = clone(c.Credentials)
	collector, err := e.factory(c)
	if err == nil && collector == nil {
		return nil, fmt.Errorf("provider factory returned no collector")
	}
	return collector, err
}
func (r *Registry) Describe(kind string) (Descriptor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[kind]
	d := e.descriptor
	d.QueryLanguages = append([]string(nil), d.QueryLanguages...)
	return d, ok
}
func (r *Registry) List() []Descriptor {
	r.mu.RLock()
	names := make([]string, 0, len(r.entries))
	for name := range r.entries {
		names = append(names, name)
	}
	r.mu.RUnlock()
	sort.Strings(names)
	out := make([]Descriptor, 0, len(names))
	for _, name := range names {
		d, _ := r.Describe(name)
		out = append(out, d)
	}
	return out
}

func clone(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
