package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/ishantanu/nines/internal/engine"
	"github.com/ishantanu/nines/internal/model"
)

func (a *Server) history(w http.ResponseWriter, r *http.Request) {
	var from, to time.Time
	var err error
	if text := r.URL.Query().Get("from"); text != "" {
		from, err = time.Parse(time.RFC3339, text)
		if err != nil {
			a.fail(w, r, model.Invalid("from must be RFC3339"))
			return
		}
	}
	if text := r.URL.Query().Get("to"); text != "" {
		to, err = time.Parse(time.RFC3339, text)
		if err != nil {
			a.fail(w, r, model.Invalid("to must be RFC3339"))
			return
		}
	}
	var resolution int64
	if text := r.URL.Query().Get("resolutionSeconds"); text != "" {
		resolution, err = strconv.ParseInt(text, 10, 64)
		if err != nil || resolution <= 0 {
			a.fail(w, r, model.Invalid("resolutionSeconds must be positive"))
			return
		}
	}
	var span []int64
	if text := r.URL.Query().Get("rangeSeconds"); text != "" {
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil || n <= 0 || n > 62*86400 || !from.IsZero() {
			a.fail(w, r, model.Invalid("rangeSeconds must be positive and cannot be combined with from"))
			return
		}
		span = []int64{n}
	}

	var result engine.History
	if text := r.URL.Query().Get("revision"); text != "" {
		revision, parseErr := strconv.ParseInt(text, 10, 64)
		if parseErr != nil || revision < 1 {
			a.fail(w, r, model.Invalid("revision must be positive"))
			return
		}
		result, err = a.Service.HistoryForRevision(r.Context(), r.PathValue("id"), revision, from, to, resolution, r.URL.Query().Get("policy"), span...)
	} else {
		result, err = a.Service.HistoryForPolicy(r.Context(), r.PathValue("id"), from, to, resolution, r.URL.Query().Get("policy"), span...)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
