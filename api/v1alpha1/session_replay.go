package v1alpha1

import (
	"encoding/json"
	"time"
)

const ReplayConsentNotice = "replay-sharing-v1"

type ReplayReference struct {
	Kind     string     `json:"kind"`
	ID       string     `json:"id"`
	Revision int64      `json:"revision"`
	SLOID    string     `json:"sloId,omitempty"`
	At       *time.Time `json:"at,omitempty"`
}
type SessionReplay struct {
	ConsentReceipt *EvidenceConsent   `json:"consentReceipt,omitempty"`
	APIVersion     string             `json:"apiVersion"`
	Tenant         string             `json:"tenant"`
	ID             string             `json:"id"`
	Title          string             `json:"title"`
	Version        int64              `json:"version"`
	CreatedAt      time.Time          `json:"createdAt"`
	ExpiresAt      time.Time          `json:"expiresAt"`
	StartedAt      time.Time          `json:"startedAt"`
	FinishedAt     time.Time          `json:"finishedAt"`
	DurationMillis int64              `json:"durationMillis"`
	EventCount     int                `json:"eventCount"`
	SizeBytes      int                `json:"sizeBytes"`
	Digest         string             `json:"digest"`
	Policy         string             `json:"policy"`
	RecordedBy     InvestigationActor `json:"recordedBy"`
	References     []ReplayReference  `json:"references"`
	Archived       bool               `json:"archived"`
}
type CreateSessionReplay struct {
	ID             string            `json:"id"`
	Title          string            `json:"title"`
	SharingConsent bool              `json:"sharingConsent"`
	NoticeVersion  string            `json:"noticeVersion"`
	Consent        bool              `json:"consent"`
	Policy         string            `json:"policy"`
	References     []ReplayReference `json:"references"`
	Events         []json.RawMessage `json:"events"`
}
type UpdateReplayLinks struct {
	ExpectedVersion int64             `json:"expectedVersion"`
	References      []ReplayReference `json:"references"`
}
type ArchiveReplay struct {
	ExpectedVersion int64 `json:"expectedVersion"`
	Archived        bool  `json:"archived"`
}
type SessionReplayList struct {
	APIVersion    string          `json:"apiVersion"`
	Tenant        string          `json:"tenant"`
	Items         []SessionReplay `json:"items"`
	RetentionDays int             `json:"retentionDays"`
	MaxSessions   int             `json:"maxSessions"`
}
type ReplayEvents struct {
	APIVersion string            `json:"apiVersion"`
	Tenant     string            `json:"tenant"`
	ID         string            `json:"id"`
	Digest     string            `json:"digest"`
	Events     []json.RawMessage `json:"events"`
}

type ReplayPrivacy struct {
	Disabled  bool   `json:"disabled,omitempty" yaml:"disabled,omitempty"`
	PolicyURL string `json:"policyURL,omitempty" yaml:"policyURL,omitempty"`
	Contact   string `json:"contact,omitempty" yaml:"contact,omitempty"`
}
