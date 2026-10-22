package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ishantanu/nines/internal/datasources"
	"github.com/ishantanu/nines/internal/model"
)

func (a *Server) connectionSave(w http.ResponseWriter, r *http.Request) {
	if a.Workspace.ManagedSources == nil {
		a.fail(w, r, model.ErrUnavailable)
		return
	}
	var in datasources.Input
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if name := r.PathValue("connection"); name != "" && name != in.Name {
		a.fail(w, r, model.Invalid("connection name cannot change"))
		return
	}
	a.Service.DefinitionMu.Lock()
	defer a.Service.DefinitionMu.Unlock()
	out, err := a.Workspace.ManagedSources.Save(r.Context(), in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, out)
}
func (a *Server) connectionDelete(w http.ResponseWriter, r *http.Request) {
	if a.Workspace.ManagedSources == nil {
		a.fail(w, r, model.ErrUnavailable)
		return
	}
	raw := r.Header.Get("If-Match")
	version, err := strconv.ParseInt(strings.Trim(raw, `"`), 10, 64)
	if err != nil || version < 1 || raw != `"`+strconv.FormatInt(version, 10)+`"` {
		a.fail(w, r, model.Invalid("If-Match requires the quoted source version"))
		return
	}
	a.Service.DefinitionMu.Lock()
	defer a.Service.DefinitionMu.Unlock()
	if err = a.Workspace.ManagedSources.Delete(r.Context(), r.PathValue("connection"), version); err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, map[string]string{"state": "deleted"})
}
