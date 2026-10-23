package httpapi

import (
	"context"
	"mime"
	"net/http"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func (a *Server) compileMeasurement(w http.ResponseWriter, r *http.Request) {
	// Typed clients send JSON; file-based clients send YAML. Both pass through
	// the same bounded, strict definition parser.
	if typ, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); typ == "application/json" {
		r = r.Clone(r.Context())
		r.Header.Set("Content-Type", "application/yaml")
	}
	def, err := definitionBody(w, r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if def.SLO == nil {
		a.fail(w, r, model.Invalid("provide one ServiceLevelObjective with spec.measurement"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	result, err := a.Service.CompileMeasurement(ctx, v1.ServiceLevelObjective(*def.SLO))
	if err != nil {
		telemetryError(w, err)
		return
	}
	write(w, 200, result)
}
