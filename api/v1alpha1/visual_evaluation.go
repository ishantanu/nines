package v1alpha1

import "time"

const VisualConsentNotice = "visual-sharing-v1"

type CreateVisualEvaluation struct {
	RouteDigest      string `json:"routeDigest"`
	ID               string `json:"id"`
	JourneyRevision  int64  `json:"journeyRevision"`
	SuccessCriterion string `json:"successCriterion"`
	ImageBase64      string `json:"imageBase64"`
	Consent          bool   `json:"consent"`
	NoticeVersion    string `json:"noticeVersion"`
}
type VisualJudgments struct {
	SuccessVisible     float64 `json:"successVisible"`
	ErrorVisible       float64 `json:"errorVisible"`
	EvidenceSufficient float64 `json:"evidenceSufficient"`
}
type VisualAssessment struct {
	Status        string           `json:"status"`
	ResolvedModel string           `json:"resolvedModel,omitempty"`
	Judgments     *VisualJudgments `json:"judgments,omitempty"`
	InputTokens   int              `json:"inputTokens"`
	OutputTokens  int              `json:"outputTokens"`
}
type EvidenceConsent struct {
	NoticeVersion string             `json:"noticeVersion"`
	At            time.Time          `json:"at"`
	Actor         InvestigationActor `json:"actor"`
	Purpose       string             `json:"purpose"`
}
type VisualEvaluation struct {
	APIVersion      string           `json:"apiVersion"`
	ID              string           `json:"id"`
	Tenant          string           `json:"tenant"`
	JourneyID       string           `json:"journeyId"`
	JourneyRevision int64            `json:"journeyRevision"`
	JourneyDigest   string           `json:"journeyDigest"`
	CriterionDigest string           `json:"criterionDigest"`
	ImageDigest     string           `json:"imageDigest"`
	RequestDigest   string           `json:"requestDigest"`
	Route           SystemOneRoute   `json:"route"`
	Consent         EvidenceConsent  `json:"consent"`
	Assessment      VisualAssessment `json:"assessment"`
	CreatedAt       time.Time        `json:"createdAt"`
	ExpiresAt       time.Time        `json:"expiresAt"`
	Review          *VisualReview    `json:"review,omitempty"`
}
type VisualReview struct {
	Outcome string             `json:"outcome"`
	Reason  string             `json:"reason"`
	Actor   InvestigationActor `json:"actor"`
	At      time.Time          `json:"at"`
}
type ReviewVisualEvaluation struct {
	RequestDigest string `json:"requestDigest"`
	Outcome       string `json:"outcome"`
	Reason        string `json:"reason"`
}

type VisualEvaluationList struct {
	APIVersion string             `json:"apiVersion"`
	Tenant     string             `json:"tenant"`
	Items      []VisualEvaluation `json:"items"`
}

type VisualCapabilities struct {
	APIVersion    string          `json:"apiVersion"`
	Tenant        string          `json:"tenant"`
	Enabled       bool            `json:"enabled"`
	Experimental  bool            `json:"experimental"`
	NoticeVersion string          `json:"noticeVersion"`
	Route         *SystemOneRoute `json:"route,omitempty"`
	RouteDigest   string          `json:"routeDigest,omitempty"`
}
