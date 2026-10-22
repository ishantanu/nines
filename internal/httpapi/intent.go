package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func (a *Server) intentPlan(w http.ResponseWriter, r *http.Request) {
	if _, err := parseContextQuery(r); err != nil {
		telemetryError(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	defer func() {
		_ = r.Body.Close()
	}()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	var in v1.PlanIntentInput
	if err := d.Decode(&in); err != nil {
		telemetryError(w, model.Invalid("invalid intent plan request"))
		return
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		telemetryError(w, model.Invalid("provide exactly one intent plan request"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	out, err := a.Service.PlanIntent(ctx, in)
	if err != nil {
		telemetryError(w, err)
		return
	}
	write(w, 200, out)
}

func intentInput(w http.ResponseWriter, r *http.Request, out any) bool {
	if _, err := parseContextQuery(r); err != nil {
		telemetryError(w, err)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	defer func() {
		_ = r.Body.Close()
	}()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		telemetryError(w, model.Invalid("invalid intent request"))
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		telemetryError(w, model.Invalid("provide one intent request"))
		return false
	}
	return true
}
func (a *Server) intentDraft(w http.ResponseWriter, r *http.Request) {
	var in v1.DraftIntentInput
	if !intentInput(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 22*time.Second)
	defer cancel()
	out, err := a.Service.DraftIntent(ctx, in)
	if err != nil {
		telemetryError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *Server) intentBacktest(w http.ResponseWriter, r *http.Request) {
	var in v1.IntentBacktestInput
	if !intentInput(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	out, err := a.Service.BacktestIntent(ctx, in)
	if err != nil {
		telemetryError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *Server) intentCapabilities(w http.ResponseWriter, r *http.Request) {
	if _, err := parseContextQuery(r); err != nil {
		telemetryError(w, err)
		return
	}
	write(w, 200, map[string]any{"apiVersion": v1.APIVersion, "tenant": a.Service.TenantID, "naturalLanguageEnabled": a.Service.IntentDrafter != nil, "backtestMaxCutoffs": 240, "backtestMaxRangeSeconds": 259200})
}
func (a *Server) bindingHealth(w http.ResponseWriter, r *http.Request) {
	q, err := parseContextQuery(r, "expectedRevision")
	if err != nil {
		telemetryError(w, err)
		return
	}
	rev, err := strconv.ParseInt(q["expectedRevision"], 10, 64)
	if err != nil || rev < 1 {
		telemetryError(w, model.Invalid("positive expectedRevision is required"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	out, err := a.Service.CheckBinding(ctx, r.PathValue("id"), rev)
	if err != nil {
		telemetryError(w, err)
		return
	}
	write(w, 200, out)
}
