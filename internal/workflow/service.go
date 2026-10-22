package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ishantanu/nines/internal/observability"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/skills"
)

type Service struct {
	Platform   *service.Service
	Repository Repository
}

func New(platform *service.Service, repo Repository) *Service { return &Service{platform, repo} }
func (s *Service) Get(ctx context.Context, id string) (Workflow, error) {
	return s.Repository.Get(ctx, id)
}
func (s *Service) List(ctx context.Context) ([]Workflow, error) { return s.Repository.List(ctx) }
func (s *Service) Create(ctx context.Context, req CreateRequest) (result Workflow, operationErr error) {
	ctx, finish := observability.Operation(ctx, "workflow.create", s.Platform.TenantID)
	defer func() { finish(operationErr) }()
	if !identifier.MatchString(req.ID) || len(req.Note) > 4000 {
		return Workflow{}, model.Invalid("workflow ID must be 1–63 lowercase letters, digits or hyphens; note at most 4000 bytes")
	}
	switch req.Kind {
	case "investigate":
		if req.Manifest != nil || (req.JourneyID != "" && !identifier.MatchString(req.JourneyID)) {
			return Workflow{}, model.Invalid("investigate accepts an optional journeyId, not a manifest")
		}
	case "propose_journey":
		if req.Manifest == nil || (req.JourneyID != "" && req.JourneyID != req.Manifest.Metadata.Name) {
			return Workflow{}, model.Invalid("propose_journey requires a manifest with matching journeyId when provided")
		}
	default:
		return Workflow{}, model.Invalid("kind must be investigate or propose_journey")
	}
	digest := requestDigest(req)
	old, err := s.Repository.Get(ctx, req.ID)
	if err == nil {
		if old.RequestDigest != digest {
			return Workflow{}, ErrConflict
		}
		return old, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Workflow{}, err
	}
	now := s.Platform.Now().UTC()
	w := Workflow{ID: req.ID, Kind: req.Kind, JourneyID: req.JourneyID, Note: req.Note, State: "completed", Version: 1, CreatedAt: now, UpdatedAt: now, Findings: []Finding{}, RequestDigest: digest}
	skillID := "reliability-investigation"
	if req.Kind == "propose_journey" {
		plan, err := s.Platform.PlanJourney(ctx, *req.Manifest)
		if err != nil {
			return Workflow{}, err
		}
		if _, ok := s.Platform.Journeys.(store.JourneyApplier); !ok {
			return Workflow{}, model.ErrUnavailable
		}
		w.Proposal = &plan
		w.JourneyID = req.Manifest.Metadata.Name
		w.State = "awaiting_approval"
		skillID = "journey-proposal"
		w.Findings = append(w.Findings, Finding{Code: "proposal_prepared", Severity: "info", Resource: w.JourneyID, Message: fmt.Sprintf("%s proposal against revision %d; explicit approval is required before application", plan.Action, plan.ExpectedRevision)})
	} else {
		if err = s.investigate(ctx, &w); err != nil {
			return Workflow{}, err
		}
	}
	for _, skill := range skills.Catalog() {
		if skill.ID == skillID {
			w.Skill = SkillRef{skill.ID, skill.Version, skill.Digest}
		}
	}
	return s.Repository.Create(ctx, w)
}
func (s *Service) investigate(ctx context.Context, w *Workflow) error {
	var journeys []service.JourneyView
	if w.JourneyID != "" {
		if s.Platform.Journeys == nil {
			return model.ErrUnavailable
		}
		rev, err := s.Platform.Journeys.JourneyRevision(ctx, w.JourneyID, 0)
		if err != nil {
			return err
		}
		view, err := s.Platform.JourneyView(ctx, rev)
		if err != nil {
			return err
		}
		journeys = []service.JourneyView{view}
	} else {
		if s.Platform.Journeys == nil {
			return model.ErrUnavailable
		}
		resources, listErr := s.Platform.Journeys.Journeys(ctx)
		if listErr != nil {
			return listErr
		}
		if len(resources) > 200 {
			resources = resources[:200]
			w.Findings = append(w.Findings, Finding{Code: "scope_truncated", Severity: "warning", Resource: "workspace", Message: "Only the first 200 journeys are included; investigate a journey explicitly for complete detail"})
		}
		for _, revision := range resources {
			view, viewErr := s.Platform.JourneyView(ctx, revision)
			if viewErr != nil {
				return viewErr
			}
			journeys = append(journeys, view)
		}
	}
	observed := map[string]bool{}
	snapshotLimitReported := false
	canObserve := func(id string) bool {
		if observed[id] {
			return false
		}
		if len(observed) >= 1000 {
			if !snapshotLimitReported {
				w.Findings = append(w.Findings, Finding{Code: "scope_truncated", Severity: "warning", Resource: "workspace", Message: "Objective snapshots are limited to 1000 distinct SLOs; scope an investigation to one journey for complete linked evidence"})
				snapshotLimitReported = true
			}
			return false
		}
		observed[id] = true
		return true
	}
	for _, j := range journeys {
		id := j.Resource.Manifest.Metadata.Name
		severity := "info"
		if j.Health == "degraded" {
			severity = "critical"
		} else if j.Health != "healthy" {
			severity = "warning"
		}
		w.Findings = append(w.Findings, Finding{Code: "journey_health", Severity: severity, Resource: id, Message: fmt.Sprintf("Required-link health is %s at journey revision %d; this is not an end-to-end measured success rate", j.Health, j.Resource.Revision)})
		for _, link := range j.Links {
			if canObserve(link.SLO.ID) {
				s.addSnapshot(w, link.Status)
			}
		}
		if j.Resource.Manifest.Spec.Agentic != nil {
			runs, err := s.Platform.Journeys.Runs(ctx, id)
			if err != nil {
				return err
			}
			counts := map[string]int{}
			for _, r := range runs {
				counts[r.Verdict]++
			}
			severity := "info"
			if len(runs) == 0 || counts["incomplete"] > 0 || counts["inconclusive"] > 0 {
				severity = "warning"
			}
			if counts["violated"] > 0 || counts["failed"] > 0 {
				severity = "critical"
			}
			w.Findings = append(w.Findings, Finding{Code: "agent_run_evidence", Severity: severity, Resource: id, Message: fmt.Sprintf("Latest %d caller-reported runs across recorded revisions: passed=%d, violated=%d, failed=%d, incomplete=%d, inconclusive=%d; run evidence is not execution authorization", len(runs), counts["passed"], counts["violated"], counts["failed"], counts["incomplete"], counts["inconclusive"])})
		}
	}
	if w.JourneyID == "" {
		slos, err := s.Platform.Store.List(ctx)
		if err != nil {
			return err
		}
		if len(slos) > 1000 {
			slos = slos[:1000]
			w.Findings = append(w.Findings, Finding{Code: "scope_truncated", Severity: "warning", Resource: "workspace", Message: "Only the first 1000 SLOs are included"})
		}
		for _, slo := range slos {
			if !canObserve(slo.ID) {
				continue
			}
			snapshot, err := s.Platform.Status(ctx, slo.ID, time.Time{})
			if err != nil {
				return err
			}
			s.addSnapshot(w, snapshot)
		}
	}
	if len(w.Findings) == 0 {
		w.Findings = append(w.Findings, Finding{Code: "empty_workspace", Severity: "warning", Resource: "workspace", Message: "No journeys or objectives are configured; reliability is unknown"})
	}
	return nil
}
func (s *Service) addSnapshot(w *Workflow, x model.Snapshot) {
	health := service.SLOHealth(x)
	severity := "info"
	if health == "degraded" {
		severity = "critical"
	} else if health != "healthy" {
		severity = "warning"
	}
	w.Findings = append(w.Findings, Finding{Code: "slo_status", Severity: severity, Resource: x.SLOID, Message: fmt.Sprintf("Observed health=%s, data=%s, coverage=%.2f%% at %s", health, x.DataStatus, x.Coverage*100, x.At.Format("2006-01-02T15:04:05Z")), Snapshot: &x})
}
func (s *Service) Approve(ctx context.Context, id string, req ApprovalRequest) (result Workflow, operationErr error) {
	ctx, finish := observability.Operation(ctx, "workflow.approve", s.Platform.TenantID)
	defer func() { finish(operationErr) }()
	if req.ExpectedVersion < 1 || len(req.Digest) != 64 || len(req.Reason) > 2000 {
		return Workflow{}, model.Invalid("approval requires expectedVersion, the exact 64-character proposal digest, and reason at most 2000 bytes")
	}
	w, err := s.Repository.Update(ctx, id, func(w *Workflow) error {
		if w.Proposal == nil || w.Proposal.Digest != req.Digest {
			return ErrConflict
		}
		if w.State == "approved" || w.State == "applied" || w.State == "conflicted" {
			if w.Decision == nil || w.Decision.Action != "approve" || w.Decision.Reason != req.Reason || (req.ExpectedVersion != w.Decision.ExpectedVersion && req.ExpectedVersion != w.Version) {
				return ErrConflict
			}
			return nil
		}
		if w.State != "awaiting_approval" || w.Version != req.ExpectedVersion {
			return ErrConflict
		}
		now := s.Platform.Now().UTC()
		w.Decision = &Decision{Action: "approve", Reason: req.Reason, At: now, ExpectedVersion: req.ExpectedVersion}
		if principal, ok := tenancy.FromContext(ctx); ok {
			w.Decision.Subject, w.Decision.Role = principal.Subject, principal.Role
		}
		w.State = "approved"
		w.Version++
		w.UpdatedAt = now
		return nil
	})
	if err != nil || w.State != "approved" {
		return w, err
	}
	return s.resume(ctx, id)
}

// Recover resumes only recorded approvals. The journey receipt proves whether
// a prior attempt committed, including a crash before workflow status commit.
func (s *Service) Recover(ctx context.Context) error {
	for {
		pending, err := s.Repository.Recoverable(ctx)
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			return nil
		}
		for _, w := range pending {
			if _, err = s.resume(ctx, w.ID); err != nil {
				return err
			}
		}
	}
}

func (s *Service) resume(ctx context.Context, id string) (result Workflow, operationErr error) {
	ctx, finish := observability.Operation(ctx, "workflow.resume", s.Platform.TenantID)
	defer func() { finish(operationErr) }()
	// The approval above is durable before applying. An atomic journey receipt
	// ties application to this operation and recovers a crash after CAS commit.
	return s.Repository.Update(ctx, id, func(w *Workflow) error {
		if w.State != "approved" {
			return nil
		}
		if err := s.Platform.ValidateJourney(ctx, w.Proposal.Manifest); err != nil {
			return err
		}
		applier, ok := s.Platform.Journeys.(store.JourneyApplier)
		if !ok {
			return model.ErrUnavailable
		}
		next := model.JourneyRevision{Manifest: w.Proposal.Manifest, Digest: w.Proposal.Digest, AppliedAt: w.Decision.At}
		result, err := applier.ApplyJourneyOnce(ctx, next, w.Proposal.ExpectedRevision, "workflow:"+w.ID)
		if errors.Is(err, model.ErrRevisionConflict) {
			w.State = "conflicted"
			w.Error = "Journey changed after proposal; create a new workflow for review"
		} else if err != nil {
			return err
		} else {
			w.State = "applied"
			w.Applied = &result
		}
		w.Version++
		w.UpdatedAt = s.Platform.Now().UTC()
		return nil
	})
}
func (s *Service) Reject(ctx context.Context, id string, req RejectionRequest) (result Workflow, operationErr error) {
	ctx, finish := observability.Operation(ctx, "workflow.reject", s.Platform.TenantID)
	defer func() { finish(operationErr) }()
	if req.ExpectedVersion < 1 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 2000 {
		return Workflow{}, model.Invalid("rejection requires expectedVersion and a reason containing 1–2000 bytes")
	}
	return s.Repository.Update(ctx, id, func(w *Workflow) error {
		if w.State == "rejected" && w.Decision != nil && w.Decision.Reason == req.Reason && (w.Version == req.ExpectedVersion || w.Decision.ExpectedVersion == req.ExpectedVersion) {
			return nil
		}
		if w.State != "awaiting_approval" || w.Version != req.ExpectedVersion {
			return ErrConflict
		}
		now := s.Platform.Now().UTC()
		w.Decision = &Decision{Action: "reject", Reason: req.Reason, At: now, ExpectedVersion: req.ExpectedVersion}
		if principal, ok := tenancy.FromContext(ctx); ok {
			w.Decision.Subject, w.Decision.Role = principal.Subject, principal.Role
		}
		w.State = "rejected"
		w.Version++
		w.UpdatedAt = now
		return nil
	})
}
