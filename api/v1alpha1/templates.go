package v1alpha1

// SLOTemplate is a shipped starting point, not a persisted tenant resource.
// Digest identifies its complete content; revision is independent of SLO revisions.
type SLOTemplate struct {
	Extension      *ExtensionReference       `json:"extension,omitempty"`
	Inputs         map[string]ExtensionInput `json:"inputs,omitempty"`
	Evaluation     *ExtensionEvaluation      `json:"evaluation,omitempty"`
	ID             string                    `json:"id"`
	Revision       int                       `json:"revision"`
	Digest         string                    `json:"digest"`
	Category       string                    `json:"category"`
	SLI            string                    `json:"sli"`
	Title          string                    `json:"title"`
	Description    string                    `json:"description"`
	EligibleEvents string                    `json:"eligibleEvents"`
	GoodEvents     string                    `json:"goodEvents"`
	Requirements   []string                  `json:"requirements"`
	Manifest       ServiceLevelObjective     `json:"manifest"`
}
type SLOTemplateCategory struct {
	ID    string `json:"id" yaml:"id"`
	Title string `json:"title" yaml:"title"`
}
type SLOTemplateDetail struct {
	APIVersion string      `json:"apiVersion"`
	Tenant     string      `json:"tenant"`
	Template   SLOTemplate `json:"template"`
}

type SLOTemplateCatalog struct {
	Extensions []SLOExtensionInfo    `json:"extensions,omitempty"`
	Tenant     string                `json:"tenant,omitempty"`
	APIVersion string                `json:"apiVersion"`
	Kind       string                `json:"kind"`
	Digest     string                `json:"digest"`
	Categories []SLOTemplateCategory `json:"categories"`
	Templates  []SLOTemplate         `json:"templates"`
}

// TemplateValues replaces a complete measurement when supplied; it cannot add
// arbitrary queries, destinations, credentials, approval or deployment options.
type TemplateValues struct {
	Inputs                 map[string]any   `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	ExpectedTemplateDigest string           `json:"expectedTemplateDigest,omitempty" yaml:"expectedTemplateDigest,omitempty"`
	Name                   string           `json:"name,omitempty" yaml:"name,omitempty"`
	Project                string           `json:"project,omitempty" yaml:"project,omitempty"`
	Service                string           `json:"service,omitempty" yaml:"service,omitempty"`
	Source                 string           `json:"source,omitempty" yaml:"source,omitempty"`
	Measurement            *MeasurementSpec `json:"measurement,omitempty" yaml:"measurement,omitempty"`
	Target                 *float64         `json:"target,omitempty" yaml:"target,omitempty"`
	WindowSeconds          *int64           `json:"windowSeconds,omitempty" yaml:"windowSeconds,omitempty"`
	StepSeconds            *int64           `json:"stepSeconds,omitempty" yaml:"stepSeconds,omitempty"`
}
type SLOTemplateDraft struct {
	Extension         *ExtensionReference   `json:"extension,omitempty"`
	Tenant            string                `json:"tenant,omitempty"`
	APIVersion        string                `json:"apiVersion"`
	Kind              string                `json:"kind"`
	TemplateID        string                `json:"templateId"`
	TemplateRevision  int                   `json:"templateRevision"`
	TemplateDigest    string                `json:"templateDigest"`
	ManifestDigest    string                `json:"manifestDigest"`
	TelemetryVerified bool                  `json:"telemetryVerified"`
	Manifest          ServiceLevelObjective `json:"manifest"`
	YAML              string                `json:"yaml"`
	Warnings          []string              `json:"warnings"`
}
