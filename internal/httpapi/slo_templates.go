package httpapi

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/slotemplates"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func (a *Server) sloTemplates(w http.ResponseWriter, r *http.Request) {
	q, err := parseContextQuery(r, "category")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.Workspace.Templates.List(q["category"])
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out.Tenant = a.Workspace.ID
	write(w, 200, out)
}
func (a *Server) sloTemplate(w http.ResponseWriter, r *http.Request) {
	if _, err := parseContextQuery(r); err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.Workspace.Templates.Get(r.PathValue("template"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v1.SLOTemplateDetail{APIVersion: v1.APIVersion, Tenant: a.Workspace.ID, Template: out})
}
func (a *Server) renderSLOTemplate(w http.ResponseWriter, r *http.Request) {
	if _, err := parseContextQuery(r); err != nil {
		a.fail(w, r, err)
		return
	}
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		a.fail(w, r, model.Invalid("Content-Type must be application/json"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	defer func() { _ = r.Body.Close() }()
	raw, err := io.ReadAll(r.Body)
	if err != nil || !json.Valid(raw) {
		a.fail(w, r, model.Invalid("invalid template values JSON"))
		return
	}
	in, err := slotemplates.ParseValues(raw)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.Workspace.Templates.Render(r.PathValue("template"), in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out.Tenant = a.Workspace.ID
	write(w, 200, out)
}
