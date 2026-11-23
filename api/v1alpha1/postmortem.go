package v1alpha1

import "time"

// Postmortem is a blameless review of an incident, written on an incident
// review. Each save appends a full revision; earlier revisions stay. The
// timeline and impact are not written here: they come from the incident
// record captured as evidence, so nobody retypes them.
type Postmortem struct {
	Summary string `json:"summary"`
	// AddedEvents are timeline entries Nines could not know, such as a
	// deploy or a decision in a chat channel.
	AddedEvents []PostmortemEvent `json:"addedEvents"`
	// ContributingCauses are several, never one root cause.
	ContributingCauses []ContributingCause `json:"contributingCauses"`
	Lessons            PostmortemLessons   `json:"lessons"`
	ActionItems        []ActionItem        `json:"actionItems"`
	// Published makes the postmortem visible from the incident.
	Published bool `json:"published"`
}

type PostmortemEvent struct {
	Time time.Time `json:"time"`
	Text string    `json:"text"`
}

// Contributing cause kinds.
const (
	CauseTrigger   = "trigger"   // what set the incident off
	CauseLatent    = "latent"    // a condition that was already there
	CauseSafeguard = "safeguard" // a missing or weak defence
)

type ContributingCause struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// PostmortemLessons are the three questions of a blameless review.
type PostmortemLessons struct {
	WentWell   []string `json:"wentWell"`
	WentPoorly []string `json:"wentPoorly"`
	GotLucky   []string `json:"gotLucky"`
}

// Action item kinds.
const (
	ActionPrevent  = "prevent"
	ActionDetect   = "detect"
	ActionMitigate = "mitigate"
)

// ActionItem is tracked work. Open items show on the SLOs and journeys they
// link until they are done.
type ActionItem struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Owner    string   `json:"owner"`
	Due      string   `json:"due,omitempty"` // YYYY-MM-DD
	Kind     string   `json:"kind"`
	SLOs     []string `json:"slos,omitempty"`
	Journeys []string `json:"journeys,omitempty"`
	Done     bool     `json:"done"`
}

// DefineSLOAction is the standing action item on a postmortem for an
// incident no SLO detected. It can be marked done, not removed.
const DefineSLOAction = "define-slo"

// SavePostmortem appends a postmortem revision at the review's version.
type SavePostmortem struct {
	ID              string     `json:"id"`
	ExpectedVersion int64      `json:"expectedVersion"`
	Postmortem      Postmortem `json:"postmortem"`
}

// HindsightFinding flags wording that judges with hindsight or blames.
type HindsightFinding struct {
	Field      string `json:"field"`
	Phrase     string `json:"phrase"`
	Suggestion string `json:"suggestion"`
}

// PostmortemTemplate starts a postmortem from the incident record.
type PostmortemTemplate struct {
	Postmortem Postmortem `json:"postmortem"`
	// Revision is the latest saved revision's entry ID, if any.
	Revision string             `json:"revision,omitempty"`
	Findings []HindsightFinding `json:"findings"`
}

// OpenActionItem is an open action item with the review it belongs to.
type OpenActionItem struct {
	ReviewID    string     `json:"reviewId"`
	ReviewTitle string     `json:"reviewTitle"`
	IncidentID  string     `json:"incidentId"`
	Item        ActionItem `json:"item"`
}
