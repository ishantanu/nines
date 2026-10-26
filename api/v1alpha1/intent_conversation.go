package v1alpha1

import (
	"encoding/json"
	"time"
)

type IntentAuthoringScope struct {
	Metadata     Metadata          `json:"metadata"`
	Service      string            `json:"service"`
	Source       string            `json:"source"`
	Population   map[string]string `json:"population"`
	NoDataPolicy string            `json:"noDataPolicy"`
	Journey      *IntentJourney    `json:"journey,omitempty"`
}
type IntentConversationMessage struct {
	ID    string             `json:"id"`
	Role  string             `json:"role"`
	Text  string             `json:"text"`
	At    time.Time          `json:"at"`
	Actor InvestigationActor `json:"actor"`
	RunID string             `json:"runId,omitempty"`
}
type IntentFlowStep struct {
	Tool   string          `json:"tool"`
	At     time.Time       `json:"at"`
	Digest string          `json:"digest"`
	Result json.RawMessage `json:"result"`
}

// IntentModelProgress contains aggregate transport progress only. It never
// exposes provider text, arguments, hidden reasoning or native correlation data.
type IntentModelProgress struct {
	ModelCall int       `json:"modelCall"`
	State     string    `json:"state"`
	Bytes     int       `json:"bytes"`
	At        time.Time `json:"at"`
}
type IntentFlowRun struct {
	SystemOneInputDigest  string               `json:"systemOneInputDigest,omitempty"`
	SystemOneConsent      bool                 `json:"systemOneConsent,omitempty"`
	SystemOneRoute        *SystemOneRoute      `json:"systemOneRoute,omitempty"`
	SystemOneCalls        int                  `json:"systemOneCalls,omitempty"`
	SystemOne             *SystemOneAssessment `json:"systemOne,omitempty"`
	TemplateCatalogDigest string               `json:"templateCatalogDigest,omitempty"`
	Progress              *IntentModelProgress `json:"progress,omitempty"`
	InputDigest           string               `json:"inputDigest"`
	ModelConsent          bool                 `json:"modelConsent"`
	DraftModelID          string               `json:"draftModelId,omitempty"`
	DraftRoute            *ReviewModelRoute    `json:"draftRoute,omitempty"`
	ID                    string               `json:"id"`
	Mode                  string               `json:"mode"`
	Status                string               `json:"status"`
	Actor                 InvestigationActor   `json:"actor"`
	CreatedAt             time.Time            `json:"createdAt"`
	UpdatedAt             time.Time            `json:"updatedAt"`
	DeadlineAt            time.Time            `json:"deadlineAt"`
	Attempts              int                  `json:"attempts"`
	ModelCalls            int                  `json:"modelCalls"`
	ModelInputBytes       int                  `json:"modelInputBytes"`
	ModelRoute            *ReviewModelRoute    `json:"modelRoute,omitempty"`
	Skill                 ReviewSkillRef       `json:"skill"`
	Steps                 []IntentFlowStep     `json:"steps"`
	ErrorCode             string               `json:"errorCode,omitempty"`
}
type IntentBindingConfirmation struct {
	MeasurementDigest string             `json:"measurementDigest,omitempty"`
	IntentDigest      string             `json:"intentDigest"`
	Binding           IntentSelection    `json:"binding"`
	Actor             InvestigationActor `json:"actor"`
	At                time.Time          `json:"at"`
}
type IntentConversationSummary struct {
	ID        string               `json:"id"`
	Title     string               `json:"title"`
	Version   int64                `json:"version"`
	State     string               `json:"state"`
	CreatedAt time.Time            `json:"createdAt"`
	UpdatedAt time.Time            `json:"updatedAt"`
	Scope     IntentAuthoringScope `json:"scope"`
}

// IntentTemplateSelection records a recipe choice, not telemetry or approval evidence.
type IntentTemplateSelection struct {
	TemplateID       string              `json:"templateId"`
	TemplateRevision int                 `json:"templateRevision"`
	TemplateDigest   string              `json:"templateDigest"`
	CatalogDigest    string              `json:"catalogDigest"`
	Extension        *ExtensionReference `json:"extension,omitempty"`
	ValuesDigest     string              `json:"valuesDigest"`
	IntentDigest     string              `json:"intentDigest"`
	At               time.Time           `json:"at"`
}
type IntentConversation struct {
	Template         *IntentTemplateSelection `json:"template,omitempty"`
	Audit            []IntentAuditEvent       `json:"audit,omitempty"`
	ChangeSetHistory []IntentChangeSet        `json:"changeSetHistory,omitempty"`
	ChangeSet        *IntentChangeSet         `json:"changeSet,omitempty"`
	APIVersion       string                   `json:"apiVersion"`
	Tenant           string                   `json:"tenant"`
	IntentConversationSummary
	CreatedBy InvestigationActor          `json:"createdBy"`
	Messages  []IntentConversationMessage `json:"messages"`
	Goal      *SLOIntent                  `json:"goal,omitempty"`
	Selection *IntentBindingConfirmation  `json:"selection,omitempty"`
	Plan      *IntentPlan                 `json:"plan,omitempty"`
	Backtest  *IntentBacktest             `json:"backtest,omitempty"`
	Runs      []IntentFlowRun             `json:"runs"`
}
type CreateIntentConversation struct {
	ID    string               `json:"id"`
	Title string               `json:"title"`
	Scope IntentAuthoringScope `json:"scope"`
	Text  string               `json:"text"`
	Goal  *SLOIntent           `json:"goal,omitempty"`
}
type AppendIntentConversation struct {
	ID              string     `json:"id"`
	ExpectedVersion int64      `json:"expectedVersion"`
	Text            string     `json:"text"`
	Goal            *SLOIntent `json:"goal,omitempty"`
}
type ConfirmIntentBinding struct {
	MeasurementDigest string          `json:"measurementDigest,omitempty"`
	ExpectedVersion   int64           `json:"expectedVersion"`
	IntentDigest      string          `json:"intentDigest"`
	Binding           IntentSelection `json:"binding"`
	Confirm           bool            `json:"confirm"`
}
type CreateIntentFlowRun struct {
	SystemOneConsent bool                      `json:"systemOneConsent,omitempty"`
	ID               string                    `json:"id"`
	ExpectedVersion  int64                     `json:"expectedVersion"`
	Mode             string                    `json:"mode"`
	ModelConsent     bool                      `json:"modelConsent"`
	Backtest         *IntentFlowBacktestWindow `json:"backtest,omitempty"`
}
type IntentFlowBacktestWindow struct {
	From              time.Time `json:"from"`
	To                time.Time `json:"to"`
	ResolutionSeconds int64     `json:"resolutionSeconds"`
}
type IntentConversationList struct {
	APIVersion string                      `json:"apiVersion"`
	Tenant     string                      `json:"tenant"`
	Items      []IntentConversationSummary `json:"items"`
	NextCursor string                      `json:"nextCursor,omitempty"`
}
