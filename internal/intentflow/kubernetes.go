package intentflow

import (
	"context"
	"net/http"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/operator"
	"github.com/ishantanu/nines/internal/tenancy"
)

func (s *Service) ExportKubernetes(ctx context.Context, id, namespace, digest string) (v1.IntentKubernetesExport, error) {
	out := v1.IntentKubernetesExport{APIVersion: v1.APIVersion, Tenant: s.Platform.TenantID, Namespace: namespace, ChangeSetDigest: digest}
	if !tenancy.ValidID(namespace) || len(digest) != 64 {
		return out, model.Invalid("provide a namespace and exact change-set digest")
	}
	r, e := s.Read(ctx, id)
	if e != nil {
		return out, e
	}
	if r.ChangeSet == nil || r.ChangeSet.Digest != digest {
		return out, ErrConflict
	}
	items := []any{}
	for _, change := range r.ChangeSet.Changes {
		d, e := model.ParseDefinition(change.Manifest)
		if e != nil {
			return out, e
		}
		resource, e := operator.FromDefinition(d, namespace)
		if e != nil {
			return out, e
		}
		resource.Metadata.Annotations["nines.dev/change-set-digest"] = digest
		items = append(items, resource)
	}
	body, e := operator.YAML(map[string]any{"apiVersion": "v1", "kind": "List", "items": items})
	out.YAML = string(body)
	return out, e
}
func mountKubernetesExport(m *http.ServeMux, s *Service) {
	m.HandleFunc("GET /api/intent-conversations/{id}/change-set/kubernetes", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for key, values := range q {
			if (key != "namespace" && key != "digest") || len(values) != 1 {
				fail(w, model.Invalid("provide only namespace and digest"))
				return
			}
		}
		out, e := s.ExportKubernetes(r.Context(), r.PathValue("id"), q.Get("namespace"), q.Get("digest"))
		reply(w, out, e, 200)
	})
}

func (s *Service) ExportRollbackKubernetes(ctx context.Context, id, namespace, digest string) (v1.IntentKubernetesExport, error) {
	out := v1.IntentKubernetesExport{APIVersion: v1.APIVersion, Tenant: s.Platform.TenantID, Namespace: namespace, ChangeSetDigest: digest}
	if !tenancy.ValidID(namespace) || len(digest) != 64 {
		return out, model.Invalid("provide a namespace and exact rollback digest")
	}
	session, e := s.Read(ctx, id)
	if e != nil {
		return out, e
	}
	r := record{Session: session}
	c := findChangeSet(&r, digest, true)
	if c == nil || c.Rollback.State == "blocked" || c.Rollback.State == "rejected" || rollbackDigest(s.Platform.TenantID, id, *c.Rollback) != digest {
		return out, ErrConflict
	}
	items := []any{}
	for _, change := range c.Rollback.Changes {
		d, e := model.ParseDefinition(change.Manifest)
		if e != nil {
			return out, e
		}
		resource, e := operator.FromDefinition(d, namespace)
		if e != nil {
			return out, e
		}
		resource.Metadata.Annotations["nines.dev/change-set-digest"] = digest
		resource.Metadata.Annotations["nines.dev/rollback-of"] = c.Digest
		items = append(items, resource)
	}
	body, e := operator.YAML(map[string]any{"apiVersion": "v1", "kind": "List", "items": items})
	out.YAML = string(body)
	return out, e
}
