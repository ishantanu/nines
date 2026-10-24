package v1alpha1

import "time"

type AuditArchiveReceipt struct {
	Bucket      string    `json:"bucket"`
	Key         string    `json:"key"`
	VersionID   string    `json:"versionId"`
	SHA256      string    `json:"sha256"`
	Mode        string    `json:"mode"`
	RetainUntil time.Time `json:"retainUntil"`
	VerifiedAt  time.Time `json:"verifiedAt"`
}
type AuditArchiveStatus struct {
	APIVersion string               `json:"apiVersion"`
	Tenant     string               `json:"tenant"`
	Enabled    bool                 `json:"enabled"`
	State      string               `json:"state"`
	Target     string               `json:"target,omitempty"`
	Pending    int64                `json:"pending"`
	Failed     int64                `json:"failed"`
	Verified   int64                `json:"verified"`
	Retained   int64                `json:"retained"`
	Expired    int64                `json:"expired"`
	RecordID   string               `json:"recordId,omitempty"`
	LastError  string               `json:"lastError,omitempty"`
	Receipt    *AuditArchiveReceipt `json:"receipt,omitempty"`
}
