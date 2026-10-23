// Package v1alpha1 contains public YAML resource types shared by the server and clients.
package v1alpha1

const APIVersion = "nines.dev/v1alpha1"
const SLOKind = "ServiceLevelObjective"
const JourneyKind = "CriticalUserJourney"

type Metadata struct {
	Name    string `json:"name" yaml:"name"`
	Project string `json:"project" yaml:"project"`
}
type ServiceLevelObjective struct {
	APIVersion string   `json:"apiVersion" yaml:"apiVersion"`
	Kind       string   `json:"kind" yaml:"kind"`
	Metadata   Metadata `json:"metadata" yaml:"metadata"`
	Spec       SLOSpec  `json:"spec" yaml:"spec"`
}

// ProviderWindow evaluates counter increases over each complete requested window.
// Selectors identify cumulative counters; aggregation happens after reset handling.
type ProviderWindow struct {
	GoodSelector          string `json:"goodSelector" yaml:"goodSelector"`
	TotalSelector         string `json:"totalSelector" yaml:"totalSelector"`
	ScrapeIntervalSeconds int64  `json:"scrapeIntervalSeconds" yaml:"scrapeIntervalSeconds"`
}
type SLOSpec struct {
	Measurement        *MeasurementSpec  `json:"measurement,omitempty" yaml:"measurement,omitempty"`
	Intent             *IntentSpec       `json:"intent,omitempty" yaml:"intent,omitempty"`
	ProviderWindow     *ProviderWindow   `json:"providerWindow,omitempty" yaml:"providerWindow,omitempty"`
	MeasurementBinding *BindingReference `json:"measurementBinding,omitempty" yaml:"measurementBinding,omitempty"`
	Source             string            `json:"source,omitempty" yaml:"source,omitempty"`
	DisplayName        string            `json:"displayName" yaml:"displayName"`
	Service            string            `json:"service" yaml:"service"`
	Target             float64           `json:"target" yaml:"target"`
	WindowSeconds      int64             `json:"windowSeconds" yaml:"windowSeconds"`
	StepSeconds        int64             `json:"stepSeconds" yaml:"stepSeconds"`
	GoodQuery          string            `json:"goodQuery,omitempty" yaml:"goodQuery,omitempty"`
	TotalQuery         string            `json:"totalQuery,omitempty" yaml:"totalQuery,omitempty"`
	Policies           []BurnRatePolicy  `json:"policies,omitempty" yaml:"policies,omitempty"`
	Policy             *BurnRatePolicy   `json:"policy,omitempty" yaml:"policy,omitempty"`
}

type BurnRatePolicy struct {
	Name               string  `json:"name,omitempty" yaml:"name,omitempty"`
	Severity           string  `json:"severity,omitempty" yaml:"severity,omitempty"`
	ShortWindowSeconds int64   `json:"shortWindowSeconds" yaml:"shortWindowSeconds"`
	LongWindowSeconds  int64   `json:"longWindowSeconds" yaml:"longWindowSeconds"`
	BurnRate           float64 `json:"burnRate" yaml:"burnRate"`
	ForSeconds         int64   `json:"forSeconds" yaml:"forSeconds"`
	MinCoverage        float64 `json:"minCoverage" yaml:"minCoverage"`
}

type CriticalUserJourney struct {
	APIVersion string      `json:"apiVersion" yaml:"apiVersion"`
	Kind       string      `json:"kind" yaml:"kind"`
	Metadata   Metadata    `json:"metadata" yaml:"metadata"`
	Spec       JourneySpec `json:"spec" yaml:"spec"`
}

type JourneySpec struct {
	DisplayName string           `json:"displayName" yaml:"displayName"`
	Description string           `json:"description" yaml:"description"`
	Owner       string           `json:"owner" yaml:"owner"`
	Criticality string           `json:"criticality" yaml:"criticality"`
	SLOs        []JourneySLO     `json:"slos" yaml:"slos"`
	Steps       []JourneyStep    `json:"steps" yaml:"steps"`
	Agentic     *AgenticContract `json:"agentic,omitempty" yaml:"agentic,omitempty"`
}
type JourneySLO struct {
	ID        string `json:"id" yaml:"id"`
	Dimension string `json:"dimension" yaml:"dimension"`
	Required  bool   `json:"required" yaml:"required"`
}
type JourneyStep struct {
	ID        string   `json:"id" yaml:"id"`
	Name      string   `json:"name" yaml:"name"`
	Actor     string   `json:"actor" yaml:"actor"`
	DependsOn []string `json:"dependsOn,omitempty" yaml:"dependsOn,omitempty"`
	SLORefs   []string `json:"sloRefs" yaml:"sloRefs"`
}
type AgenticContract struct {
	SuccessDefinition  string   `json:"successDefinition" yaml:"successDefinition"`
	MaxDurationSeconds int64    `json:"maxDurationSeconds,omitempty" yaml:"maxDurationSeconds,omitempty"`
	MaxCostUSD         *float64 `json:"maxCostUSD,omitempty" yaml:"maxCostUSD,omitempty"`
	MaxSteps           int      `json:"maxSteps,omitempty" yaml:"maxSteps,omitempty"`
	ApprovalRequired   bool     `json:"approvalRequired" yaml:"approvalRequired"`
}

// Resource is a versioned declarative definition accepted by plan/apply.
type Resource interface{ ResourceKind() string }

func (s ServiceLevelObjective) ResourceKind() string { return s.Kind }
func (j CriticalUserJourney) ResourceKind() string   { return j.Kind }
func NewSLO(name, project string, spec SLOSpec) ServiceLevelObjective {
	return ServiceLevelObjective{APIVersion: APIVersion, Kind: SLOKind, Metadata: Metadata{Name: name, Project: project}, Spec: spec}
}
func NewJourney(name, project string, spec JourneySpec) CriticalUserJourney {
	return CriticalUserJourney{APIVersion: APIVersion, Kind: JourneyKind, Metadata: Metadata{Name: name, Project: project}, Spec: spec}
}
