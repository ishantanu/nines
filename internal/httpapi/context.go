package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func contextError(w http.ResponseWriter, err error) {
	problem := v1.ContextError{Code: "internal_error", Message: "Context could not be read", Recovery: "Retry the bounded read; contact an operator if it persists"}
	status := 500
	var validation *model.ValidationError
	switch {
	case errors.As(err, &validation):
		status = 400
		problem.Code = "invalid_request"
		problem.Message = validation.Error()
		problem.Recovery = "Correct the filters, cursor, time or page size and retry"
	case errors.Is(err, model.ErrNotFound), errors.Is(err, model.ErrJourneyNotFound):
		status = 404
		problem.Code = "resource_not_found"
		problem.Message = "Resource not found in this workspace"
		problem.Recovery = "Search resources in the configured workspace and use an exact ID"
	case errors.Is(err, model.ErrRevisionConflict):
		status = 409
		problem.Code = "revision_conflict"
		problem.Message = "Resource revision changed"
		problem.Recovery = "Read the latest context and review its revision before retrying"
	case errors.Is(err, model.ErrUnavailable):
		status = 503
		problem.Code = "integration_unavailable"
		problem.Message = "Required read integration is not configured"
		problem.Recovery = "Ask an operator to configure this workspace integration"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status = 504
		problem.Code = "deadline_exceeded"
		problem.Message = "Bounded read timed out"
		problem.Retryable = true
		problem.Recovery = "Retry with a smaller page or narrower filters"
	}
	write(w, status, map[string]any{"apiVersion": v1.APIVersion, "error": problem})
}
func parseContextQuery(r *http.Request, allowed ...string) (map[string]string, error) {
	values, err := urlValues(r)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for k, v := range values {
		found := false
		for _, key := range allowed {
			if key == k {
				found = true
				break
			}
		}
		if !found || len(v) != 1 {
			return nil, model.Invalid("unknown or repeated query parameter")
		}
		out[k] = v[0]
	}
	return out, nil
}
func urlValues(r *http.Request) (map[string][]string, error) {
	if len(r.URL.RawQuery) > 4096 {
		return nil, model.Invalid("query exceeds 4096 bytes")
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, model.Invalid("invalid query encoding")
	}
	return q, nil
}
func pageLimit(text string) (int, error) {
	if text == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < 1 {
		return 0, model.Invalid("limit must be positive")
	}
	return n, nil
}
func (a *Server) resourceSearch(w http.ResponseWriter, r *http.Request) {
	q, err := parseContextQuery(r, "kind", "project", "query", "limit", "cursor", "linkedSLO")
	if err != nil {
		contextError(w, err)
		return
	}
	limit, err := pageLimit(q["limit"])
	if err != nil {
		contextError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	result, err := a.Service.SearchResources(ctx, v1.SearchOptions{Kind: q["kind"], Project: q["project"], Query: q["query"], Limit: limit, Cursor: q["cursor"], LinkedSLO: q["linkedSLO"]})
	if err != nil {
		contextError(w, err)
		return
	}
	write(w, 200, result)
}
func (a *Server) resourceContext(w http.ResponseWriter, r *http.Request, journey bool) {
	allowed := []string{"at", "expectedRevision"}
	if journey {
		allowed = append(allowed, "limit", "cursor")
	}
	q, err := parseContextQuery(r, allowed...)
	if err != nil {
		contextError(w, err)
		return
	}
	in := v1.ContextOptions{Cursor: q["cursor"]}
	if q["at"] != "" {
		in.At, err = time.Parse(time.RFC3339Nano, q["at"])
		if err != nil {
			contextError(w, model.Invalid("at must be RFC3339"))
			return
		}
	}
	if q["expectedRevision"] != "" {
		in.ExpectedRevision, err = strconv.ParseInt(q["expectedRevision"], 10, 64)
		if err != nil || in.ExpectedRevision < 1 {
			contextError(w, model.Invalid("expectedRevision must be positive"))
			return
		}
	}
	in.Limit, err = pageLimit(q["limit"])
	if err != nil {
		contextError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var result any
	if journey {
		result, err = a.Service.JourneyContext(ctx, r.PathValue("id"), in)
	} else {
		result, err = a.Service.SLOContext(ctx, r.PathValue("id"), in)
	}
	if err != nil {
		contextError(w, err)
		return
	}
	write(w, 200, result)
}
