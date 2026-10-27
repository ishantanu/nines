package v1alpha1

import (
	"encoding/json"
	"time"
)

const BindingKind = "MeasurementBinding"
const DependencyMapKind = "ServiceDependencyMap"

// MeasurementSpec is owned and versioned by its SLO. It has no independent
// resource name, revision or lifecycle.
type MeasurementSpec struct {
	Type                  string            `json:"type" yaml:"type"`
	Metric                string            `json:"metric" yaml:"metric"`
	Labels                map[string]string `json:"labels" yaml:"labels"`
	SuccessLabels         map[string]string `json:"successLabels,omitempty" yaml:"successLabels,omitempty"`
	CountFrom             string            `json:"countFrom,omitempty" yaml:"countFrom,omitempty"`
	ThresholdSeconds      *float64          `json:"thresholdSeconds,omitempty" yaml:"thresholdSeconds,omitempty"`
	ScrapeIntervalSeconds int64             `json:"scrapeIntervalSeconds" yaml:"scrapeIntervalSeconds"`
	ObservationPoint      string            `json:"observationPoint" yaml:"observationPoint"`
}
type CompiledMeasurement struct {
	APIVersion        string                `json:"apiVersion"`
	Tenant            string                `json:"tenant"`
	CompilerVersion   string                `json:"compilerVersion"`
	MeasurementDigest string                `json:"measurementDigest"`
	Evidence          TelemetryDiscovery    `json:"evidence"`
	Manifest          ServiceLevelObjective `json:"manifest"`
	YAML              string                `json:"yaml"`
	Digest            string                `json:"digest"`
	Warnings          []string              `json:"warnings"`
}

type MeasurementBinding struct {
	APIVersion string      `json:"apiVersion" yaml:"apiVersion"`
	Kind       string      `json:"kind" yaml:"kind"`
	Metadata   Metadata    `json:"metadata" yaml:"metadata"`
	Spec       BindingSpec `json:"spec" yaml:"spec"`
}
type BindingSpec struct {
	ScrapeIntervalSeconds int64             `json:"scrapeIntervalSeconds,omitempty" yaml:"scrapeIntervalSeconds,omitempty"`
	CountFrom             string            `json:"countFrom,omitempty" yaml:"countFrom,omitempty"`
	DisplayName           string            `json:"displayName" yaml:"displayName"`
	Service               string            `json:"service" yaml:"service"`
	Source                string            `json:"source" yaml:"source"`
	Measurement           string            `json:"measurement" yaml:"measurement"` // availability or latency
	Metric                string            `json:"metric" yaml:"metric"`
	Labels                map[string]string `json:"labels" yaml:"labels"`
	SuccessLabels         map[string]string `json:"successLabels,omitempty" yaml:"successLabels,omitempty"`
	ThresholdSeconds      *float64          `json:"thresholdSeconds,omitempty" yaml:"thresholdSeconds,omitempty"`
	StepSeconds           int64             `json:"stepSeconds" yaml:"stepSeconds"`
	ObservationPoint      string            `json:"observationPoint" yaml:"observationPoint"`
}
type BindingReference struct {
	Name     string `json:"name" yaml:"name"`
	Revision int64  `json:"revision" yaml:"revision"`
	Digest   string `json:"digest" yaml:"digest"`
}
type ServiceDependencyMap struct {
	APIVersion string            `json:"apiVersion" yaml:"apiVersion"`
	Kind       string            `json:"kind" yaml:"kind"`
	Metadata   Metadata          `json:"metadata" yaml:"metadata"`
	Spec       DependencyMapSpec `json:"spec" yaml:"spec"`
}
type DependencyMapSpec struct {
	DisplayName  string               `json:"displayName" yaml:"displayName"`
	Services     []MapService         `json:"services" yaml:"services"`
	Dependencies []DeclaredDependency `json:"dependencies" yaml:"dependencies"`
	Observed     *GraphObservation    `json:"observed,omitempty" yaml:"observed,omitempty"`
}
type GraphPosition struct {
	X int `json:"x" yaml:"x"`
	Y int `json:"y" yaml:"y"`
}
type MapService struct {
	Position    *GraphPosition `json:"position,omitempty" yaml:"position,omitempty"`
	Name        string         `json:"name" yaml:"name"`
	DisplayName string         `json:"displayName" yaml:"displayName"`
	SLORefs     []string       `json:"sloRefs" yaml:"sloRefs"`
}
type DeclaredDependency struct {
	Origin      string `json:"origin,omitempty" yaml:"origin,omitempty"`
	Caller      string `json:"caller" yaml:"caller"`
	Callee      string `json:"callee" yaml:"callee"`
	Critical    bool   `json:"critical" yaml:"critical"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}
type GraphObservation struct {
	ScrapeIntervalSeconds int64             `json:"scrapeIntervalSeconds,omitempty" yaml:"scrapeIntervalSeconds,omitempty"`
	MaxSampleAgeSeconds   int64             `json:"maxSampleAgeSeconds,omitempty" yaml:"maxSampleAgeSeconds,omitempty"`
	Source                string            `json:"source" yaml:"source"`
	Labels                map[string]string `json:"labels" yaml:"labels"`
	WindowSeconds         int64             `json:"windowSeconds" yaml:"windowSeconds"`
}

func (b MeasurementBinding) ResourceKind() string   { return b.Kind }
func (g ServiceDependencyMap) ResourceKind() string { return g.Kind }

type TelemetryMetadata struct {
	Type string `json:"type"`
	Help string `json:"help"`
	Unit string `json:"unit"`
}
type TelemetryDiscovery struct {
	APIVersion  string              `json:"apiVersion"`
	Tenant      string              `json:"tenant"`
	Source      string              `json:"source"`
	Metric      string              `json:"metric"`
	From        time.Time           `json:"from"`
	To          time.Time           `json:"to"`
	Metadata    []TelemetryMetadata `json:"metadata"`
	Series      []map[string]string `json:"series"`
	Truncated   bool                `json:"truncated"`
	Fingerprint string              `json:"fingerprint"`
	Warnings    []string            `json:"warnings"`
}
type MetricNames struct {
	APIVersion string    `json:"apiVersion"`
	Tenant     string    `json:"tenant"`
	Source     string    `json:"source"`
	ObservedAt time.Time `json:"observedAt"`
	Names      []string  `json:"names"`
	Truncated  bool      `json:"truncated"`
}
type CompileBindingInput struct {
	ExpectedRevision int64            `json:"expectedRevision"`
	Name             string           `json:"name"`
	DisplayName      string           `json:"displayName"`
	Target           float64          `json:"target"`
	WindowSeconds    int64            `json:"windowSeconds"`
	Policies         []BurnRatePolicy `json:"policies,omitempty" yaml:"policies,omitempty"`
	Policy           *BurnRatePolicy  `json:"policy,omitempty"`
}
type CompiledBinding struct {
	MeasurementDigest string                `json:"measurementDigest,omitempty"`
	Definition        MeasurementBinding    `json:"definition"`
	APIVersion        string                `json:"apiVersion"`
	Tenant            string                `json:"tenant"`
	CompilerVersion   string                `json:"compilerVersion"`
	Binding           BindingReference      `json:"binding"`
	Evidence          TelemetryDiscovery    `json:"evidence"`
	Manifest          ServiceLevelObjective `json:"manifest"`
	YAML              string                `json:"yaml"`
	Digest            string                `json:"digest"`
	Warnings          []string              `json:"warnings"`
}
type TopologyResource struct {
	Kind        string    `json:"kind"`
	ID          string    `json:"id"`
	DisplayName string    `json:"displayName"`
	Revision    int64     `json:"revision"`
	Digest      string    `json:"digest"`
	AppliedAt   time.Time `json:"appliedAt"`
}
type TopologyCatalog struct {
	APIVersion string             `json:"apiVersion"`
	Tenant     string             `json:"tenant"`
	Items      []TopologyResource `json:"items"`
	NextCursor string             `json:"nextCursor,omitempty"`
}
type MapObjective struct {
	ID              string   `json:"id"`
	Revision        int64    `json:"revision"`
	DisplayName     string   `json:"displayName"`
	Health          string   `json:"health"`
	Reliability     *float64 `json:"reliability"`
	Target          float64  `json:"target"`
	DataStatus      string   `json:"dataStatus"`
	Freshness       string   `json:"freshness"`
	CollectionState string   `json:"collectionState"`
}
type MapJourneyStep struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type MapJourney struct {
	Steps       []MapJourneyStep `json:"steps,omitempty"`
	ID          string           `json:"id"`
	DisplayName string           `json:"displayName"`
	Revision    int64            `json:"revision"`
}
type DependencyNode struct {
	Position              *GraphPosition `json:"position,omitempty"`
	PotentialImpact       []MapJourney   `json:"potentialImpact"`
	ImpactTruncated       bool           `json:"impactTruncated"`
	Name                  string         `json:"name"`
	DisplayName           string         `json:"displayName"`
	Declared              bool           `json:"declared"`
	Health                string         `json:"health"`
	Objectives            []MapObjective `json:"objectives"`
	Journeys              []MapJourney   `json:"journeys"`
	JourneyLinksTruncated bool           `json:"journeyLinksTruncated"`
}
type DependencyEdge struct {
	Origin         string             `json:"origin,omitempty"`
	Alignment      string             `json:"alignment,omitempty"`
	Evidence       *GraphEdgeEvidence `json:"evidence,omitempty"`
	Caller         string             `json:"caller"`
	Callee         string             `json:"callee"`
	Provenance     string             `json:"provenance"`
	Critical       bool               `json:"critical"`
	Description    string             `json:"description,omitempty"`
	ConnectionType string             `json:"connectionType,omitempty"`
	Requests       *float64           `json:"requests,omitempty"`
}
type ObservedEdge struct {
	Caller, Callee, ConnectionType string
	Requests                       float64
	Evidence                       *GraphSampleEvidence
}
type DependencyView struct {
	SourceDigest            string           `json:"sourceDigest,omitempty"`
	InstrumentationCoverage string           `json:"instrumentationCoverage,omitempty"`
	DeclaredEdges           int              `json:"declaredEdges"`
	MatchedDeclaredEdges    int              `json:"matchedDeclaredEdges"`
	APIVersion              string           `json:"apiVersion"`
	Tenant                  string           `json:"tenant"`
	ID                      string           `json:"id"`
	Revision                int64            `json:"revision"`
	ObservedAt              time.Time        `json:"observedAt"`
	Nodes                   []DependencyNode `json:"nodes"`
	Edges                   []DependencyEdge `json:"edges"`
	ObservationState        string           `json:"observationState"`
	Source                  string           `json:"source,omitempty"`
	From                    *time.Time       `json:"from,omitempty"`
	To                      *time.Time       `json:"to,omitempty"`
	Truncated               bool             `json:"truncated"`
	Warnings                []string         `json:"warnings"`
}

// BindingHealth verifies the measurement contract, not observed reliability.
type BindingHealth struct {
	MeasurementDigest string           `json:"measurementDigest,omitempty"`
	SourceDigest      string           `json:"sourceDigest"`
	SLORevision       int64            `json:"sloRevision,omitempty"`
	APIVersion        string           `json:"apiVersion"`
	Tenant            string           `json:"tenant"`
	Binding           BindingReference `json:"binding"`
	State             string           `json:"state"`
	Code              string           `json:"code"`
	Message           string           `json:"message"`
	CheckedAt         time.Time        `json:"checkedAt"`
	Fingerprint       string           `json:"fingerprint,omitempty"`
	EvaluationBlocked bool             `json:"evaluationBlocked"`
}

// MarshalJSON keeps the legacy response unchanged and omits inapplicable shared
// binding fields for a measurement owned by an SLO.
func (c CompiledBinding) MarshalJSON() ([]byte, error) {
	type plain CompiledBinding
	if c.MeasurementDigest == "" {
		return json.Marshal(plain(c))
	}
	return json.Marshal(struct {
		*plain
		Definition *MeasurementBinding `json:"definition,omitempty"`
		Binding    *BindingReference   `json:"binding,omitempty"`
	}{plain: (*plain)(&c)})
}
func (c BindingHealth) MarshalJSON() ([]byte, error) {
	type plain BindingHealth
	if c.MeasurementDigest == "" {
		return json.Marshal(plain(c))
	}
	return json.Marshal(struct {
		*plain
		Binding *BindingReference `json:"binding,omitempty"`
	}{plain: (*plain)(&c)})
}

// GraphSampleEvidence describes only the observed counter series, not all
// application traffic, trace sampling or instrumentation coverage.
type GraphSampleEvidence struct {
	LastSampleAt      *time.Time `json:"lastSampleAt,omitempty"`
	Samples           float64    `json:"samples"`
	Series            int64      `json:"series"`
	SampleContinuity  *float64   `json:"sampleContinuity,omitempty"`
	ResolutionSeconds int64      `json:"resolutionSeconds"`
}
type GraphEdgeEvidence struct {
	Source                  string               `json:"source"`
	SourceDigest            string               `json:"sourceDigest"`
	Metric                  string               `json:"metric"`
	ReadAt                  time.Time            `json:"readAt"`
	From                    time.Time            `json:"from"`
	To                      time.Time            `json:"to"`
	Freshness               string               `json:"freshness"`
	MaxSampleAgeSeconds     int64                `json:"maxSampleAgeSeconds"`
	SampleAgeSeconds        *int64               `json:"sampleAgeSeconds,omitempty"`
	Samples                 *GraphSampleEvidence `json:"samples,omitempty"`
	InstrumentationCoverage string               `json:"instrumentationCoverage"`
}

func (o GraphObservation) EvidenceTiming() (interval, age int64) {
	interval = o.ScrapeIntervalSeconds
	if interval == 0 {
		interval = 15
	}
	age = o.MaxSampleAgeSeconds
	if age == 0 {
		age = max(30, 3*interval)
	}
	return
}
