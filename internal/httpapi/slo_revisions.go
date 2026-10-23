package httpapi

import (
	"net/http"
	"strconv"

	"github.com/ishantanu/nines/internal/model"
)

func (a *Server) sloRevisions(w http.ResponseWriter, r *http.Request) {
	limit := 20
	before := int64(0)
	var err error
	if text := r.URL.Query().Get("limit"); text != "" {
		limit, err = strconv.Atoi(text)
		if err != nil {
			a.fail(w, r, model.Invalid("limit must be an integer"))
			return
		}
	}
	if text := r.URL.Query().Get("beforeRevision"); text != "" {
		before, err = strconv.ParseInt(text, 10, 64)
		if err != nil {
			a.fail(w, r, model.Invalid("beforeRevision must be an integer"))
			return
		}
	}
	revisions, err := a.Service.SLORevisions(r.Context(), r.PathValue("id"), before, limit)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	next := int64(0)
	if len(revisions) == limit {
		next = revisions[len(revisions)-1].Revision
	}
	write(w, 200, map[string]any{"apiVersion": model.APIVersion, "sloId": r.PathValue("id"), "revisions": revisions, "nextBeforeRevision": next})
}
func (a *Server) sloRevision(w http.ResponseWriter, r *http.Request) {
	revision, err := strconv.ParseInt(r.PathValue("revision"), 10, 64)
	if err != nil || revision < 1 {
		a.fail(w, r, model.Invalid("revision must be positive"))
		return
	}
	archive, err := a.Service.SLORevision(r.Context(), r.PathValue("id"), revision)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, archive.Summary)
}
