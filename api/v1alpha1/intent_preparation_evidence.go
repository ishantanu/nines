package v1alpha1

// RecordIntentEvidence requests fresh server-generated evidence. No client
// receipt, source route, credentials or confirmation can be supplied.
type RecordIntentEvidence struct {
	ExpectedVersion int64                       `json:"expectedVersion" yaml:"expectedVersion"`
	IntentDigest    string                      `json:"intentDigest" yaml:"intentDigest"`
	ReadTelemetry   bool                        `json:"readTelemetry" yaml:"readTelemetry"`
	History         *IntentEvidenceRange        `json:"history,omitempty" yaml:"history,omitempty"`
	Alternatives    []IntentEvidenceAlternative `json:"alternatives,omitempty" yaml:"alternatives,omitempty"`
}
type IntentPreparationEvidence struct {
	CapturedVersion int64              `json:"capturedVersion"`
	RequestedBy     InvestigationActor `json:"requestedBy"`
	Report          IntentEvidence     `json:"report"`
}
