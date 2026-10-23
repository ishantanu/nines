package httpapi

import (
	"context"
	"net/http"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
)

func (a *Server) rulePlan(w http.ResponseWriter, r *http.Request) {
	if a.Workspace.Rules == nil {
		a.fail(w, r, model.ErrUnavailable)
		return
	}
	record, err := a.Service.Store.Read(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	plan, err := a.Workspace.Rules.PlanContext(r.Context(), record.SLO)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, plan)
}
func (a *Server) ruleApply(w http.ResponseWriter, r *http.Request)    { a.ruleMutation(w, r, false) }
func (a *Server) ruleWithdraw(w http.ResponseWriter, r *http.Request) { a.ruleMutation(w, r, true) }
func (a *Server) ruleMutation(w http.ResponseWriter, r *http.Request, withdraw bool) {
	if a.Workspace.Rules == nil {
		a.fail(w, r, model.ErrUnavailable)
		return
	}
	var body struct {
		ExpectedDigest string `json:"expectedDigest"`
	}
	if err := decode(w, r, &body); err != nil {
		a.fail(w, r, err)
		return
	}
	a.Service.DefinitionMu.Lock()
	defer a.Service.DefinitionMu.Unlock()
	record, err := a.Service.Store.Read(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var result rules.Deployment
	if withdraw {
		target, _ := a.Workspace.Rules.TargetFor(record.SLO)
		if target.Kind != "operator" {
			a.fail(w, r, model.Invalid("withdraw is supported only for operator publications; pause automation before manual provider cleanup"))
			return
		}
		if err = a.Service.PauseAutomaticRules(ctx, record, a.Workspace.Rules, body.ExpectedDigest); err != nil {
			a.fail(w, r, err)
			return
		}
		result, err = a.Workspace.Rules.Withdraw(ctx, record.SLO, body.ExpectedDigest)
	} else {
		result, err = a.Workspace.Rules.Deploy(ctx, record.SLO, body.ExpectedDigest)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if !withdraw {
		if err = a.Service.ResumeAutomaticRules(ctx, record, a.Workspace.Rules); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	write(w, 200, result)
}

func (a *Server) ruleAutomation(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ExpectedRevision int64 `json:"expectedRevision"`
		Paused           *bool `json:"paused"`
	}
	if e := decode(w, r, &in); e != nil {
		a.fail(w, r, e)
		return
	}
	if in.Paused == nil {
		a.fail(w, r, model.Invalid("paused must be explicitly true or false"))
		return
	}
	result, e := a.Service.SetRuleAutomation(r.Context(), r.PathValue("id"), in.ExpectedRevision, *in.Paused, a.Workspace.Rules)
	if e != nil {
		a.fail(w, r, e)
		return
	}
	write(w, 200, v1.RuleAutomationResult{APIVersion: v1.APIVersion, Tenant: a.Service.TenantID, SLOID: r.PathValue("id"), Automation: *result})
}
