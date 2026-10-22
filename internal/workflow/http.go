package workflow

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/skills"
)

// Mount installs handlers on the tenant's inner mux. Authentication, membership,
// role checks and same-origin protections belong to the outer API server.
func Mount(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/skills", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, skills.Catalog()) })
	mux.HandleFunc("GET /api/skills/{id}", func(w http.ResponseWriter, r *http.Request) {
		for _, skill := range skills.Catalog() {
			if skill.ID == r.PathValue("id") {
				respond(w, 200, skill)
				return
			}
		}
		fail(w, ErrNotFound)
	})
	mux.HandleFunc("GET /api/workflows", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.List(r.Context())
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, v)
	})
	mux.HandleFunc("GET /api/workflows/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, v)
	})
	mux.HandleFunc("POST /api/workflows", func(w http.ResponseWriter, r *http.Request) {
		var req CreateRequest
		if err := decode(w, r, &req); err != nil {
			fail(w, err)
			return
		}
		v, err := s.Create(r.Context(), req)
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 201, v)
	})
	mux.HandleFunc("POST /api/workflows/{id}/approve", func(w http.ResponseWriter, r *http.Request) {
		var req ApprovalRequest
		if err := decode(w, r, &req); err != nil {
			fail(w, err)
			return
		}
		v, err := s.Approve(r.Context(), r.PathValue("id"), req)
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, v)
	})
	mux.HandleFunc("POST /api/workflows/{id}/reject", func(w http.ResponseWriter, r *http.Request) {
		var req RejectionRequest
		if err := decode(w, r, &req); err != nil {
			fail(w, err)
			return
		}
		v, err := s.Reject(r.Context(), r.PathValue("id"), req)
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, v)
	})
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
	if err := d.Decode(v); err != nil {
		return model.Invalid("invalid workflow JSON: %v", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return model.Invalid("provide exactly one JSON document")
	}
	return nil
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, err error) {
	status := 500
	message := "internal server error"
	var validation *model.ValidationError
	switch {
	case errors.As(err, &validation):
		status = 400
		message = err.Error()
	case errors.Is(err, ErrNotFound), errors.Is(err, model.ErrNotFound), errors.Is(err, model.ErrJourneyNotFound):
		status = 404
		message = err.Error()
	case errors.Is(err, ErrConflict), errors.Is(err, model.ErrRevisionConflict):
		status = 409
		message = err.Error()
	case errors.Is(err, model.ErrUnavailable):
		status = 503
		message = err.Error()
	}
	respond(w, status, map[string]string{"error": message})
}
