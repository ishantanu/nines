package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"

	v1alpha1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/api/v1beta1"

	"gopkg.in/yaml.v3"
)

// APIVersion is the shared declarative schema version for all resource kinds.
const APIVersion = v1alpha1.APIVersion
const SLOKind = v1alpha1.SLOKind
const JourneyKind = v1alpha1.JourneyKind

type ResourceMetadata = v1alpha1.Metadata
type SLODefinition v1alpha1.ServiceLevelObjective
type SLOSpec = v1alpha1.SLOSpec

func DefinitionFromSLO(s SLO) SLODefinition {
	window := s.ProviderWindow
	if s.Measurement != nil {
		window = nil
	}
	return SLODefinition{APIVersion: APIVersion, Kind: SLOKind, Metadata: ResourceMetadata{Name: s.ID, Project: s.Project}, Spec: SLOSpec{Customers: s.Customers, BadInterval: s.BadInterval, Labels: s.Labels, QueryLanguage: s.QueryLanguage, ErrorBudgetPolicy: s.ErrorBudgetPolicy, Quality: s.Quality, Lifecycle: s.Lifecycle, Measurement: s.Measurement, Intent: s.Intent, ProviderWindow: window, MeasurementBinding: s.MeasurementBinding, Source: s.Source, DisplayName: s.Name, Service: s.Service, Target: s.Target, WindowSeconds: s.WindowSeconds, StepSeconds: s.StepSeconds, GoodQuery: s.GoodQuery, TotalQuery: s.TotalQuery, Policies: s.Policies, Policy: s.Policy}}
}
func (d SLODefinition) SLO() SLO {
	s := SLO{Customers: d.Spec.Customers, BadInterval: d.Spec.BadInterval, Labels: d.Spec.Labels, QueryLanguage: d.Spec.QueryLanguage, ErrorBudgetPolicy: d.Spec.ErrorBudgetPolicy, Quality: d.Spec.Quality, Lifecycle: d.Spec.Lifecycle, Measurement: d.Spec.Measurement, Intent: d.Spec.Intent, ProviderWindow: d.Spec.ProviderWindow, MeasurementBinding: d.Spec.MeasurementBinding, Source: d.Spec.Source, ID: d.Metadata.Name, Project: d.Metadata.Project, Name: d.Spec.DisplayName, Service: d.Spec.Service, Target: d.Spec.Target, WindowSeconds: d.Spec.WindowSeconds, StepSeconds: d.Spec.StepSeconds, GoodQuery: d.Spec.GoodQuery, TotalQuery: d.Spec.TotalQuery, Policies: d.Spec.Policies, Policy: d.Spec.Policy}
	if s.Measurement != nil && s.ProviderWindow == nil {
		s.ProviderWindow = MeasurementWindow(*s.Measurement)
	}
	return s
}
func (d SLODefinition) Validate() error {
	if d.Spec.Measurement != nil && (d.Spec.ProviderWindow != nil || d.Spec.MeasurementBinding != nil || d.Spec.GoodQuery != "" || d.Spec.TotalQuery != "") {
		return Invalid("spec.measurement owns its generated queries; omit providerWindow, measurementBinding, goodQuery and totalQuery")
	}
	if d.APIVersion != APIVersion || d.Kind != SLOKind {
		return Invalid("expected apiVersion %s and kind %s", APIVersion, SLOKind)
	}
	return d.SLO().Validate()
}
func ParseSLODefinition(body []byte) (SLODefinition, error) {
	if isV1beta1(body) {
		def, err := ParseDefinition(body)
		if def.SLO == nil {
			return SLODefinition{}, firstError(err, Invalid("expected kind %s", SLOKind))
		}
		return *def.SLO, err
	}
	var d SLODefinition
	if err := decodeDefinition(body, &d); err != nil {
		return d, err
	}
	return d, d.Validate()
}
func decodeDefinition(body []byte, out any) error {
	if len(body) == 0 || len(body) > 1<<20 {
		return Invalid("manifest must contain 1–1048576 bytes")
	}
	d := yaml.NewDecoder(bytes.NewReader(body))
	d.KnownFields(true)
	if err := d.Decode(out); err != nil {
		return Invalid("invalid resource manifest: %v", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return Invalid("provide exactly one manifest document")
	}
	return nil
}

// DefinitionDigest computes a content digest for definition versioning and change detection.
func DefinitionDigest(manifest any) string {
	b, _ := json.Marshal(manifest)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Definition dispatches supported kinds through the same strict YAML decoder.
// JSON is also valid YAML syntax; canonical exports and authoring use YAML.
type Definition struct {
	SLO           *SLODefinition
	Journey       *Journey
	Binding       *MeasurementBinding
	DependencyMap *ServiceDependencyMap
	// SourceAPIVersion is the apiVersion the definition was written in.
	SourceAPIVersion string
	// Notes describe defaults applied while converting a v1beta1 definition.
	Notes []string
	// BetaJourney keeps a v1beta1 journey so the server can derive omitted
	// link dimensions from the linked SLOs, which offline parsing cannot see.
	BetaJourney *v1beta1.CriticalUserJourney
}

func (d Definition) Manifest() any {
	if d.Binding != nil {
		return *d.Binding
	}
	if d.DependencyMap != nil {
		return *d.DependencyMap
	}
	if d.SLO != nil {
		return *d.SLO
	}
	if d.Journey != nil {
		return *d.Journey
	}
	return nil
}
func (d Definition) Kind() string {
	if d.Binding != nil {
		return v1alpha1.BindingKind
	}
	if d.DependencyMap != nil {
		return v1alpha1.DependencyMapKind
	}
	if d.SLO != nil {
		return SLOKind
	}
	return JourneyKind
}
func (d Definition) Metadata() ResourceMetadata {
	if d.Binding != nil {
		return d.Binding.Metadata
	}
	if d.DependencyMap != nil {
		return d.DependencyMap.Metadata
	}
	if d.SLO != nil {
		return d.SLO.Metadata
	}
	return d.Journey.Metadata
}
func ParseDefinition(body []byte) (Definition, error) {
	var header struct {
		APIVersion string           `yaml:"apiVersion"`
		Kind       string           `yaml:"kind"`
		Metadata   ResourceMetadata `yaml:"metadata"`
		Spec       yaml.Node        `yaml:"spec"`
	}
	if err := decodeDefinition(body, &header); err != nil {
		return Definition{}, err
	}
	if header.APIVersion == v1beta1.APIVersion {
		return parseV1beta1(body, header.Kind)
	}
	if header.APIVersion != APIVersion {
		return Definition{}, Invalid("unsupported apiVersion %q; expected %s or %s", header.APIVersion, v1beta1.APIVersion, APIVersion)
	}
	d, err := parseV1alpha1(body, header.Kind)
	d.SourceAPIVersion = APIVersion
	d.Notes = append(d.Notes, V1alpha1Deprecation)
	return d, err
}

// V1alpha1Deprecation is reported whenever a v1alpha1 definition is read.
const V1alpha1Deprecation = "nines.dev/v1alpha1 is deprecated; write nines.dev/v1beta1 (nines export prints any stored definition in it)"

func parseV1alpha1(body []byte, kind string) (Definition, error) {
	switch kind {
	case v1alpha1.BindingKind:
		var b MeasurementBinding
		if err := decodeDefinition(body, &b); err != nil {
			return Definition{}, err
		}
		return Definition{Binding: &b}, b.Validate()
	case v1alpha1.DependencyMapKind:
		var g ServiceDependencyMap
		if err := decodeDefinition(body, &g); err != nil {
			return Definition{}, err
		}
		return Definition{DependencyMap: &g}, g.Validate()
	case SLOKind:
		d, err := ParseSLODefinition(body)
		return Definition{SLO: &d}, err
	case JourneyKind:
		j, err := ParseJourney(body)
		return Definition{Journey: &j}, err
	default:
		return Definition{}, Invalid("unsupported kind %q", kind)
	}
}

// parseV1beta1 decodes a v1beta1 definition, converts it to the stored form
// and validates it exactly as a v1alpha1 definition is validated.
func parseV1beta1(body []byte, kind string) (Definition, error) {
	out := Definition{SourceAPIVersion: v1beta1.APIVersion}
	switch kind {
	case v1beta1.SLOKind:
		var in v1beta1.ServiceLevelObjective
		if err := decodeDefinition(body, &in); err != nil {
			return out, err
		}
		hub, notes, err := in.ToHub()
		if err != nil {
			return out, Invalid("%v", err)
		}
		d := SLODefinition(hub)
		out.SLO, out.Notes = &d, notes
		return out, d.Validate()
	case v1beta1.JourneyKind:
		var in v1beta1.CriticalUserJourney
		if err := decodeDefinition(body, &in); err != nil {
			return out, err
		}
		hub, notes, err := in.ToHub(nil)
		if err != nil {
			return out, Invalid("%v", err)
		}
		j := Journey(hub)
		out.Journey, out.Notes, out.BetaJourney = &j, notes, &in
		return out, j.Validate()
	case v1beta1.BindingKind:
		var in v1beta1.MeasurementBinding
		if err := decodeDefinition(body, &in); err != nil {
			return out, err
		}
		hub, err := in.ToHub()
		if err != nil {
			return out, Invalid("%v", err)
		}
		b := MeasurementBinding(hub)
		out.Binding = &b
		return out, b.Validate()
	case v1beta1.DependencyMapKind:
		var in v1beta1.ServiceDependencyMap
		if err := decodeDefinition(body, &in); err != nil {
			return out, err
		}
		hub, err := in.ToHub()
		if err != nil {
			return out, Invalid("%v", err)
		}
		g := ServiceDependencyMap(hub)
		out.DependencyMap = &g
		return out, g.Validate()
	default:
		return out, Invalid("unsupported kind %q", kind)
	}
}

// isV1beta1 reports whether a manifest declares nines.dev/v1beta1.
func isV1beta1(body []byte) bool {
	var header struct {
		APIVersion string `yaml:"apiVersion"`
	}
	return yaml.Unmarshal(body, &header) == nil && header.APIVersion == v1beta1.APIVersion
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
