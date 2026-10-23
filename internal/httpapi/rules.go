package httpapi

import (
	"context"
	"net/http"
	"time"

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
		result, err = a.Workspace.Rules.Withdraw(ctx, record.SLO, body.ExpectedDigest)
	} else {
		result, err = a.Workspace.Rules.Deploy(ctx, record.SLO, body.ExpectedDigest)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
