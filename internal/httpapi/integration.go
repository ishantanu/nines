package httpapi

import "net/http"

func (a *Server) integrationStatus(w http.ResponseWriter, r *http.Request) {
	out, err := a.Service.IntegrationStatus(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, out)
}
