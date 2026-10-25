package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ishantanu/nines/internal/intentflow"
	"github.com/ishantanu/nines/internal/investigation"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
	"github.com/ishantanu/nines/internal/replay"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/workflow"
)

type Server struct {
	Workspace Workspace
	Service   *service.Service
	Log       *slog.Logger
	requests  atomic.Uint64
}

func New(s *service.Service, token string, log *slog.Logger) http.Handler {
	id := s.TenantID
	if id == "" {
		id = "default"
	}
	name := s.TenantName
	if name == "" {
		name = "Default workspace"
	}
	workspace := Workspace{ID: id, Name: name, Service: s, Anonymous: token == ""}
	if token != "" {
		workspace.Grants = []tenancy.Grant{{Token: token, Role: "owner"}}
	}
	h, err := NewMulti([]Workspace{workspace}, log)
	if err != nil {
		panic(err)
	}
	return h
}
func newSingle(s *service.Service, log *slog.Logger, workflows *workflow.Service, workspace Workspace) http.Handler {
	a := &Server{Service: s, Log: log, Workspace: workspace}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.Store.Ping(ctx); err != nil {
			write(w, 503, map[string]string{"status": "unavailable"})
			return
		}
		write(w, 200, map[string]string{"status": "ok"})
	})
	mux.Handle("/", frontend())
	api := http.NewServeMux()
	if workspace.IntentConversations != nil {
		intentflow.Mount(api, workspace.IntentConversations)
	}
	if workspace.Replays != nil {
		replay.Mount(api, workspace.Replays)
	}
	if workspace.Investigations != nil {
		investigation.Mount(api, workspace.Investigations)
	}
	if workflows != nil {
		workflow.Mount(api, workflows)
	}
	api.HandleFunc("GET /api/workspace", func(w http.ResponseWriter, r *http.Request) {
		mode := "workspace"
		if s.Demo {
			mode = "demo"
		}
		role := "owner"
		if p, ok := tenancy.FromContext(r.Context()); ok {
			role = p.Role
		}
		count := 0
		kinds := map[string]bool{}
		if s.Collector != nil {
			count++
			kinds[s.SourceKind] = true
		}
		for _, source := range s.Sources {
			if source.Collector != nil {
				count++
				kinds[source.Kind] = true
			}
		}
		if workspace.ManagedSources != nil {
			for _, connection := range workspace.ManagedSources.List() {
				count++
				kinds[connection["kind"].(string)] = true
			}
		}
		kind := "manual"
		for sourceKind := range kinds {
			kind = sourceKind
		}
		if len(kinds) > 1 {
			kind = "multiple"
		}
		write(w, 200, map[string]any{"mode": mode, "prometheusConfigured": count > 0, "journeysConfigured": s.Journeys != nil, "tenant": map[string]string{"id": s.TenantID, "name": s.TenantName}, "source": map[string]any{"kind": kind, "configured": count > 0, "connectionCount": count}, "role": role, "workflowsConfigured": workflows != nil})
	})
	api.HandleFunc("GET /api/access", a.access)
	api.HandleFunc("GET /api/connections", a.connections)
	api.HandleFunc("POST /api/connections", a.connectionSave)
	api.HandleFunc("PUT /api/connections/{connection}", a.connectionSave)
	api.HandleFunc("DELETE /api/connections/{connection}", a.connectionDelete)
	a.mountTelemetry(api)
	api.HandleFunc("POST /api/connections/{connection}/test", a.connectionTest)

	api.HandleFunc("GET /api/slos/{id}/integration", a.integrationStatus)
	api.HandleFunc("GET /api/slos/{id}/rules", a.rulePlan)
	api.HandleFunc("POST /api/slos/{id}/rules/plan", a.rulePlan)
	api.HandleFunc("POST /api/slos/{id}/rules/apply", a.ruleApply)
	api.HandleFunc("POST /api/slos/{id}/rules/automation", a.ruleAutomation)
	api.HandleFunc("POST /api/slos/{id}/rules/withdraw", a.ruleWithdraw)

	api.HandleFunc("GET /api/resources/search", a.resourceSearch)
	api.HandleFunc("GET /api/slos/{id}/context", func(w http.ResponseWriter, r *http.Request) { a.resourceContext(w, r, false) })
	api.HandleFunc("GET /api/journeys/{id}/context", func(w http.ResponseWriter, r *http.Request) { a.resourceContext(w, r, true) })
	api.HandleFunc("POST /api/resources/validate", a.definitionValidate)
	api.HandleFunc("POST /api/slos/compile", a.compileMeasurement)
	api.HandleFunc("POST /api/resources/plan", a.definitionPlan)
	api.HandleFunc("POST /api/resources/apply", a.definitionApply)
	api.HandleFunc("GET /api/slos/{id}/manifest", a.sloManifest)
	api.HandleFunc("GET /api/slos/{id}/revisions", a.sloRevisions)
	api.HandleFunc("GET /api/slos/{id}/revisions/{revision}", a.sloRevision)
	api.HandleFunc("GET /api/slos", a.list)
	api.HandleFunc("GET /api/slos/{id}", a.sloGet)
	api.HandleFunc("POST /api/slos", a.create)
	api.HandleFunc("GET /api/slos/{id}/status", a.status)
	api.HandleFunc("GET /api/slos/{id}/quality", a.measurementQuality)
	api.HandleFunc("GET /api/slos/{id}/budget", a.budgetDecision)
	api.HandleFunc("GET /api/slos/{id}/samples", a.samples)
	api.HandleFunc("GET /api/slos/{id}/history", a.history)
	api.HandleFunc("POST /api/slos/{id}/samples", a.ingest)
	api.HandleFunc("POST /api/slos/{id}/collect", a.collect)
	api.HandleFunc("GET /api/events", a.events)
	if s.Journeys != nil {
		api.HandleFunc("GET /api/journeys", a.journeyList)
		api.HandleFunc("POST /api/journeys/plan", a.journeyPlan)
		api.HandleFunc("POST /api/journeys/apply", a.journeyApply)
		api.HandleFunc("GET /api/journeys/{id}", a.journeyGet)
		api.HandleFunc("GET /api/journeys/{id}/history", a.journeyHistory)
		api.HandleFunc("GET /api/journeys/{id}/manifest", a.journeyManifest)
		api.HandleFunc("GET /api/journeys/{id}/runs", a.journeyRuns)
		api.HandleFunc("POST /api/journeys/{id}/runs", a.journeyRun)
		api.HandleFunc("GET /api/slos/{id}/journeys", a.sloJourneys)
	}
	mux.Handle("/api/", api)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.requests.Add(1)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		defer func() {
			if x := recover(); x != nil {
				log.ErrorContext(r.Context(), "request panic", "error", x)
				write(w, 500, map[string]string{"error": "internal error"})
			}
		}()
		defer func() { observability.Route(r.Context(), r.Pattern) }()
		mux.ServeHTTP(w, r)
	})
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (a *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var validation *model.ValidationError
	switch {
	case errors.As(err, &validation):
		write(w, 400, map[string]string{"error": validation.Error()})
	case errors.Is(err, model.ErrNotFound), errors.Is(err, model.ErrJourneyNotFound):
		write(w, 404, map[string]string{"error": err.Error()})
	case errors.Is(err, model.ErrConflict), errors.Is(err, model.ErrRevisionConflict), errors.Is(err, model.ErrRunConflict):
		write(w, 409, map[string]string{"error": err.Error()})
	case errors.Is(err, model.ErrUnavailable):
		write(w, 503, map[string]string{"error": err.Error()})
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		write(w, 504, map[string]string{"error": "request canceled or timed out"})
	default:
		a.Log.ErrorContext(r.Context(), "API operation failed", "error", err)
		write(w, 500, map[string]string{"error": "operation failed"})
	}
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		return model.Invalid("Content-Type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	defer func() {
		_ = r.Body.Close()
	}()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err = d.Decode(v); err != nil {
		return model.Invalid("invalid JSON body: %v", err)
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return model.Invalid("body must contain exactly one JSON value")
	}
	return nil
}
func (a *Server) list(w http.ResponseWriter, r *http.Request) {
	configs, err := a.Service.Store.List(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out := make([]service.SLOView, 0, len(configs))
	for _, s := range configs {
		view, err := a.Service.ReadSLO(r.Context(), s.ID)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		out = append(out, view)
	}
	write(w, 200, out)
}

func (a *Server) sloGet(w http.ResponseWriter, r *http.Request) {
	view, err := a.Service.ReadSLO(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, view)
}
func (a *Server) create(w http.ResponseWriter, r *http.Request) {
	var s model.SLO
	if err := decode(w, r, &s); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.Service.Create(r.Context(), s); err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/slos/"+s.ID+"/status")
	write(w, 201, map[string]string{"id": s.ID})
}
func (a *Server) status(w http.ResponseWriter, r *http.Request) {
	var at time.Time
	var err error
	if text := r.URL.Query().Get("at"); text != "" {
		at, err = time.Parse(time.RFC3339, text)
		if err != nil {
			a.fail(w, r, model.Invalid("at must be RFC3339"))
			return
		}
	}
	x, err := a.Service.Status(r.Context(), r.PathValue("id"), at)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, x)
}
func (a *Server) samples(w http.ResponseWriter, r *http.Request) {
	x, err := a.Service.Samples(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, x)
}
func (a *Server) ingest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Samples []model.Point `json:"samples"`
	}
	if err := decode(w, r, &body); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.Service.Ingest(r.Context(), r.PathValue("id"), body.Samples); err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, map[string]int{"accepted": len(body.Samples)})
}
func (a *Server) collect(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Recent bool      `json:"recent"`
		From   time.Time `json:"from"`
		To     time.Time `json:"to"`
	}
	if err := decode(w, r, &body); err != nil {
		a.fail(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if body.Recent && (!body.From.IsZero() || !body.To.IsZero()) {
		a.fail(w, r, model.Invalid("recent cannot be combined with from or to"))
		return
	}
	var count int
	var err error
	if body.Recent {
		count, err = a.Service.CollectRecent(ctx, r.PathValue("id"))
	} else {
		count, err = a.Service.Collect(ctx, r.PathValue("id"), body.From, body.To)
	}
	if err != nil {
		if strings.HasPrefix(err.Error(), "collection failed:") {
			a.Log.ErrorContext(r.Context(), "source collection failed", "slo", r.PathValue("id"), "error", err)
			write(w, 502, map[string]string{"error": "Prometheus collection failed; no samples were committed"})
			return
		}
		a.fail(w, r, err)
		return
	}
	write(w, 200, map[string]int{"collected": count})
}
func (a *Server) events(w http.ResponseWriter, r *http.Request) {
	x, err := a.Service.Store.Deliveries(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, x)
}
