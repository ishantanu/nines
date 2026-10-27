package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/kubediscovery"
	"github.com/ishantanu/nines/internal/model"
)

func (a *Server) dependencyDiscoveryCapabilities(w http.ResponseWriter, r *http.Request) {
	if _, err := parseContextQuery(r); err != nil {
		telemetryError(w, err)
		return
	}
	namespaces := []string{}
	if reader := a.Service.KubernetesDiscovery; reader != nil {
		namespaces = reader.Namespaces()
	}
	write(w, 200, map[string]any{"apiVersion": v1.APIVersion, "tenant": a.Service.TenantID, "namespaces": namespaces, "clusterEnabled": len(namespaces) > 0, "resourceImport": true})
}
func (a *Server) dependencyDiscover(w http.ResponseWriter, r *http.Request) {
	if _, err := parseContextQuery(r); err != nil {
		telemetryError(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, kubediscovery.MaxBytes)
	defer func() { _ = r.Body.Close() }()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var in v1.DependencyDiscoveryInput
	if decoder.Decode(&in) != nil {
		telemetryError(w, model.Invalid("provide one sanitized dependency discovery request, at most 2 MiB"))
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		telemetryError(w, model.Invalid("provide one discovery request"))
		return
	}
	if err := kubediscovery.ValidateRequest(in); err != nil {
		telemetryError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	switch in.Mode {
	case "resources":
		if in.Inventory == nil {
			telemetryError(w, model.Invalid("resource import requires sanitized inventory"))
			return
		}
	case "cluster":
		if in.Inventory != nil {
			telemetryError(w, model.Invalid("cluster mode does not accept caller-supplied inventory"))
			return
		}
		if a.Service.KubernetesDiscovery == nil {
			telemetryError(w, model.Invalid("Kubernetes cluster discovery is not configured for this tenant; import resources instead"))
			return
		}
		allowed := false
		for _, ns := range a.Service.KubernetesDiscovery.Namespaces() {
			if ns == in.Namespace {
				allowed = true
			}
		}
		if !allowed || !kubediscovery.ValidNamespace(in.Namespace) {
			telemetryError(w, model.Invalid("namespace is not enabled for this tenant's Kubernetes discovery"))
			return
		}
		inventory, err := a.Service.KubernetesDiscovery.Read(ctx, in.Namespace)
		if err != nil {
			telemetryError(w, err)
			return
		}
		in.Inventory = &inventory
	default:
		telemetryError(w, model.Invalid("discovery mode must be resources or cluster"))
		return
	}
	out, err := kubediscovery.Discover(in)
	if err != nil {
		telemetryError(w, err)
		return
	}
	out.Tenant = a.Service.TenantID
	write(w, 200, out)
}
