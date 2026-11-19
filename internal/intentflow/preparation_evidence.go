package intentflow

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func preparationEvidenceMatches(v v1.IntentConversation) bool {
	receipt := v.PreparationEvidence
	if v.Goal == nil || receipt == nil || receipt.CapturedVersion < 1 || receipt.CapturedVersion > v.Version || !savedCompilationMatches(v, hash(v.Goal)) {
		return false
	}
	report := clone(receipt.Report)
	expected := report.Digest
	report.Digest = ""
	if !auditDigestPattern.MatchString(expected) || hash(report) != expected || report.APIVersion != v1.APIVersion || report.Tenant != v.Tenant || report.IntentDigest != hash(*v.Goal) || report.MeasurementDigest != v.Plan.Compiled.MeasurementDigest || report.ExpectedRevision != v.Plan.ExpectedRevision {
		return false
	}
	source := hash(struct{ Tenant, Source, Route string }{v.Tenant, v.Scope.Source, v.Plan.SourceDigest})
	if report.SourceDigest != source {
		return false
	}
	return v.Template == nil || v.Template.CatalogDigest == report.CatalogDigest
}
func (s *Service) preparationEvidenceCurrent(ctx context.Context, v v1.IntentConversation) error {
	if !preparationEvidenceMatches(v) || v.PreparationEvidence.Report.SourceDigest != s.Platform.IntentSourceDigest(v.Scope.Source) || v.PreparationEvidence.Report.CatalogDigest != s.templateCatalogDigest() {
		return ErrConflict
	}
	current, err := s.Platform.Store.Read(ctx, v.Goal.Metadata.Name)
	if errors.Is(err, model.ErrNotFound) {
		if v.Plan.ExpectedRevision == 0 {
			return nil
		}
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if model.ConfigRevision(current.SLO) != v.Plan.ExpectedRevision {
		return ErrConflict
	}
	return nil
}

// RecordPreparationEvidence does provider reads outside the conversation lock,
// then saves one version-fenced receipt. It never confirms or applies an SLO.
func (s *Service) RecordPreparationEvidence(ctx context.Context, id string, in v1.RecordIntentEvidence) (out v1.IntentConversation, err error) {
	defer func() { s.auditFailure(ctx, id, "preparation.evidence_recorded", "", err) }()
	ctx, cancel := context.WithTimeout(ctx, 80*time.Second)
	defer cancel()
	a, err := actor(ctx, s.Platform.TenantID)
	if err != nil {
		return out, err
	}
	if !identifier.MatchString(id) || in.ExpectedVersion < 1 || !auditDigestPattern.MatchString(in.IntentDigest) || !in.ReadTelemetry {
		return out, model.Invalid("provide conversation, exact version/intent digest and readTelemetry: true")
	}
	b, err := json.Marshal(in)
	if err != nil || len(b) > 4096 || s.sensitiveInput(in) {
		return out, model.Invalid("evidence request must be credential-free and at most 4 KiB")
	}
	var detached v1.RecordIntentEvidence
	if err := json.Unmarshal(b, &detached); err != nil {
		return out, model.Invalid("invalid evidence request")
	}
	in = detached
	if s.Authorize == nil || s.Authorize(ctx, a) != nil {
		return out, ErrForbidden
	}
	r, err := s.Repository.Read(ctx, id)
	if err != nil {
		return out, err
	}
	if r.Session.Tenant != s.Platform.TenantID {
		return out, ErrNotFound
	}
	before := r.Session
	if active(r) || before.Version != in.ExpectedVersion || before.ChangeSet != nil {
		return out, ErrConflict
	}
	if before.State != "ready" || before.Goal == nil || before.Goal.Spec.Measurement == nil || hash(*before.Goal) != in.IntentDigest || !savedCompilationMatches(before, in.IntentDigest) || before.Selection == nil || before.Selection.IntentDigest != in.IntentDigest || !selectionMatchesCompilation(before) {
		return out, model.Invalid("prepare and human-confirm an inline measurement before recording evidence")
	}
	if before.Plan.SourceDigest != s.Platform.SourceFingerprint(before.Scope.Source) {
		return out, ErrConflict
	}
	catalog := s.templateCatalogDigest()
	if before.Template != nil && before.Template.CatalogDigest != catalog {
		return out, ErrConflict
	}
	report, err := s.Platform.PrepareIntentEvidence(ctx, v1.IntentEvidenceInput{Plan: v1.PlanIntentInput{Intent: clone(*before.Goal)}, IntentDigest: in.IntentDigest, MeasurementDigest: before.Plan.Compiled.MeasurementDigest, SourceDigest: s.Platform.IntentSourceDigest(before.Scope.Source), CatalogDigest: catalog, ExpectedRevision: before.Plan.ExpectedRevision, History: in.History, Alternatives: in.Alternatives}, s.Templates)
	if err != nil {
		return out, err
	}
	receipt := v1.IntentPreparationEvidence{CapturedVersion: before.Version, RequestedBy: a, Report: report}
	if s.sensitiveInput(receipt) {
		return out, model.Invalid("evidence contains configured credentials")
	}
	saved, err := s.Repository.Update(ctx, id, func(n *record, now time.Time) error {
		if n.Session.Tenant != s.Platform.TenantID {
			return ErrNotFound
		}
		if active(*n) || n.Session.Version != before.Version || n.Session.ChangeSet != nil || hash(n.Session.Goal) != hash(before.Goal) || hash(n.Session.Plan) != hash(before.Plan) {
			return ErrConflict
		}
		if s.Authorize == nil || s.Authorize(ctx, a) != nil {
			return ErrForbidden
		}
		if err := auditRoom(n); err != nil {
			return err
		}
		n.Session.PreparationEvidence = &receipt
		if err := s.preparationEvidenceCurrent(ctx, n.Session); err != nil {
			return err
		}
		if err := validSize(*n); err != nil {
			return err
		}
		touch(n, now)
		audit(n, a, "preparation.evidence_recorded", "succeeded", "", "Server-generated evidence saved for the exact reviewed measurement. This is not approval.", now, struct{ Request, Report string }{hash(in), report.Digest})
		return nil
	})
	return saved.Session, err
}
