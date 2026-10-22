package replay

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func Mount(m *http.ServeMux, s *Service) {
	m.HandleFunc("GET /api/session-replays", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for k, v := range q {
			if len(v) != 1 || (k != "resourceKind" && k != "resourceId" && k != "archived") {
				fail(w, model.Invalid("unknown or duplicate filter"))
				return
			}
		}
		if q.Get("archived") != "" && q.Get("archived") != "true" {
			fail(w, model.Invalid("archived must be true"))
			return
		}
		out, err := s.List(r.Context(), q.Get("resourceKind"), q.Get("resourceId"), q.Get("archived") == "true")
		reply(w, out, err, 200)
	})
	m.HandleFunc("POST /api/session-replays", func(w http.ResponseWriter, r *http.Request) {
		var in v1.CreateSessionReplay
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		out, err := s.Create(r.Context(), in)
		reply(w, out, err, 201)
	})
	m.HandleFunc("GET /api/session-replays/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			fail(w, model.Invalid("query fields are not supported"))
			return
		}
		out, err := s.Read(r.Context(), r.PathValue("id"))
		reply(w, out, err, 200)
	})
	m.HandleFunc("GET /api/session-replays/{id}/events", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			fail(w, model.Invalid("query fields are not supported"))
			return
		}
		out, err := s.Events(r.Context(), r.PathValue("id"))
		reply(w, out, err, 200)
	})
	m.HandleFunc("POST /api/session-replays/{id}/links", func(w http.ResponseWriter, r *http.Request) {
		var in v1.UpdateReplayLinks
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		out, err := s.Links(r.Context(), r.PathValue("id"), in)
		reply(w, out, err, 200)
	})
	m.HandleFunc("POST /api/session-replays/{id}/archive", func(w http.ResponseWriter, r *http.Request) {
		var in v1.ArchiveReplay
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		out, err := s.Archive(r.Context(), r.PathValue("id"), in)
		reply(w, out, err, 200)
	})
}
func decode(w http.ResponseWriter, r *http.Request, out any) error {
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" || r.URL.RawQuery != "" {
		return model.Invalid("send one JSON document with no query fields")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 3<<20)
	defer func() {
		_ = r.Body.Close()
	}()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return model.Invalid("invalid session replay input")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return model.Invalid("send exactly one document")
	}
	return nil
}
func reply(w http.ResponseWriter, out any, err error, status int) {
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(out)
}
func fail(w http.ResponseWriter, err error) {
	status := 500
	msg := "session replay unavailable"
	var validation *model.ValidationError
	switch {
	case errors.As(err, &validation):
		status = 400
		msg = err.Error()
	case errors.Is(err, ErrNotFound), errors.Is(err, model.ErrNotFound), errors.Is(err, model.ErrJourneyNotFound):
		status = 404
		msg = "session or linked resource not found"
	case errors.Is(err, ErrConflict):
		status = 409
		msg = err.Error()
	case errors.Is(err, ErrLimit):
		status = 429
		msg = err.Error()
	}
	reply(w, map[string]string{"error": msg}, nil, status)
}
