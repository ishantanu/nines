package v1alpha1

import "time"

// SystemOneRoute is a server-owned semantic assessment route, not a tool model.
type SystemOneRoute struct {
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	ContractDigest string `json:"contractDigest"`
}
type SystemOneChoice struct {
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}
type SystemOneJudgments struct {
	Category                SystemOneChoice `json:"category"`
	PopulationDefined       float64         `json:"populationDefined"`
	GoodEventsDefined       float64         `json:"goodEventsDefined"`
	ObservationPointDefined float64         `json:"observationPointDefined"`
}

// SystemOneAssessment never constitutes measurement verification or approval.
// Input text remains in the original conversation; the receipt contains hashes.
type SystemOneAssessment struct {
	Status        string              `json:"status"`
	ErrorCode     string              `json:"errorCode,omitempty"`
	ResolvedModel string              `json:"resolvedModel,omitempty"`
	RequestDigest string              `json:"requestDigest,omitempty"`
	InputDigest   string              `json:"inputDigest"`
	Judgments     *SystemOneJudgments `json:"judgments,omitempty"`
	InputTokens   int                 `json:"inputTokens"`
	OutputTokens  int                 `json:"outputTokens"`
	At            time.Time           `json:"at"`
}

// SystemOneAuditEvidence is retained independently from a mutable conversation.
type SystemOneAuditEvidence struct {
	RunID       string               `json:"runId"`
	Route       *SystemOneRoute      `json:"route"`
	InputDigest string               `json:"inputDigest"`
	Assessment  *SystemOneAssessment `json:"assessment,omitempty"`
}
