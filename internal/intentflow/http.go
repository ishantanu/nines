package intentflow

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func reply(w http.ResponseWriter, out any, e error, status int) {
	if e != nil {
		fail(w, e)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(out)
}
func fail(w http.ResponseWriter, e error) {
	status := 500
	message := "intent workflow unavailable"
	var invalid *model.ValidationError
	switch {
	case errors.As(e, &invalid):
		status = 400
		message = e.Error()
	case errors.Is(e, ErrNotFound):
		status = 404
		message = ErrNotFound.Error()
	case errors.Is(e, ErrConflict), errors.Is(e, model.ErrRevisionConflict):
		status = 409
		message = ErrConflict.Error()
	case errors.Is(e, ErrLimit):
		status = 429
		message = ErrLimit.Error()
	case errors.Is(e, ErrForbidden):
		status = 403
		message = ErrForbidden.Error()
	}
	reply(w, map[string]string{"error": message}, nil, status)
}
func decode(w http.ResponseWriter, r *http.Request, out any) error {
	typ, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || typ != "application/json" || r.URL.RawQuery != "" {
		return model.Invalid("send one JSON request without query fields")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	defer func() {
		_ = r.Body.Close()
	}()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		return model.Invalid("invalid intent conversation request")
	}
	return nil
}
func Mount(m *http.ServeMux, s *Service) {
	mountKubernetesExport(m, s)
	mountGovernance(m, s)
	m.HandleFunc("POST /api/intent-conversations/{id}/change-set", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ExpectedVersion int64 `json:"expectedVersion"`
		}
		if e := decode(w, r, &in); e != nil {
			fail(w, e)
			return
		}
		out, e := s.PrepareChangeSet(r.Context(), r.PathValue("id"), in.ExpectedVersion)
		reply(w, out, e, 200)
	})
	m.HandleFunc("POST /api/intent-conversations/{id}/change-set/approve", func(w http.ResponseWriter, r *http.Request) {
		var in v1.ReviewIntentChangeSet
		if e := decode(w, r, &in); e != nil {
			fail(w, e)
			return
		}
		out, e := s.ApproveChangeSet(r.Context(), r.PathValue("id"), in)
		reply(w, out, e, 200)
	})
	m.HandleFunc("POST /api/intent-conversations/{id}/change-set/apply", func(w http.ResponseWriter, r *http.Request) {
		var in v1.ReviewIntentChangeSet
		if e := decode(w, r, &in); e != nil {
			fail(w, e)
			return
		}
		out, e := s.ApplyChangeSet(r.Context(), r.PathValue("id"), in)
		reply(w, out, e, 200)
	})
	m.HandleFunc("GET /api/intent-conversations/{id}/change-set/verification", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			fail(w, model.Invalid("query fields are not supported"))
			return
		}
		out, e := s.VerifyChangeSet(r.Context(), r.PathValue("id"))
		reply(w, out, e, 200)
	})
	m.HandleFunc("GET /api/intent-conversations", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for k, v := range q {
			if k != "cursor" || len(v) != 1 {
				fail(w, model.Invalid("only one cursor is supported"))
				return
			}
		}
		out, e := s.List(r.Context(), q.Get("cursor"))
		reply(w, out, e, 200)
	})
	m.HandleFunc("GET /api/intent-conversations/capabilities", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			fail(w, model.Invalid("query fields are not supported"))
			return
		}
		reply(w, s.Capabilities(), nil, 200)
	})
	m.HandleFunc("GET /api/intent-conversations/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			fail(w, model.Invalid("query fields are not supported"))
			return
		}
		out, e := s.Read(r.Context(), r.PathValue("id"))
		reply(w, out, e, 200)
	})
	m.HandleFunc("POST /api/intent-conversations", func(w http.ResponseWriter, r *http.Request) {
		var in v1.CreateIntentConversation
		if e := decode(w, r, &in); e != nil {
			fail(w, e)
			return
		}
		out, e := s.Create(r.Context(), in)
		reply(w, out, e, 201)
	})
	m.HandleFunc("POST /api/intent-conversations/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		var in v1.AppendIntentConversation
		if e := decode(w, r, &in); e != nil {
			fail(w, e)
			return
		}
		out, e := s.Append(r.Context(), r.PathValue("id"), in)
		reply(w, out, e, 200)
	})
	m.HandleFunc("POST /api/intent-conversations/{id}/selection", func(w http.ResponseWriter, r *http.Request) {
		var in v1.ConfirmIntentBinding
		if e := decode(w, r, &in); e != nil {
			fail(w, e)
			return
		}
		out, e := s.Select(r.Context(), r.PathValue("id"), in)
		reply(w, out, e, 200)
	})
	m.HandleFunc("POST /api/intent-conversations/{id}/runs", func(w http.ResponseWriter, r *http.Request) {
		var in v1.CreateIntentFlowRun
		if e := decode(w, r, &in); e != nil {
			fail(w, e)
			return
		}
		out, e := s.Start(r.Context(), r.PathValue("id"), in)
		reply(w, out, e, 202)
	})
	m.HandleFunc("POST /api/intent-conversations/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ExpectedVersion int64 `json:"expectedVersion"`
		}
		if e := decode(w, r, &in); e != nil {
			fail(w, e)
			return
		}
		out, e := s.Cancel(r.Context(), r.PathValue("id"), in.ExpectedVersion)
		reply(w, out, e, 200)
	})
}
