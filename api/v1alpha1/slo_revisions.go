package v1alpha1

import "time"

// SLORevision describes a frozen definition and the evidence retained when it
// was replaced. It is not an assertion that the archived SLO was healthy.
type SLORevision struct {
	Revision           int64                 `json:"revision"`
	Digest             string                `json:"digest"`
	Manifest           ServiceLevelObjective `json:"manifest"`
	ArchivedAt         time.Time             `json:"archivedAt"`
	ArchivedBy         InvestigationActor    `json:"archivedBy"`
	IntervalSamples    int                   `json:"intervalSamples"`
	WindowObservations int                   `json:"windowObservations"`
}
