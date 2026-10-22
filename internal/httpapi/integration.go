package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func (a *Server) integrationStatus(w http.ResponseWriter, r *http.Request) {
	record, err := a.Service.Store.Read(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	now := a.Service.Now().UTC()
	out := v1.IntegrationStatus{APIVersion: model.APIVersion, Kind: "SLOIntegrationStatus", Tenant: a.Service.TenantID, SLOID: record.SLO.ID, Revision: model.ConfigRevision(record.SLO), CheckedAt: now,
		Definition: v1.IntegrationStage{State: "saved", Message: fmt.Sprintf("Definition revision %d is saved in Nines.", model.ConfigRevision(record.SLO))},
		Collection: v1.IntegrationStage{State: "not_collected", Message: "No successful provider collection has been observed for this revision."},
		Deployment: v1.IntegrationStage{State: "not_configured", Message: "Configure a rule destination to deploy recording and alert rules."},
		Evaluation: v1.IntegrationStage{State: "unknown", Message: "External rule evaluation has not been verified."},
	}
	h := record.Collection
	if a.Service.CollectorFor(record.SLO) == nil || !record.SLO.HasQueries() {
		out.Collection = v1.IntegrationStage{State: "not_configured", Message: "This SLO has no configured provider collector and queries."}
	} else if h.State == "failed" {
		out.Collection = v1.IntegrationStage{State: "failed", Message: "The latest collection attempt failed. Inspect the source connection and server logs."}
	} else if h.SLORevision == out.Revision && h.State == "succeeded" {
		out.Collection = v1.IntegrationStage{State: "empty", Message: "The provider query succeeded but returned no measurement intervals.", ObservedAt: &h.CompletedAt}
		if h.Points > 0 {
			out.Collection.State = "collected"
			out.Collection.Message = fmt.Sprintf("The latest collection stored %d measurements. This does not deploy external rules.", h.Points)
			limit := max(45*time.Second, 2*time.Duration(record.SLO.StepSeconds)*time.Second)
			if h.CompletedAt.After(now) || now.Sub(h.CompletedAt) > limit || h.To.After(now) || now.Sub(h.To) > limit+45*time.Second {
				out.Collection.State = "stale"
				out.Collection.Message = "Measurements were collected, but the latest provider range is stale."
			}
		}
	}
	if a.Workspace.Rules != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		out.Deployment, out.Evaluation = a.Workspace.Rules.Inspect(ctx, record.SLO, now)
	}
	write(w, 200, out)
}
