package intentflow

import (
	"context"
	"encoding/json"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// ReviewArtifact reads a revision-fenced conversation and complete audited journal.
// If it changes while reading, callers must retry and review the new snapshot.
func (s *Service) ReviewArtifact(ctx context.Context, id string) (v1.IntentReviewArtifact, error) {
	var out v1.IntentReviewArtifact
	before, err := s.Read(ctx, id)
	if err != nil {
		return out, err
	}
	if before.ChangeSet == nil {
		return out, model.Invalid("prepare a change set before exporting a review artifact")
	}
	if before.ChangeSet.Digest != changeDigest(s.Platform.TenantID, id, *before.ChangeSet) {
		return out, ErrConflict
	}
	events := []v1.IntentAuditEvent{}
	var after, through int64
	for {
		page, err := s.AuditPage(ctx, id, after, through)
		if err != nil {
			return out, err
		}
		if page.Integrity != "verified" {
			return out, model.Invalid("audit integrity verification failed")
		}
		if through == 0 {
			through = page.Through
		}
		events = append(events, page.Events...)
		if len(events) > 4096 {
			return out, ErrLimit
		}
		if page.NextAfter == 0 {
			break
		}
		if page.NextAfter <= after {
			return out, ErrConflict
		}
		after = page.NextAfter
	}
	beforeSequence, beforeHash := lastAudit(before.Audit)
	if beforeSequence != through {
		return out, ErrConflict
	}
	if through > 0 && (len(events) == 0 || events[len(events)-1].Hash != beforeHash) {
		return out, ErrConflict
	}
	current, err := s.Read(ctx, id)
	if err != nil {
		return out, err
	}
	currentSequence, currentHash := lastAudit(current.Audit)
	if current.Version != before.Version || currentSequence != beforeSequence || currentHash != beforeHash || current.ChangeSet == nil || current.ChangeSet.Digest != before.ChangeSet.Digest {
		return out, ErrConflict
	}
	out = v1.IntentReviewArtifact{APIVersion: v1.APIVersion, Tenant: before.Tenant, Conversation: before.ID, Version: before.Version, CapturedAt: time.Now().UTC(), Status: workflowStatus(before), ChangeSet: clone(*before.ChangeSet), AuditIntegrity: "verified", AuditThrough: through, Audit: events, Limitations: []string{"Snapshot contains potentially sensitive definitions and human review notes; share only with authorized recipients.", "A saved proposal or configuration does not establish deployed rules or healthy runtime.", "Approval is bound to the exact change and version; this export does not authorize an action.", "Configuration rollback cannot recover consumed error budget or reverse a service deployment."}}
	encoded, err := json.Marshal(out)
	if err != nil {
		return v1.IntentReviewArtifact{}, err
	}
	if len(encoded) > 3<<20 {
		return v1.IntentReviewArtifact{}, ErrLimit
	}
	return out, nil
}
