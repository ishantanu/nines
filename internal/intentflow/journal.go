package intentflow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/tenancy"
)

// AuditSearch is server-owned; callers cannot select another tenant's journal.
type AuditSearch struct {
	Query           v1.AuditQuery
	Tenant          string
	BeforeAt        time.Time
	BeforeID        string
	ID              string
	ThroughSequence int64
	AfterSequence   int64
	Ascending       bool
}

var auditRecordID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)

type auditCursor struct {
	Tenant string    `json:"tenant"`
	Filter string    `json:"filter"`
	At     time.Time `json:"at"`
	ID     string    `json:"id"`
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
}

func lastAudit(events []v1.IntentAuditEvent) (int64, string) {
	if len(events) == 0 {
		return 0, ""
	}
	e := events[len(events)-1]
	return e.Sequence, e.Hash
}
func journalAppend(old, next record) ([]v1.AuditRecord, error) {
	sequence, previous := lastAudit(old.Session.Audit)
	out := []v1.AuditRecord{}
	for _, event := range next.Session.Audit {
		if event.Sequence <= sequence {
			continue
		}
		if event.Sequence != sequence+1 || event.PreviousHash != previous {
			return nil, ErrConflict
		}
		unsigned := event
		unsigned.Hash = ""
		if event.Hash != hash(unsigned) {
			return nil, model.Invalid("audit event integrity check failed")
		}
		item := v1.AuditRecord{APIVersion: v1.APIVersion, ID: model.NewID(), Tenant: next.Session.Tenant, Conversation: next.Session.ID, Event: event, Resources: []v1.IntentAppliedResource{}}
		if event.Action == "systemone.requested" || event.Action == "systemone.assessed" {
			for _, run := range next.Session.Runs {
				var assessment *v1.SystemOneAssessment
				if event.Action == "systemone.assessed" {
					assessment = run.SystemOne
				}
				evidence := systemOneEvidence(run, assessment)
				if hash(evidence) == event.EvidenceDigest {
					item.SystemOne = evidence
					break
				}
			}
			if item.SystemOne == nil {
				return nil, model.Invalid("System One audit evidence missing or changed")
			}
		}
		changes := next.Session.ChangeSetHistory
		if next.Session.ChangeSet != nil {
			changes = append(clone(changes), *next.Session.ChangeSet)
		}
		for _, change := range changes {
			matches := change.Digest == event.ChangeSetDigest
			if change.Rollback != nil && change.Rollback.Digest == event.ChangeSetDigest {
				matches = true
			}
			for _, rollback := range change.RollbackHistory {
				if rollback.Digest == event.ChangeSetDigest {
					matches = true
				}
			}
			if !matches {
				continue
			}
			for _, resource := range change.Changes {
				item.Resources = append(item.Resources, v1.IntentAppliedResource{Kind: resource.Kind, Name: resource.Name, Revision: resource.ExpectedRevision, Digest: resource.Digest})
			}
			if event.Action == "change_set.approved" {
				item.ApprovalDigest = change.ApprovalDigest
			}
			if event.Action == "rollback.approved" && change.Rollback != nil {
				item.ApprovalDigest = change.Rollback.ApprovalDigest
			}
			if event.Action == "change_set.applied" && event.EvidenceDigest != "" {
				item.SafetyCheck = clone(change.SafetyCheck)
			}
			if event.Action == "rollback.applied" && event.EvidenceDigest != "" && change.Rollback != nil {
				item.SafetyCheck = clone(change.Rollback.SafetyCheck)
			}
			if event.Action == "change_set.applied" && event.Outcome == "blocked" && event.EvidenceDigest != "" {
				item.SafetyCheck = clone(change.SafetyCheck)
			}
			if event.Action == "change_set.observed" {
				item.Outcome = clone(change.Outcome)
			}
			if event.Action == "rollback.observed" && change.Rollback != nil {
				item.Outcome = clone(change.Rollback.Outcome)
			}
			if event.Action == "change_set.proposed" || event.Action == "rollback.proposed" || event.Action == "change_set.approved" || event.Action == "rollback.approved" {
				frozen := clone(change)
				item.Proposal = &frozen
			}
			break
		}
		if len(item.Resources) == 0 && next.Session.Scope.Metadata.Name != "" {
			item.Resources = append(item.Resources, v1.IntentAppliedResource{Kind: v1.SLOKind, Name: next.Session.Scope.Metadata.Name})
		}
		out = append(out, item)
		sequence = event.Sequence
		previous = event.Hash
	}
	return out, nil
}
func auditMatches(item v1.AuditRecord, search AuditSearch) bool {
	q := search.Query
	if search.ID != "" && item.ID != search.ID {
		return false
	}
	if search.ThroughSequence > 0 && item.Event.Sequence > search.ThroughSequence {
		return false
	}
	if item.Tenant != search.Tenant || q.Conversation != "" && q.Conversation != item.Conversation || q.Actor != "" && q.Actor != item.Event.Actor.Subject || q.Action != "" && q.Action != item.Event.Action || q.Outcome != "" && q.Outcome != item.Event.Outcome {
		return false
	}
	if !q.From.IsZero() && item.Event.At.Before(q.From) || !q.To.IsZero() && item.Event.At.After(q.To) {
		return false
	}
	if search.Ascending {
		if item.Event.Sequence <= search.AfterSequence {
			return false
		}
	} else if !search.BeforeAt.IsZero() && (item.Event.At.After(search.BeforeAt) || item.Event.At.Equal(search.BeforeAt) && item.ID >= search.BeforeID) {
		return false
	}
	if q.Resource != "" {
		for _, r := range item.Resources {
			if r.Name == q.Resource {
				return true
			}
		}
		return false
	}
	return true
}
func sortAudit(items []v1.AuditRecord, ascending bool) {
	sort.Slice(items, func(i, j int) bool {
		if ascending {
			return items[i].Event.Sequence < items[j].Event.Sequence
		}
		if items[i].Event.At.Equal(items[j].Event.At) {
			return items[i].ID > items[j].ID
		}
		return items[i].Event.At.After(items[j].Event.At)
	})
}
func (s *Service) ListAudit(ctx context.Context, q v1.AuditQuery) (v1.AuditPage, error) {
	out := v1.AuditPage{APIVersion: v1.APIVersion, Tenant: s.Platform.TenantID, Items: []v1.AuditRecord{}}
	p, ok := tenancy.FromContext(ctx)
	if !ok || p.Subject == "" || p.TenantID != s.Platform.TenantID || (p.Role != "owner" && p.Role != "editor") {
		return out, ErrForbidden
	}
	if q.Limit == 0 {
		q.Limit = 50
	}
	if q.Limit < 1 || q.Limit > 100 || q.Conversation != "" && !identifier.MatchString(q.Conversation) || q.Resource != "" && !identifier.MatchString(q.Resource) || len(q.Actor) > 256 || len(q.Action) > 64 || len(q.Outcome) > 32 || q.Action != "" && !auditActionValid(q.Action) || q.Outcome != "" && q.Outcome != "succeeded" && q.Outcome != "failed" && q.Outcome != "denied" && q.Outcome != "conflict" && q.Outcome != "blocked" && q.Outcome != "uncertain" {
		return out, model.Invalid("invalid audit filter or limit (1–100)")
	}
	search := AuditSearch{Tenant: s.Platform.TenantID}
	var cursor auditCursor
	if q.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(q.Cursor)
		if err != nil || len(q.Cursor) > 2048 || json.Unmarshal(raw, &cursor) != nil || cursor.Tenant != s.Platform.TenantID || !auditRecordID.MatchString(cursor.ID) || cursor.At.IsZero() {
			return out, model.Invalid("invalid tenant-scoped audit cursor")
		}
		if q.From.IsZero() {
			q.From = cursor.From
		}
		if q.To.IsZero() {
			q.To = cursor.To
		}
		search.BeforeAt = cursor.At
		search.BeforeID = cursor.ID
	}
	if q.To.IsZero() {
		q.To = time.Now().UTC()
	}
	if q.From.IsZero() {
		q.From = q.To.Add(-30 * 24 * time.Hour)
	}
	if !q.To.After(q.From) || q.To.Sub(q.From) > 366*24*time.Hour {
		return out, model.Invalid("audit range must be positive and at most 366 days")
	}
	filter := q
	filter.Cursor = ""
	filter.Limit = 0
	if q.Cursor != "" && cursor.Filter != hash(filter) {
		return out, model.Invalid("audit cursor does not match filters")
	}
	limit := q.Limit
	q.Limit++
	search.Query = q
	rows, err := s.Repository.Audit(ctx, search)
	if err != nil {
		return out, err
	}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		raw, _ := json.Marshal(auditCursor{Tenant: s.Platform.TenantID, Filter: hash(filter), At: last.Event.At, ID: last.ID, From: q.From, To: q.To})
		out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	for i := range rows {
		rows[i].Proposal = nil
	}
	out.Items = rows
	out.From = q.From
	out.To = q.To
	return out, nil
}
func auditResources(item v1.AuditRecord) []string {
	out := []string{}
	for _, r := range item.Resources {
		out = append(out, r.Name)
	}
	return out
}
func auditActionValid(action string) bool {
	parts := strings.Split(action, ".")
	return len(parts) == 2 && identifier.MatchString(strings.ReplaceAll(parts[0], "_", "-")) && identifier.MatchString(strings.ReplaceAll(parts[1], "_", "-"))
}

// AuditRecord returns the frozen proposal only after tenant and reviewer authorization.
func (s *Service) AuditRecord(ctx context.Context, id string) (v1.AuditRecord, error) {
	p, ok := tenancy.FromContext(ctx)
	if !ok || p.Subject == "" || p.TenantID != s.Platform.TenantID || (p.Role != "owner" && p.Role != "editor") {
		return v1.AuditRecord{}, ErrForbidden
	}
	if !auditRecordID.MatchString(id) {
		return v1.AuditRecord{}, model.Invalid("invalid audit record ID")
	}
	rows, err := s.Repository.Audit(ctx, AuditSearch{Tenant: p.TenantID, ID: id, Query: v1.AuditQuery{Limit: 1}})
	if err != nil {
		return v1.AuditRecord{}, err
	}
	if len(rows) == 0 {
		return v1.AuditRecord{}, ErrNotFound
	}
	item := rows[0]
	if err := validateJournalRecord(item); err != nil {
		return v1.AuditRecord{}, err
	}
	return item, nil
}

func validJournalHead(old record, head *v1.AuditRecord) error {
	if len(old.Session.Audit) == 0 {
		return nil
	}
	if head == nil || head.Tenant != old.Session.Tenant || head.Conversation != old.Session.ID || hash(head.Event) != hash(old.Session.Audit[len(old.Session.Audit)-1]) {
		return model.Invalid("audit journal head mismatch; inspect retained records before changing definitions")
	}
	return nil
}

func validateJournalRecord(item v1.AuditRecord) error {
	if item.Proposal != nil {
		matches := item.Proposal.Digest == item.Event.ChangeSetDigest
		if item.Proposal.Rollback != nil && item.Proposal.Rollback.Digest == item.Event.ChangeSetDigest {
			matches = true
		}
		for _, rollback := range item.Proposal.RollbackHistory {
			if rollback.Digest == item.Event.ChangeSetDigest {
				matches = true
			}
		}
		if !matches {
			return model.Invalid("audit proposal digest mismatch")
		}
	}
	if item.SafetyCheck != nil && hash(item.SafetyCheck) != item.Event.EvidenceDigest {
		return model.Invalid("audit safety evidence digest mismatch")
	}
	if item.Outcome != nil && hash(item.Outcome) != item.Event.EvidenceDigest {
		return model.Invalid("audit outcome evidence digest mismatch")
	}
	if item.SystemOne != nil && ((item.Event.Action != "systemone.requested" && item.Event.Action != "systemone.assessed") || hash(item.SystemOne) != item.Event.EvidenceDigest) {
		return model.Invalid("System One audit evidence digest mismatch")
	}
	if item.Event.EvidenceDigest != "" && item.SafetyCheck == nil && item.Outcome == nil && item.SystemOne == nil {
		valid := item.ApprovalDigest != "" && hash(item.ApprovalDigest) == item.Event.EvidenceDigest && (item.Event.Action == "change_set.approved" || item.Event.Action == "rollback.approved")
		if valid && item.Proposal != nil {
			if item.Event.Action == "change_set.approved" {
				valid = approvalDigest(*item.Proposal) == item.ApprovalDigest
			}
			if item.Event.Action == "rollback.approved" {
				valid = item.Proposal.Rollback != nil && rollbackApprovalDigest(*item.Proposal.Rollback) == item.ApprovalDigest
			}
		}
		if !valid {
			return model.Invalid("audit evidence is missing or changed")
		}
	}
	unsigned := item.Event
	unsigned.Hash = ""
	if !auditChainValid([]v1.IntentAuditEvent{item.Event}) || hash(unsigned) != item.Event.Hash || (item.Proposal != nil && !frozenReviewsValid(v1.IntentConversation{Tenant: item.Tenant, IntentConversationSummary: v1.IntentConversationSummary{ID: item.Conversation}, ChangeSet: item.Proposal})) {
		return model.Invalid("audit record integrity check failed")
	}
	return nil
}

// Approval evidence is read from the independent journal, not trusted solely from
// mutable conversation metadata. It is fetched before the conversation lock.
func (s *Service) approvalProofs(ctx context.Context, id, digest, action string) (map[string]bool, error) {
	rows, err := s.Repository.Audit(ctx, AuditSearch{Tenant: s.Platform.TenantID, Query: v1.AuditQuery{Conversation: id, Action: action, Limit: 100}})
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, row := range rows {
		if row.Event.ChangeSetDigest != digest || row.Event.Outcome != "succeeded" {
			continue
		}
		if err := validateJournalRecord(row); err != nil {
			return nil, err
		}
		proof := row.ApprovalDigest
		if proof != "" && hash(proof) == row.Event.EvidenceDigest {
			out[proof] = true
		}
	}
	return out, nil
}
