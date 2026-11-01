package v1alpha1

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

// BurnWindowPattern is shared with the Kubernetes admission schema. Burn
// windows use whole-second Go-style durations, with explicit h, m or s units.
const BurnWindowPattern = `^([0-9]+[hms])+$`

var burnWindowPattern = regexp.MustCompile(BurnWindowPattern)

type burnPolicyWire struct {
	Name               string  `json:"name,omitempty" yaml:"name,omitempty"`
	Severity           string  `json:"severity,omitempty" yaml:"severity,omitempty"`
	ShortWindow        *string `json:"shortWindow,omitempty" yaml:"shortWindow,omitempty"`
	LongWindow         *string `json:"longWindow,omitempty" yaml:"longWindow,omitempty"`
	ShortWindowSeconds *int64  `json:"shortWindowSeconds,omitempty" yaml:"shortWindowSeconds,omitempty"`
	LongWindowSeconds  *int64  `json:"longWindowSeconds,omitempty" yaml:"longWindowSeconds,omitempty"`
	BurnRate           float64 `json:"burnRate" yaml:"burnRate"`
	ForSeconds         int64   `json:"forSeconds" yaml:"forSeconds"`
	MinCoverage        float64 `json:"minCoverage" yaml:"minCoverage"`
	MinEvents          float64 `json:"minEvents,omitempty" yaml:"minEvents,omitempty"`
}

func burnWindow(text *string, seconds *int64, name string) (int64, error) {
	if text != nil && seconds != nil {
		return 0, fmt.Errorf("use either %s or %sSeconds, not both", name, name)
	}
	if text == nil {
		if seconds != nil {
			return *seconds, nil
		}
		return 0, nil
	}
	if len(*text) > 64 || !burnWindowPattern.MatchString(*text) {
		return 0, fmt.Errorf("%s requires a duration such as 300s, 5m or 1h30m", name)
	}
	d, err := time.ParseDuration(*text)
	if err != nil || d <= 0 || d%time.Second != 0 {
		return 0, fmt.Errorf("%s requires a positive whole-second duration", name)
	}
	return int64(d / time.Second), nil
}
func (p *BurnRatePolicy) fromWire(w burnPolicyWire) error {
	short, err := burnWindow(w.ShortWindow, w.ShortWindowSeconds, "shortWindow")
	if err != nil {
		return err
	}
	long, err := burnWindow(w.LongWindow, w.LongWindowSeconds, "longWindow")
	if err != nil {
		return err
	}
	*p = BurnRatePolicy{Name: w.Name, Severity: w.Severity, ShortWindowSeconds: short, LongWindowSeconds: long, BurnRate: w.BurnRate, ForSeconds: w.ForSeconds, MinCoverage: w.MinCoverage, MinEvents: w.MinEvents}
	return nil
}
func (p *BurnRatePolicy) UnmarshalJSON(data []byte) error {
	// Keep numeric JSON responses/storage stable, while accepting duration-based
	// declarative input through API and Kubernetes clients as well as YAML.
	if !json.Valid(data) {
		return fmt.Errorf("invalid burn policy JSON")
	}
	var wire burnPolicyWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return err
	}
	if len(node.Content) != 1 {
		return fmt.Errorf("burn policy must be an object")
	}
	return p.UnmarshalYAML(node.Content[0])
}
func (p *BurnRatePolicy) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("burn policy must be an object")
	}
	allowed := map[string]bool{"name": true, "severity": true, "shortWindow": true, "longWindow": true, "shortWindowSeconds": true, "longWindowSeconds": true, "burnRate": true, "forSeconds": true, "minCoverage": true, "minEvents": true}
	seen := map[string]bool{}
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if !allowed[key] || seen[key] {
			return fmt.Errorf("unknown or duplicate burn policy field %q", key)
		}
		if node.Content[i+1].Tag == "!!null" {
			return fmt.Errorf("burn policy field %q cannot be null", key)
		}
		if (key == "shortWindow" || key == "longWindow") && node.Content[i+1].Tag != "!!str" {
			return fmt.Errorf("%s requires a duration string", key)
		}
		if (key == "shortWindowSeconds" || key == "longWindowSeconds" || key == "forSeconds") && node.Content[i+1].Tag != "!!int" {
			return fmt.Errorf("%s requires whole integer seconds", key)
		}
		seen[key] = true
	}
	var w burnPolicyWire
	if err := node.Decode(&w); err != nil {
		return err
	}
	return p.fromWire(w)
}
func formatBurnWindow(seconds int64) string {
	if seconds != 0 && seconds%3600 == 0 {
		return fmt.Sprintf("%dh", seconds/3600)
	}
	if seconds != 0 && seconds%60 == 0 {
		return fmt.Sprintf("%dm", seconds/60)
	}
	return fmt.Sprintf("%ds", seconds)
}
func (p BurnRatePolicy) MarshalYAML() (any, error) {
	short, long := formatBurnWindow(p.ShortWindowSeconds), formatBurnWindow(p.LongWindowSeconds)
	return burnPolicyWire{Name: p.Name, Severity: p.Severity, ShortWindow: &short, LongWindow: &long, BurnRate: p.BurnRate, ForSeconds: p.ForSeconds, MinCoverage: p.MinCoverage, MinEvents: p.MinEvents}, nil
}

// CanonicalDefinitionJSON uses the YAML representation of declarative resources
// for Kubernetes spec objects without changing runtime JSON or stored digests.
func CanonicalDefinitionJSON(value any) ([]byte, error) {
	data, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	var obj any
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err = decoder.Decode(&obj); err != nil {
		return nil, err
	}
	return json.Marshal(obj)
}
