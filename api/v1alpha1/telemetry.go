package v1alpha1

import "time"

const BindingKind = "MeasurementBinding"
const DependencyMapKind = "ServiceDependencyMap"

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
type MapService struct {
	Name        string   `json:"name" yaml:"name"`
	DisplayName string   `json:"displayName" yaml:"displayName"`
	SLORefs     []string `json:"sloRefs" yaml:"sloRefs"`
}
type DeclaredDependency struct {
	Caller      string `json:"caller" yaml:"caller"`
	Callee      string `json:"callee" yaml:"callee"`
	Critical    bool   `json:"critical" yaml:"critical"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}
type GraphObservation struct {
	Source        string            `json:"source" yaml:"source"`
	Labels        map[string]string `json:"labels" yaml:"labels"`
	WindowSeconds int64             `json:"windowSeconds" yaml:"windowSeconds"`
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
	Definition      MeasurementBinding    `json:"definition"`
	APIVersion      string                `json:"apiVersion"`
	Tenant          string                `json:"tenant"`
	CompilerVersion string                `json:"compilerVersion"`
	Binding         BindingReference      `json:"binding"`
	Evidence        TelemetryDiscovery    `json:"evidence"`
	Manifest        ServiceLevelObjective `json:"manifest"`
	YAML            string                `json:"yaml"`
	Digest          string                `json:"digest"`
	Warnings        []string              `json:"warnings"`
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
type MapJourney struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Revision    int64  `json:"revision"`
}
type DependencyNode struct {
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
	Caller         string   `json:"caller"`
	Callee         string   `json:"callee"`
	Provenance     string   `json:"provenance"`
	Critical       bool     `json:"critical"`
	Description    string   `json:"description,omitempty"`
	ConnectionType string   `json:"connectionType,omitempty"`
	Requests       *float64 `json:"requests,omitempty"`
}
type ObservedEdge struct {
	Caller, Callee, ConnectionType string
	Requests                       float64
}
type DependencyView struct {
	APIVersion       string           `json:"apiVersion"`
	Tenant           string           `json:"tenant"`
	ID               string           `json:"id"`
	Revision         int64            `json:"revision"`
	ObservedAt       time.Time        `json:"observedAt"`
	Nodes            []DependencyNode `json:"nodes"`
	Edges            []DependencyEdge `json:"edges"`
	ObservationState string           `json:"observationState"`
	Source           string           `json:"source,omitempty"`
	From             *time.Time       `json:"from,omitempty"`
	To               *time.Time       `json:"to,omitempty"`
	Truncated        bool             `json:"truncated"`
	Warnings         []string         `json:"warnings"`
}

// BindingHealth verifies the measurement contract, not observed reliability.
type BindingHealth struct {
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
