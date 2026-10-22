package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/ishantanu/nines/internal/model"
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
	plan, err := a.Workspace.Rules.Plan(record.SLO)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, plan)
}
func (a *Server) ruleApply(w http.ResponseWriter, r *http.Request) {
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
	result, err := a.Workspace.Rules.Apply(ctx, record.SLO, body.ExpectedDigest)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
