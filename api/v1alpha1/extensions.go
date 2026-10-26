package v1alpha1

// SLOExtensionBundle is declarative content. It cannot execute code or add runtime permissions.
type SLOExtensionBundle struct {
	APIVersion string            `json:"apiVersion" yaml:"apiVersion"`
	Kind       string            `json:"kind" yaml:"kind"`
	Metadata   ExtensionMetadata `json:"metadata" yaml:"metadata"`
	Spec       ExtensionSpec     `json:"spec" yaml:"spec"`
}
type ExtensionMetadata struct {
	Name        string `json:"name" yaml:"name"`
	Version     string `json:"version" yaml:"version"`
	Publisher   string `json:"publisher" yaml:"publisher"`
	Description string `json:"description" yaml:"description"`
}
type ExtensionSpec struct {
	Engine      string                `json:"engine" yaml:"engine"`
	Categories  []SLOTemplateCategory `json:"categories,omitempty" yaml:"categories,omitempty"`
	Recipes     []ExtensionRecipe     `json:"recipes" yaml:"recipes"`
	Policies    []ExtensionPolicy     `json:"policies,omitempty" yaml:"policies,omitempty"`
	Evaluations []ExtensionEvaluation `json:"evaluations,omitempty" yaml:"evaluations,omitempty"`
	Tests       []ExtensionTest       `json:"tests" yaml:"tests"`
}
type ExtensionInput struct {
	Field       string   `json:"field" yaml:"field"`
	Type        string   `json:"type" yaml:"type"`
	Description string   `json:"description" yaml:"description"`
	Required    bool     `json:"required" yaml:"required"`
	Default     any      `json:"default,omitempty" yaml:"default,omitempty"`
	Minimum     *float64 `json:"minimum,omitempty" yaml:"minimum,omitempty"`
	Maximum     *float64 `json:"maximum,omitempty" yaml:"maximum,omitempty"`
	Enum        []string `json:"enum,omitempty" yaml:"enum,omitempty"`
}
type ExtensionRecipe struct {
	Name           string                    `json:"name" yaml:"name"`
	Category       string                    `json:"category" yaml:"category"`
	SLI            string                    `json:"sli" yaml:"sli"`
	Title          string                    `json:"title" yaml:"title"`
	Description    string                    `json:"description" yaml:"description"`
	EligibleEvents string                    `json:"eligibleEvents" yaml:"eligibleEvents"`
	GoodEvents     string                    `json:"goodEvents" yaml:"goodEvents"`
	Requirements   []string                  `json:"requirements" yaml:"requirements"`
	Inputs         map[string]ExtensionInput `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Policy         string                    `json:"policy,omitempty" yaml:"policy,omitempty"`
	Evaluation     string                    `json:"evaluation,omitempty" yaml:"evaluation,omitempty"`
	Manifest       ServiceLevelObjective     `json:"manifest" yaml:"manifest"`
}
type ExtensionPolicy struct {
	Name          string                    `json:"name" yaml:"name"`
	Description   string                    `json:"description" yaml:"description"`
	Target        *float64                  `json:"target,omitempty" yaml:"target,omitempty"`
	WindowSeconds *int64                    `json:"windowSeconds,omitempty" yaml:"windowSeconds,omitempty"`
	Policies      []BurnRatePolicy          `json:"policies,omitempty" yaml:"policies,omitempty"`
	Quality       *MeasurementQualityPolicy `json:"quality,omitempty" yaml:"quality,omitempty"`
}
type ExtensionEvaluation struct {
	PopulationLabels map[string]string `json:"populationLabels" yaml:"populationLabels"`
	Name             string            `json:"name" yaml:"name"`
	Version          string            `json:"version" yaml:"version"`
	Rubric           string            `json:"rubric" yaml:"rubric"`
	Sampling         string            `json:"sampling" yaml:"sampling"`
	UnknownVerdict   string            `json:"unknownVerdict" yaml:"unknownVerdict"`
	MissingVerdict   string            `json:"missingVerdict" yaml:"missingVerdict"`
}
type ExtensionTest struct {
	Name          string         `json:"name" yaml:"name"`
	Recipe        string         `json:"recipe" yaml:"recipe"`
	Inputs        map[string]any `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	ExpectError   bool           `json:"expectError,omitempty" yaml:"expectError,omitempty"`
	GoodSelector  string         `json:"goodSelector,omitempty" yaml:"goodSelector,omitempty"`
	TotalSelector string         `json:"totalSelector,omitempty" yaml:"totalSelector,omitempty"`
}
type ExtensionReference struct {
	Name    string `json:"name" yaml:"name"`
	Version string `json:"version" yaml:"version"`
	SHA256  string `json:"sha256" yaml:"sha256"`
}
type SLOExtensionInfo struct {
	ExtensionReference
	Publisher   string `json:"publisher"`
	Description string `json:"description"`
	Engine      string `json:"engine"`
	Recipes     int    `json:"recipes"`
	Tests       int    `json:"tests"`
}
type SLOExtensionLock struct {
	APIVersion string               `json:"apiVersion" yaml:"apiVersion"`
	Kind       string               `json:"kind" yaml:"kind"`
	Extensions []ExtensionLockEntry `json:"extensions" yaml:"extensions"`
}
type ExtensionLockEntry struct {
	Name    string `json:"name" yaml:"name"`
	Version string `json:"version" yaml:"version"`
	SHA256  string `json:"sha256" yaml:"sha256"`
	Path    string `json:"path" yaml:"path"`
}
