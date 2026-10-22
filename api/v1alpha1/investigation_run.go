package v1alpha1

import "time"

// InvestigationRun is a durable, bounded read-only job. Existing investigation
// and declarative resource representations are unchanged.
type InvestigationRun struct {
	Skill                *ReviewSkillRef        `json:"skill,omitempty"`
	SkillExplicit        bool                   `json:"skillExplicit,omitempty"`
	SkillInputDigest     string                 `json:"skillInputDigest,omitempty"`
	SkillResult          *ReviewSkillResult     `json:"skillResult,omitempty"`
	Mode                 string                 `json:"mode,omitempty"`
	Question             string                 `json:"question,omitempty"`
	ModelRoute           *ReviewModelRoute      `json:"modelRoute,omitempty"`
	ModelPolicy          string                 `json:"modelPolicy,omitempty"`
	ModelCalls           int                    `json:"modelCalls,omitempty"`
	ModelInputBytes      int                    `json:"modelInputBytes,omitempty"`
	OutputTokensReserved int                    `json:"outputTokensReserved,omitempty"`
	ReportedInputTokens  int                    `json:"reportedInputTokens,omitempty"`
	ReportedOutputTokens int                    `json:"reportedOutputTokens,omitempty"`
	UsageMissing         int                    `json:"usageMissing,omitempty"`
	ToolCalls            int                    `json:"toolCalls,omitempty"`
	ReceivedBytes        int                    `json:"receivedBytes,omitempty"`
	ModelReview          *ModelReview           `json:"modelReview,omitempty"`
	APIVersion           string                 `json:"apiVersion"`
	Tenant               string                 `json:"tenant"`
	ID                   string                 `json:"id"`
	InvestigationID      string                 `json:"investigationId"`
	ReviewVersion        int64                  `json:"reviewVersion"`
	Version              int64                  `json:"version"`
	Scope                InvestigationScope     `json:"scope"`
	RequestedBy          InvestigationActor     `json:"requestedBy"`
	CancelledBy          *InvestigationActor    `json:"cancelledBy,omitempty"`
	Status               string                 `json:"status"`
	ErrorCode            string                 `json:"errorCode,omitempty"`
	CreatedAt            time.Time              `json:"createdAt"`
	UpdatedAt            time.Time              `json:"updatedAt"`
	StartedAt            *time.Time             `json:"startedAt,omitempty"`
	FinishedAt           *time.Time             `json:"finishedAt,omitempty"`
	DeadlineAt           time.Time              `json:"deadlineAt"`
	Attempts             int                    `json:"attempts"`
	Steps                []InvestigationRunStep `json:"steps"`
}
type InvestigationRunStep struct {
	CallID      string                 `json:"callId,omitempty"`
	Tool        string                 `json:"tool"`
	Status      string                 `json:"status"`
	Attempts    int                    `json:"attempts"`
	CompletedAt *time.Time             `json:"completedAt,omitempty"`
	Evidence    *InvestigationEvidence `json:"evidence,omitempty"`
	Assessment  *EvidenceAssessment    `json:"assessment,omitempty"`
}

// EvidenceAssessment reports observations rather than inferred causes.
type EvidenceAssessment struct {
	EvidenceDigest string            `json:"evidenceDigest"`
	State          string            `json:"state"`
	Findings       []EvidenceFinding `json:"findings"`
	Limitations    []string          `json:"limitations"`
}
type EvidenceFinding struct {
	ResourceID string `json:"resourceId"`
	State      string `json:"state"`
	Message    string `json:"message"`
}
type CreateInvestigationRun struct {
	Skill           *ReviewSkillRef `json:"skill,omitempty"`
	Mode            string          `json:"mode,omitempty"`
	Question        string          `json:"question,omitempty"`
	ID              string          `json:"id"`
	ExpectedVersion int64           `json:"expectedVersion"`
}
type CancelInvestigationRun struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}
type InvestigationRunList struct {
	APIVersion string             `json:"apiVersion"`
	Tenant     string             `json:"tenant"`
	Items      []InvestigationRun `json:"items"`
}

type ReviewModelRoute struct {
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Protocol    string `json:"protocol"`
	RouteDigest string `json:"routeDigest"`
	Local       bool   `json:"local"`
}
type ModelReview struct {
	Summary         string   `json:"summary"`
	EvidenceDigests []string `json:"evidenceDigests"`
	Limitations     []string `json:"limitations"`
}
type ReviewCapabilities struct {
	APIVersion      string            `json:"apiVersion"`
	Tenant          string            `json:"tenant"`
	Model           *ReviewModelRoute `json:"model,omitempty"`
	MaxModelCalls   int               `json:"maxModelCalls"`
	MaxToolCalls    int               `json:"maxToolCalls"`
	MaxOutputTokens int               `json:"maxOutputTokens"`
	MaxInputBytes   int               `json:"maxInputBytes"`
}
