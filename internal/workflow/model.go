// Package workflow implements durable tenant-bound reliability workflows.
// A Service is bound to one already-authorized platform service and repository.
package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
)

var (
	ErrNotFound = errors.New("workflow or skill not found")
	ErrConflict = errors.New("workflow input, version or decision conflicts with stored workflow")
	identifier  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
)

type CreateRequest struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	JourneyID string         `json:"journeyId,omitempty"`
	Manifest  *model.Journey `json:"manifest,omitempty"`
	Note      string         `json:"note,omitempty"`
}
type ApprovalRequest struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Digest          string `json:"digest"`
	Reason          string `json:"reason,omitempty"`
}
type RejectionRequest struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
}
type SkillRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}
type Finding struct {
	Code     string          `json:"code"`
	Severity string          `json:"severity"`
	Resource string          `json:"resource"`
	Message  string          `json:"message"`
	Snapshot *model.Snapshot `json:"snapshot,omitempty"`
}
type Decision struct {
	Action          string    `json:"action"`
	Reason          string    `json:"reason"`
	At              time.Time `json:"at"`
	ExpectedVersion int64     `json:"expectedVersion"`
	Subject         string    `json:"subject,omitempty"`
	Role            string    `json:"role,omitempty"`
}
type Workflow struct {
	ID            string                 `json:"id"`
	Kind          string                 `json:"kind"`
	State         string                 `json:"state"`
	Version       int64                  `json:"version"`
	Skill         SkillRef               `json:"skill"`
	JourneyID     string                 `json:"journeyId,omitempty"`
	Note          string                 `json:"note"`
	CreatedAt     time.Time              `json:"createdAt"`
	UpdatedAt     time.Time              `json:"updatedAt"`
	Findings      []Finding              `json:"findings"`
	Proposal      *service.JourneyPlan   `json:"proposal,omitempty"`
	Applied       *model.JourneyRevision `json:"applied,omitempty"`
	Decision      *Decision              `json:"decision,omitempty"`
	Error         string                 `json:"error,omitempty"`
	RequestDigest string                 `json:"requestDigest"`
}

func requestDigest(req CreateRequest) string {
	b, _ := json.Marshal(req)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func clone(w Workflow) Workflow {
	b, _ := json.Marshal(w)
	var out Workflow
	_ = json.Unmarshal(b, &out)
	return out
}
