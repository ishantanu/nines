package httpapi

import (
	"net/http"
	"strconv"

	"github.com/ishantanu/nines/internal/model"
)

func (a *Server) measurementQuality(w http.ResponseWriter, r *http.Request) {
	expected := int64(0)
	var err error
	if text := r.URL.Query().Get("expectedRevision"); text != "" {
		expected, err = strconv.ParseInt(text, 10, 64)
		if err != nil || expected < 1 {
			a.fail(w, r, model.Invalid("expectedRevision must be positive"))
			return
		}
	}
	cohorts := false
	if text := r.URL.Query().Get("cohorts"); text != "" {
		if text != "true" && text != "false" {
			a.fail(w, r, model.Invalid("cohorts must be true or false"))
			return
		}
		cohorts = text == "true"
	}
	out, err := a.Service.MeasurementQuality(r.Context(), r.PathValue("id"), expected, cohorts)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, out)
}

func (a *Server) budgetDecision(w http.ResponseWriter, r *http.Request) {
	expected := int64(0)
	if text := r.URL.Query().Get("expectedRevision"); text != "" {
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil || n < 1 {
			a.fail(w, r, model.Invalid("expectedRevision must be positive"))
			return
		}
		expected = n
	}
	out, err := a.Service.ErrorBudgetDecision(r.Context(), r.PathValue("id"), expected)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, out)
}
