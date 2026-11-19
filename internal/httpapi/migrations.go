package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/migrate"
	"github.com/ishantanu/nines/internal/model"
)

// migrationConvert converts Sloth or Pyrra definitions. It is stateless and
// read-only: it saves nothing and contacts no provider. The result is a
// proposal for review; applying still goes through plan and apply.
func (a *Server) migrationConvert(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, migrate.MaxInputBytes+64<<10)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		a.fail(w, r, model.Invalid("the request is too large or unreadable"))
		return
	}
	var in v1.MigrationRequest
	if json.Unmarshal(raw, &in) != nil {
		a.fail(w, r, model.Invalid("the request must be JSON with tool and content"))
		return
	}
	result, err := migrate.Convert(in.Tool, []byte(in.Content), migrate.Options{Project: in.Project, Source: in.Source, WindowSeconds: in.WindowSeconds, Lifecycle: in.Lifecycle})
	if err != nil {
		a.fail(w, r, model.Invalid("%s", err.Error()))
		return
	}
	result.Tenant = a.Workspace.ID
	if kind := a.sourceKind(in.Source); kind != "" && !speaksPromQL(kind) {
		result.Notes = append(result.Notes, v1.MigrationNote{Level: "lossy", Field: "source", Message: "The connection " + sourceName(in.Source) + " reads " + kind + ", not PromQL. Sloth and Pyrra queries are PromQL, so Nines will refuse these definitions when you plan them. Choose a Prometheus, Mimir, Dash0 or VictoriaMetrics connection, or rewrite the queries in the provider's own query language."})
	}
	write(w, 200, result)
}

func sourceName(name string) string {
	if name == "" {
		return "primary"
	}
	return name
}

// sourceKind returns the provider kind of a configured connection, or "" when
// the name is not a connection this server knows (a managed one, for example).
func (a *Server) sourceKind(name string) string {
	if name == "" || name == "primary" {
		return a.Workspace.Source.Kind
	}
	for _, c := range a.Workspace.Connections {
		if c.Name == name {
			return c.Kind
		}
	}
	return ""
}

// speaksPromQL reports whether a provider can serve the PromQL queries that
// the importers produce.
func speaksPromQL(kind string) bool {
	switch kind {
	case "prometheus", "mimir", "dash0", "victoriametrics":
		return true
	}
	return false
}
