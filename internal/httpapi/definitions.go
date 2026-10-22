package httpapi

import (
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ishantanu/nines/internal/model"
)

func definitionBody(w http.ResponseWriter, r *http.Request) (model.Definition, error) {
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || (typ != "application/yaml" && typ != "application/x-yaml" && typ != "text/yaml") {
		return model.Definition{}, model.Invalid("Content-Type must be application/yaml")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer func() {
		_ = r.Body.Close()
	}()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return model.Definition{}, model.Invalid("manifest exceeds size limit or cannot be read")
	}
	return model.ParseDefinition(b)
}
func (a *Server) definitionValidate(w http.ResponseWriter, r *http.Request) {
	d, err := definitionBody(w, r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, map[string]any{"apiVersion": model.APIVersion, "kind": d.Kind(), "metadata": d.Metadata(), "digest": model.DefinitionDigest(d.Manifest()), "status": "valid"})
}
func (a *Server) definitionPlan(w http.ResponseWriter, r *http.Request) {
	d, err := definitionBody(w, r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	p, err := a.Service.PlanDefinition(r.Context(), d)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, p)
}
func (a *Server) definitionApply(w http.ResponseWriter, r *http.Request) {
	d, err := definitionBody(w, r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	value := r.Header.Get("If-Match")
	expected, err := strconv.ParseInt(strings.Trim(value, `"`), 10, 64)
	if err != nil || expected < 0 || value != `"`+strconv.FormatInt(expected, 10)+`"` {
		a.fail(w, r, model.Invalid("If-Match must contain the quoted expected revision, e.g. \"0\" to create"))
		return
	}
	result, err := a.Service.ApplyDefinition(r.Context(), d, expected)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("ETag", `"`+strconv.FormatInt(result.Revision, 10)+`"`)
	write(w, 200, result)
}
func (a *Server) sloManifest(w http.ResponseWriter, r *http.Request) {
	rec, err := a.Service.Store.Read(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	b, err := yaml.Marshal(model.DefinitionFromSLO(rec.SLO))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("ETag", `"`+strconv.FormatInt(model.ConfigRevision(rec.SLO), 10)+`"`)
	_, _ = w.Write(b)
}
