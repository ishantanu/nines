package httpapi

import (
	"net/http"
	"strconv"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/tenancy"
)

func incidentActor(r *http.Request) string {
	p, ok := tenancy.FromContext(r.Context())
	if !ok {
		return ""
	}
	if p.Name != "" {
		return p.Name
	}
	return p.Subject
}

func (a *Server) incidentList(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			a.fail(w, r, model.Invalid("limit must be 1 to 200"))
			return
		}
		limit = n
	}
	list, err := a.Service.ListIncidents(r.Context(), limit)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, map[string]any{"incidents": list})
}

func (a *Server) incidentGet(w http.ResponseWriter, r *http.Request) {
	v, err := a.Service.Incident(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}

func (a *Server) incidentDeclare(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string   `json:"title"`
		SLOs  []string `json:"slos"`
	}
	if err := decode(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	i, err := a.Service.DeclareIncident(r.Context(), incidentActor(r), req.Title, req.SLOs)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := a.Service.Incident(r.Context(), i.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}

func (a *Server) incidentTransition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		State            string `json:"state"`
		Note             string `json:"note"`
		ExpectedRevision *int64 `json:"expectedRevision"`
	}
	if err := decode(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	if req.ExpectedRevision == nil {
		a.fail(w, r, model.Invalid("expectedRevision is required: use the incident's current revision"))
		return
	}
	i, err := a.Service.TransitionIncident(r.Context(), r.PathValue("id"), *req.ExpectedRevision, req.State, incidentActor(r), req.Note)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := a.Service.Incident(r.Context(), i.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
