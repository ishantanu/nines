package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
)

func telemetryError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrIntentModel) {
		write(w, 503, map[string]any{"apiVersion": v1.APIVersion, "error": v1.ContextError{Code: "intent_model_unavailable", Message: err.Error(), Retryable: true, Recovery: "Clarify the goal or ask an operator to verify model configuration"}})
		return
	}
	if errors.Is(err, service.ErrTelemetry) {
		write(w, 502, map[string]any{"apiVersion": v1.APIVersion, "error": v1.ContextError{Code: "telemetry_unavailable", Message: err.Error(), Retryable: true, Recovery: "Refine population labels or ask an operator to check provider compatibility and availability"}})
		return
	}
	contextError(w, err)
}
func (a *Server) metricNames(w http.ResponseWriter, r *http.Request) {
	q, err := parseContextQuery(r, "prefix")
	if err != nil {
		telemetryError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()
	out, err := a.Service.DiscoverMetricNames(ctx, r.PathValue("connection"), q["prefix"])
	if err != nil {
		telemetryError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *Server) telemetryDiscover(w http.ResponseWriter, r *http.Request) {
	q, err := parseContextQuery(r, "metric", "labels")
	if err != nil {
		telemetryError(w, err)
		return
	}
	labels := map[string]string{}
	if q["labels"] != "" {
		if err = json.Unmarshal([]byte(q["labels"]), &labels); err != nil {
			telemetryError(w, model.Invalid("labels must be an equality-filter JSON object"))
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	out, err := a.Service.DiscoverTelemetry(ctx, r.PathValue("connection"), q["metric"], labels)
	if err != nil {
		telemetryError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *Server) topologyList(w http.ResponseWriter, r *http.Request) {
	q, err := parseContextQuery(r, "kind", "cursor", "limit")
	if err != nil {
		telemetryError(w, err)
		return
	}
	limit, err := pageLimit(q["limit"])
	if err != nil {
		telemetryError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	out, err := a.Service.ListTopology(ctx, q["kind"], q["cursor"], limit)
	if err != nil {
		telemetryError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *Server) topologyRead(kind string, manifest bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, err := parseContextQuery(r, "revision")
		if err != nil {
			telemetryError(w, err)
			return
		}
		revision := int64(0)
		if q["revision"] != "" {
			revision, err = strconv.ParseInt(q["revision"], 10, 64)
			if err != nil || revision < 1 {
				telemetryError(w, model.Invalid("revision must be positive"))
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		out, err := a.Service.ReadTopology(ctx, kind, r.PathValue("id"), revision)
		if err != nil {
			telemetryError(w, err)
			return
		}
		w.Header().Set("ETag", `"`+strconv.FormatInt(out.Revision, 10)+`"`)
		if !manifest {
			write(w, 200, map[string]any{"apiVersion": v1.APIVersion, "tenant": a.Service.TenantID, "resource": out})
			return
		}
		var d any
		if err = json.Unmarshal(out.Manifest, &d); err != nil {
			telemetryError(w, err)
			return
		}
		b, err := yaml.Marshal(d)
		if err != nil {
			telemetryError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(b)
	}
}
func (a *Server) bindingCompile(w http.ResponseWriter, r *http.Request) {
	if _, err := parseContextQuery(r); err != nil {
		telemetryError(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	defer func() {
		_ = r.Body.Close()
	}()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var in v1.CompileBindingInput
	if err := decoder.Decode(&in); err != nil {
		telemetryError(w, model.Invalid("invalid compile request"))
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		telemetryError(w, model.Invalid("provide one compile request"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	out, err := a.Service.CompileBinding(ctx, r.PathValue("id"), in)
	if err != nil {
		telemetryError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *Server) dependencyView(w http.ResponseWriter, r *http.Request) {
	q, err := parseContextQuery(r, "expectedRevision")
	if err != nil {
		telemetryError(w, err)
		return
	}
	revision := int64(0)
	if q["expectedRevision"] != "" {
		revision, err = strconv.ParseInt(q["expectedRevision"], 10, 64)
		if err != nil || revision < 1 {
			telemetryError(w, model.Invalid("expectedRevision must be positive"))
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	out, err := a.Service.DependencyView(ctx, r.PathValue("id"), revision)
	if err != nil {
		telemetryError(w, err)
		return
	}
	write(w, 200, out)
}
func (a *Server) mountTelemetry(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/intents/plan", a.intentPlan)
	mux.HandleFunc("POST /api/intents/draft", a.intentDraft)
	mux.HandleFunc("POST /api/intents/backtest", a.intentBacktest)
	mux.HandleFunc("GET /api/intents/capabilities", a.intentCapabilities)
	mux.HandleFunc("GET /api/bindings/{id}/health", a.bindingHealth)
	mux.HandleFunc("GET /api/connections/{connection}/metrics", a.metricNames)
	mux.HandleFunc("GET /api/connections/{connection}/telemetry", a.telemetryDiscover)
	mux.HandleFunc("GET /api/topology/resources", a.topologyList)
	mux.HandleFunc("GET /api/bindings/{id}", a.topologyRead(v1.BindingKind, false))
	mux.HandleFunc("GET /api/bindings/{id}/manifest", a.topologyRead(v1.BindingKind, true))
	mux.HandleFunc("POST /api/bindings/{id}/compile", a.bindingCompile)
	mux.HandleFunc("GET /api/dependency-maps/{id}", a.topologyRead(v1.DependencyMapKind, false))
	mux.HandleFunc("GET /api/dependency-maps/{id}/manifest", a.topologyRead(v1.DependencyMapKind, true))
	mux.HandleFunc("GET /api/dependency-maps/{id}/view", a.dependencyView)
}
