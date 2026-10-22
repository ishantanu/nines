package investigation

import (
	"net/http"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/reviewskills"
)

func mountRuns(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/review-skills", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			fail(w, model.Invalid("query fields are not supported"))
			return
		}
		respond(w, v1.ReviewSkillList{APIVersion: v1.APIVersion, Tenant: s.Platform.TenantID, Items: reviewskills.Catalog()}, nil, 200)
	})
	mux.HandleFunc("GET /api/review-skills/{id}/versions/{version}", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			fail(w, model.Invalid("query fields are not supported"))
			return
		}
		for _, skill := range reviewskills.Catalog() {
			if skill.Metadata.Name == r.PathValue("id") && skill.Metadata.Version == r.PathValue("version") {
				respond(w, v1.ReviewSkillDocument{APIVersion: v1.APIVersion, Tenant: s.Platform.TenantID, Item: skill}, nil, 200)
				return
			}
		}
		fail(w, ErrNotFound)
	})

	mux.HandleFunc("GET /api/investigations/{id}/runs/{run}/events", s.streamRun)
	mux.HandleFunc("GET /api/investigations/capabilities", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			fail(w, model.Invalid("query fields are not supported"))
			return
		}
		respond(w, s.Capabilities(), nil, 200)
	})
	mux.HandleFunc("GET /api/investigations/{id}/runs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			fail(w, model.Invalid("query fields are not supported"))
			return
		}
		out, err := s.ListRuns(r.Context(), r.PathValue("id"))
		respond(w, out, err, 200)
	})
	mux.HandleFunc("POST /api/investigations/{id}/runs", func(w http.ResponseWriter, r *http.Request) {
		var in v1.CreateInvestigationRun
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		out, err := s.CreateRun(r.Context(), r.PathValue("id"), in)
		respond(w, out, err, 202)
	})
	mux.HandleFunc("GET /api/investigations/{id}/runs/{run}", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			fail(w, model.Invalid("query fields are not supported"))
			return
		}
		out, err := s.ReadRun(r.Context(), r.PathValue("id"), r.PathValue("run"))
		respond(w, out, err, 200)
	})
	mux.HandleFunc("POST /api/investigations/{id}/runs/{run}/cancel", func(w http.ResponseWriter, r *http.Request) {
		var in v1.CancelInvestigationRun
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		out, err := s.CancelRun(r.Context(), r.PathValue("id"), r.PathValue("run"), in)
		respond(w, out, err, 200)
	})
}
