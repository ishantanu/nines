package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/systemone"
	"github.com/ishantanu/nines/internal/tenancy"
)

func (a *Server) visualAdapter() systemone.VisualAssessor {
	if a.Workspace.IntentConversations != nil {
		if v, ok := a.Workspace.IntentConversations.SystemOne.(systemone.VisualAssessor); ok && v.VisualEnabled() {
			return v
		}
	}
	return nil
}
func (a *Server) visualCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		a.fail(w, r, model.Invalid("query fields are not supported"))
		return
	}
	v := a.visualAdapter()
	out := map[string]any{"apiVersion": v1.APIVersion, "tenant": a.Service.TenantID, "enabled": v != nil, "experimental": true, "noticeVersion": v1.VisualConsentNotice, "imageStorage": "not retained", "receiptRetentionDays": 30, "maxImageBytes": systemone.MaxVisualImageBytes, "minDimension": 280, "maxDimension": 1568}
	if v != nil {
		out["route"] = v.VisualDescriptor()
		out["routeDigest"] = model.DefinitionDigest(v.VisualDescriptor())
	}
	write(w, 200, out)
}
func (a *Server) visualList(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		a.fail(w, r, model.Invalid("query fields are not supported"))
		return
	}
	st, ok := a.Service.Store.(store.VisualStore)
	if !ok {
		a.fail(w, r, model.ErrUnavailable)
		return
	}
	out, e := st.ListVisual(r.Context(), r.PathValue("id"))
	if e != nil {
		a.fail(w, r, e)
		return
	}
	for _, v := range out {
		if v.Tenant != a.Service.TenantID {
			a.fail(w, r, model.ErrUnavailable)
			return
		}
	}
	write(w, 200, v1.VisualEvaluationList{APIVersion: v1.APIVersion, Tenant: a.Service.TenantID, Items: out})
}
func (a *Server) visualCreate(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		a.fail(w, r, model.Invalid("query fields are not supported"))
		return
	}
	p, ok := tenancy.FromContext(r.Context())
	if !ok || p.TenantID != a.Service.TenantID || (p.Role != "owner" && p.Role != "editor") {
		write(w, 403, map[string]string{"error": "an owner or editor must explicitly authorize image sharing"})
		return
	}
	var in v1.CreateVisualEvaluation
	if e := decodeLimit(w, r, &in, 3<<20); e != nil {
		a.fail(w, r, e)
		return
	}
	if !in.Consent || in.NoticeVersion != v1.VisualConsentNotice {
		a.fail(w, r, model.Invalid("explicit consent to visual-sharing-v1 is required for this image and criterion"))
		return
	}
	if id, e := uuid.Parse(in.ID); e != nil || id.String() != in.ID || id.Version() != 4 || !systemone.VisualCriterionValid(in.SuccessCriterion) || in.JourneyRevision < 1 {
		a.fail(w, r, model.Invalid("provide a UUIDv4, exact journey revision and success criterion (1–2000 bytes)"))
		return
	}
	v := a.visualAdapter()
	st, ok := a.Service.Store.(store.VisualStore)
	if v == nil || !ok || a.Service.Journeys == nil {
		a.fail(w, r, model.Invalid("visual evaluation is disabled; explicitly configure systemOne.imagesEnabled"))
		return
	}
	if guard, ok := v.(interface{ ContainsCredential([]byte) bool }); ok && guard.ContainsCredential([]byte(in.SuccessCriterion)) {
		a.fail(w, r, model.Invalid("credentials do not belong in visual criteria"))
		return
	}
	journey, e := a.Service.Journeys.JourneyRevision(r.Context(), r.PathValue("id"), in.JourneyRevision)
	if e != nil {
		a.fail(w, r, e)
		return
	}
	route := v.VisualDescriptor()
	if in.RouteDigest != model.DefinitionDigest(route) {
		a.fail(w, r, model.Invalid("provider/model contract changed; inspect capabilities and grant fresh consent"))
		return
	}
	pixels, imageHash, e := systemone.PrepareVisualImage(in.ImageBase64)
	if e != nil {
		a.fail(w, r, model.Invalid("%s", e.Error()))
		return
	}
	actor := v1.InvestigationActor{Subject: p.Subject, Kind: p.Kind, Name: p.Name}
	criterionHash := model.DefinitionDigest(in.SuccessCriterion)
	requestHash := model.DefinitionDigest(struct {
		Journey, Image, Criterion, Notice string
		Revision                          int64
		Actor                             v1.InvestigationActor
		Route                             v1.SystemOneRoute
	}{journey.Digest, imageHash, criterionHash, in.NoticeVersion, journey.Revision, actor, route})
	now := time.Now().UTC()
	receipt := v1.VisualEvaluation{APIVersion: v1.APIVersion, ID: in.ID, Tenant: a.Service.TenantID, JourneyID: r.PathValue("id"), JourneyRevision: journey.Revision, JourneyDigest: journey.Digest, CriterionDigest: criterionHash, ImageDigest: imageHash, RequestDigest: requestHash, Route: route, CreatedAt: now, ExpiresAt: now.Add(30 * 24 * time.Hour), Consent: v1.EvidenceConsent{At: now, Actor: actor, NoticeVersion: in.NoticeVersion, Purpose: "one image assessment by " + route.Provider}, Assessment: v1.VisualAssessment{Status: "pending"}}
	saved, created, e := st.ReserveVisual(r.Context(), receipt)
	if e != nil {
		a.fail(w, r, e)
		return
	}
	if !created {
		write(w, 200, saved)
		return
	}
	assessed, err := v.AssessVisual(r.Context(), systemone.VisualInput{Criterion: in.SuccessCriterion, Image: pixels})
	if err != nil {
		assessed = v1.VisualAssessment{Status: "unavailable"}
		switch {
		case errors.Is(err, systemone.ErrPaused):
			assessed.Status = "paused"
		case errors.Is(err, systemone.ErrBudgetExhausted):
			assessed.Status = "budget_exhausted"
		}
	}
	// Persist a terminal receipt even if the caller disconnects after sharing.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	saved, e = st.UpdateVisual(ctx, in.ID, func(out *v1.VisualEvaluation) error {
		if out.Tenant != a.Service.TenantID || out.RequestDigest != requestHash || out.Assessment.Status != "pending" {
			return model.ErrConflict
		}
		out.Assessment = assessed
		return nil
	})
	if e != nil {
		a.fail(w, r, e)
		return
	}
	write(w, 201, saved)
}
func (a *Server) visualReview(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		a.fail(w, r, model.Invalid("query fields are not supported"))
		return
	}
	p, ok := tenancy.FromContext(r.Context())
	if !ok || p.TenantID != a.Service.TenantID || (p.Role != "owner" && p.Role != "editor") {
		write(w, 403, map[string]string{"error": "an owner or editor must review visual evidence"})
		return
	}
	var in v1.ReviewVisualEvaluation
	if e := decode(w, r, &in); e != nil {
		a.fail(w, r, e)
		return
	}
	if (in.Outcome != "success" && in.Outcome != "failure" && in.Outcome != "inconclusive") || !systemone.VisualCriterionValid(in.Reason) || len(in.RequestDigest) != 64 {
		a.fail(w, r, model.Invalid("provide the receipt digest, an outcome and a review reason"))
		return
	}
	if a.Workspace.IntentConversations != nil {
		if guard, ok := a.Workspace.IntentConversations.SystemOne.(interface{ ContainsCredential([]byte) bool }); ok && guard.ContainsCredential([]byte(in.Reason)) {
			a.fail(w, r, model.Invalid("credentials do not belong in review reasons"))
			return
		}
	}
	st, ok := a.Service.Store.(store.VisualStore)
	if !ok {
		a.fail(w, r, model.ErrUnavailable)
		return
	}
	out, e := st.UpdateVisual(r.Context(), r.PathValue("evaluation"), func(out *v1.VisualEvaluation) error {
		if out.Tenant != p.TenantID || out.JourneyID != r.PathValue("id") {
			return model.ErrNotFound
		}
		if out.RequestDigest != in.RequestDigest || out.Assessment.Status != "assessed" {
			return model.ErrConflict
		}
		if out.Review != nil {
			if out.Review.Outcome == in.Outcome && out.Review.Reason == strings.TrimSpace(in.Reason) && out.Review.Actor.Subject == p.Subject && out.Review.Actor.Kind == p.Kind {
				return nil
			}
			return model.ErrConflict
		}
		out.Review = &v1.VisualReview{Outcome: in.Outcome, Reason: strings.TrimSpace(in.Reason), Actor: v1.InvestigationActor{Subject: p.Subject, Kind: p.Kind, Name: p.Name}, At: time.Now().UTC()}
		return nil
	})
	if e != nil {
		a.fail(w, r, e)
		return
	}
	write(w, 200, out)
}
