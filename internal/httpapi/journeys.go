package httpapi

import (
	"net/http"
	"strconv"

	"github.com/ishantanu/nines/internal/model"
)

type journeyRequest struct {
	Manifest         string `json:"manifest"`
	ExpectedRevision *int64 `json:"expectedRevision,omitempty"`
}

func (a *Server) journeyPlan(w http.ResponseWriter, r *http.Request) {
	var req journeyRequest
	if err := decode(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	j, err := a.parseJourney(w, r, req.Manifest)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	p, err := a.Service.PlanJourney(r.Context(), j)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, p)
}
func (a *Server) journeyApply(w http.ResponseWriter, r *http.Request) {
	var req journeyRequest
	if err := decode(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	if req.ExpectedRevision == nil {
		a.fail(w, r, model.Invalid("expectedRevision is required: use 0 to create or the current revision to update"))
		return
	}
	j, err := a.parseJourney(w, r, req.Manifest)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	x, err := a.Service.ApplyJourney(r.Context(), j, *req.ExpectedRevision)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("ETag", `"`+strconv.FormatInt(x.Revision, 10)+`"`)
	write(w, 200, x)
}
func (a *Server) journeyList(w http.ResponseWriter, r *http.Request) {
	x, err := a.Service.ListJourneys(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, x)
}
func (a *Server) journeyGet(w http.ResponseWriter, r *http.Request) {
	rev, err := a.Service.Journeys.JourneyRevision(r.Context(), r.PathValue("id"), 0)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	x, err := a.Service.JourneyView(r.Context(), rev)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, x)
}
func (a *Server) journeyHistory(w http.ResponseWriter, r *http.Request) {
	x, err := a.Service.Journeys.JourneyHistory(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, x)
}
func (a *Server) journeyManifest(w http.ResponseWriter, r *http.Request) {
	var rev int64
	if text := r.URL.Query().Get("revision"); text != "" {
		var err error
		rev, err = strconv.ParseInt(text, 10, 64)
		if err != nil || rev < 1 {
			a.fail(w, r, model.Invalid("revision must be positive"))
			return
		}
	}
	x, err := a.Service.Journeys.JourneyRevision(r.Context(), r.PathValue("id"), rev)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	journey := x.Manifest
	b, err := a.Service.ExportYAML(r.Context(), model.Definition{Journey: &journey}, r.URL.Query().Get("apiVersion"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(b)
}
func (a *Server) journeyRuns(w http.ResponseWriter, r *http.Request) {
	x, err := a.Service.Journeys.Runs(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, x)
}
func (a *Server) journeyRun(w http.ResponseWriter, r *http.Request) {
	var e model.RunEvidence
	if err := decode(w, r, &e); err != nil {
		a.fail(w, r, err)
		return
	}
	x, err := a.Service.RecordRun(r.Context(), r.PathValue("id"), e)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, x)
}
func (a *Server) sloJourneys(w http.ResponseWriter, r *http.Request) {
	if _, err := a.Service.Store.Read(r.Context(), r.PathValue("id")); err != nil {
		a.fail(w, r, err)
		return
	}
	all, err := a.Service.Journeys.Journeys(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out := []model.JourneyRevision{}
	for _, j := range all {
		for _, l := range j.Manifest.Spec.SLOs {
			if l.ID == r.PathValue("id") {
				out = append(out, j)
				break
			}
		}
	}
	write(w, 200, out)
}

// parseJourney reads a v1alpha1 or v1beta1 journey, resolving v1beta1 link
// dimensions from the tenant's SLOs.
func (a *Server) parseJourney(w http.ResponseWriter, r *http.Request, manifest string) (model.Journey, error) {
	d, err := model.ParseDefinition([]byte(manifest))
	warnDeprecated(w, d)
	if err != nil {
		return model.Journey{}, err
	}
	if d.Journey == nil {
		return model.Journey{}, model.Invalid("expected kind CriticalUserJourney")
	}
	if d, err = a.Service.ResolveJourney(r.Context(), d); err != nil {
		return model.Journey{}, err
	}
	return *d.Journey, nil
}
