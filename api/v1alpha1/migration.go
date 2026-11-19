package v1alpha1

// MigrationRequest asks Nines to convert another tool's SLO definitions. The
// conversion is stateless: nothing is saved, planned or applied.
type MigrationRequest struct {
	// Tool is the source format: sloth or pyrra.
	Tool    string `json:"tool"`
	Content string `json:"content"`
	// Project, Source, WindowSeconds and Lifecycle are optional import choices.
	Project       string `json:"project,omitempty"`
	Source        string `json:"source,omitempty"`
	WindowSeconds int64  `json:"windowSeconds,omitempty"`
	Lifecycle     string `json:"lifecycle,omitempty"`
}

// MigrationNote explains one approximation, omission or decision. Level is
// info (nothing lost), review (a choice to confirm) or lossy (information that
// the other tool had is not carried over).
type MigrationNote struct {
	Level   string `json:"level"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// MigrationDefinition is one converted SLO, ready for review and planning.
type MigrationDefinition struct {
	Name   string          `json:"name"`
	Origin string          `json:"origin"`
	YAML   string          `json:"yaml"`
	Notes  []MigrationNote `json:"notes"`
}

// MigrationSkipped is a definition that could not be converted faithfully.
// Nothing is guessed: the reason says what to change.
type MigrationSkipped struct {
	Origin string `json:"origin"`
	Reason string `json:"reason"`
}

// MigrationResult is the outcome of a conversion.
type MigrationResult struct {
	APIVersion  string                `json:"apiVersion"`
	Tenant      string                `json:"tenant,omitempty"`
	Tool        string                `json:"tool"`
	Definitions []MigrationDefinition `json:"definitions"`
	Skipped     []MigrationSkipped    `json:"skipped"`
	Notes       []MigrationNote       `json:"notes"`
	Limitations []string              `json:"limitations"`
}
