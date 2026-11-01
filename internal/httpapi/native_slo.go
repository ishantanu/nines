package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

func (a *Server) nativeSLOResults(w http.ResponseWriter, r *http.Request) {
	if a.Workspace.Rules == nil {
		a.fail(w, r, model.ErrUnavailable)
		return
	}
	values, queryErr := urlValues(r)
	if queryErr != nil {
		a.fail(w, r, queryErr)
		return
	}
	for key, values := range values {
		if (key != "from" && key != "to") || len(values) != 1 {
			a.fail(w, r, model.Invalid("native results accept only one from and to value"))
			return
		}
	}
	from, e := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	if e != nil {
		a.fail(w, r, model.Invalid("from must be RFC3339"))
		return
	}
	to, e := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if e != nil || !to.After(from) || to.After(a.Service.Now().UTC()) || to.Sub(from) > 31*24*time.Hour || from.Unix() < 0 || from.Nanosecond() != 0 || to.Nanosecond() != 0 {
		a.fail(w, r, model.Invalid("provide a past whole-second from/to range of at most 31 days"))
		return
	}
	record, e := a.Service.Store.Read(r.Context(), r.PathValue("id"))
	if e != nil {
		a.fail(w, r, e)
		return
	}
	fingerprint := a.Service.SourceFingerprint(record.SLO.Source)
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	out, e := a.Workspace.Rules.NativeResults(ctx, record.SLO, from.UTC(), to.UTC())
	if e != nil {
		a.fail(w, r, e)
		return
	}
	current, e := a.Service.Store.Read(ctx, record.SLO.ID)
	if e != nil || model.ConfigRevision(current.SLO) != model.ConfigRevision(record.SLO) || a.Service.SourceFingerprint(record.SLO.Source) != fingerprint {
		a.fail(w, r, model.ErrRevisionConflict)
		return
	}
	write(w, 200, out)
}
func (a *Server) nativeSLOCatalog(w http.ResponseWriter, r *http.Request) {
	if a.Workspace.Rules == nil {
		a.fail(w, r, model.ErrUnavailable)
		return
	}
	values, queryErr := urlValues(r)
	if queryErr != nil {
		a.fail(w, r, queryErr)
		return
	}
	if len(values) > 0 {
		a.fail(w, r, model.Invalid("native catalog does not accept routing overrides"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	out, e := a.Workspace.Rules.NativeCatalog(ctx, r.PathValue("connection"))
	if e != nil {
		a.fail(w, r, e)
		return
	}
	write(w, 200, out)
}
func (a *Server) nativeSLODraft(w http.ResponseWriter, r *http.Request) {
	if a.Workspace.Rules == nil {
		a.fail(w, r, model.ErrUnavailable)
		return
	}
	parsed, queryErr := urlValues(r)
	if queryErr != nil {
		a.fail(w, r, queryErr)
		return
	}
	q := url.Values(parsed)
	for k, values := range q {
		if (k != "nativeID" && k != "name" && k != "project" && k != "service") || len(values) != 1 {
			a.fail(w, r, model.Invalid("draft accepts only nativeID, name, project and service"))
			return
		}
	}
	for _, k := range []string{"nativeID", "name", "project", "service"} {
		if q.Get(k) == "" || len(q.Get(k)) > 512 {
			a.fail(w, r, model.Invalid("nativeID, name, project and service are required"))
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	out, e := a.Workspace.Rules.NativeDraft(ctx, r.PathValue("connection"), q.Get("nativeID"), q.Get("name"), q.Get("project"), q.Get("service"))
	if e != nil {
		a.fail(w, r, e)
		return
	}
	write(w, 200, out)
}
