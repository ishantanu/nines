package httpapi

import (
	"context"
	"net/http"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func (a *Server) retirementPlan(w http.ResponseWriter, r *http.Request) {
	q, e := parseContextQuery(r, "action")
	if e != nil {
		contextError(w, e)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	out, e := a.Service.PlanRetirement(ctx, r.PathValue("id"), q["action"])
	if e != nil {
		contextError(w, e)
		return
	}
	write(w, 200, out)
}
func (a *Server) retirementApply(w http.ResponseWriter, r *http.Request) {
	if _, e := parseContextQuery(r); e != nil {
		contextError(w, e)
		return
	}
	var in v1.RetirementInput
	if e := decode(w, r, &in); e != nil {
		contextError(w, e)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	out, e := a.Service.ApplyRetirement(ctx, r.PathValue("id"), in)
	if e != nil {
		contextError(w, e)
		return
	}
	write(w, 200, out)
}
func (a *Server) retirementStatus(w http.ResponseWriter, r *http.Request) {
	if _, e := parseContextQuery(r); e != nil {
		contextError(w, e)
		return
	}
	out, e := a.Service.RetirementReceipt(r.Context(), r.PathValue("id"))
	if e != nil {
		contextError(w, e)
		return
	}
	write(w, 200, out)
}
func (a *Server) retirementRetry(w http.ResponseWriter, r *http.Request) {
	if _, e := parseContextQuery(r); e != nil {
		contextError(w, e)
		return
	}
	// The owner retries the already reviewed, persisted cleanup. No route or rule
	// content may be supplied by the request, including when credentials rotate.
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	out, e := a.Service.ReconcileRetirement(ctx, r.PathValue("id"))
	if e != nil {
		contextError(w, e)
		return
	}
	write(w, 200, out)
}
