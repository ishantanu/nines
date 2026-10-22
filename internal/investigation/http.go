package investigation

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	rm "github.com/ishantanu/nines/internal/reviewmodel"
)

func Mount(mux *http.ServeMux, s *Service) {
	mountRuns(mux, s)
	mux.HandleFunc("GET /api/investigations", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for k := range q {
			if k != "cursor" && k != "limit" {
				fail(w, model.Invalid("unknown query field"))
				return
			}
			if len(q[k]) != 1 {
				fail(w, model.Invalid("duplicate query field"))
				return
			}
		}
		limit := 25
		if q.Has("limit") {
			n, e := strconv.Atoi(q.Get("limit"))
			if e != nil {
				fail(w, model.Invalid("invalid limit"))
				return
			}
			limit = n
		}
		v, err := s.List(r.Context(), q.Get("cursor"), limit)
		respond(w, v, err, 200)
	})
	mux.HandleFunc("GET /api/investigations/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			fail(w, model.Invalid("query fields are not supported"))
			return
		}
		v, err := s.Read(r.Context(), r.PathValue("id"))
		respond(w, v, err, 200)
	})
	mux.HandleFunc("POST /api/investigations", func(w http.ResponseWriter, r *http.Request) {
		var in v1.CreateInvestigation
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		v, err := s.Create(ctx, in)
		respond(w, v, err, 201)
	})
	mux.HandleFunc("POST /api/investigations/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		var in v1.AppendInvestigationMessage
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		v, err := s.Message(r.Context(), r.PathValue("id"), in)
		respond(w, v, err, 200)
	})
	mux.HandleFunc("POST /api/investigations/{id}/evidence", func(w http.ResponseWriter, r *http.Request) {
		var in v1.CaptureInvestigationEvidence
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		v, err := s.Capture(ctx, r.PathValue("id"), in)
		respond(w, v, err, 200)
	})
}
func decode(w http.ResponseWriter, r *http.Request, out any) error {
	if r.URL.RawQuery != "" {
		return model.Invalid("query fields are not supported")
	}
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		return model.Invalid("Content-Type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	defer func() {
		_ = r.Body.Close()
	}()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return model.Invalid("invalid investigation request")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return model.Invalid("provide exactly one JSON document")
	}
	return nil
}
func respond(w http.ResponseWriter, value any, err error, status int) {
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, err error) {
	status := 500
	code := "internal_error"
	message := "investigation operation failed"
	var validation *model.ValidationError
	var modelError *rm.Error
	switch {
	case errors.As(err, &modelError):
		status = 503
		code = modelError.Code
		message = "a review model is not configured or available for this tenant"
	case errors.Is(err, ErrForbidden):
		status = 403
		code = "forbidden"
		message = "only the run initiator or an owner may cancel this run"
	case errors.As(err, &validation):
		status = 400
		code = "invalid_request"
		message = err.Error()
	case errors.Is(err, ErrNotFound), errors.Is(err, model.ErrNotFound), errors.Is(err, model.ErrJourneyNotFound):
		status = 404
		code = "not_found"
		message = "investigation or scoped resource not found"
	case errors.Is(err, model.ErrRevisionConflict):
		status = 409
		code = "scope_revision_conflict"
		message = "scoped resource revision changed; start a new investigation for its current revision"
	case errors.Is(err, ErrConflict):
		status = 409
		code = "revision_conflict"
		message = "investigation or resource changed; reload before retrying"
	case errors.Is(err, model.ErrUnavailable):
		status = 503
		code = "unavailable"
		message = "scoped evidence is unavailable"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": v1.APIVersion, "error": map[string]string{"code": code, "message": message}})
}
