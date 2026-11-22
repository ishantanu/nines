package operator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"slices"

	"gopkg.in/yaml.v3"

	k "github.com/ishantanu/nines/api/kubernetes/v1alpha1"
	domain "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// migrated are the kinds whose CRDs moved from v1alpha1 to v1beta1.
var migrated = []string{domain.SLOKind, domain.JourneyKind, domain.BindingKind}

// Migrate rewrites every SLO, journey and binding resource stored as
// v1alpha1, in every namespace, as v1beta1, then records that v1alpha1 is no
// longer stored so the v1beta1-only CRDs can be applied. It needs the
// transitional CRDs, which serve v1alpha1 and store v1beta1, and no writer
// using v1beta1 until it finishes. Each original object is logged as a JSON
// line before it is rewritten. A failure leaves storedVersions unchanged, and
// running it again is safe.
func Migrate(ctx context.Context, api *API, log io.Writer) error {
	for _, kind := range migrated {
		crd, err := api.crd(ctx, kind)
		if err != nil {
			return err
		}
		if slices.Equal(crd.Status.StoredVersions, []string{"v1beta1"}) {
			_, _ = fmt.Fprintf(log, "%s: already migrated\n", kind)
			continue
		}
		if !crd.serves(domain.APIVersion) || crd.storage() != Versions[kind] {
			return fmt.Errorf("%s: apply deploy/operator/crds-transitional.yaml before migrating", kind)
		}
		items, err := api.listAll(ctx, domain.APIVersion, kind)
		if err != nil {
			return err
		}
		failed := 0
		for _, r := range items {
			if lastWrite(r) == Versions[kind] {
				// Already written as v1beta1; reading it as v1alpha1 would prune it.
				_, _ = fmt.Fprintf(log, "%s %s/%s: already v1beta1\n", kind, r.Metadata.Namespace, r.Metadata.Name)
				continue
			}
			backup, _ := json.Marshal(r)
			_, _ = fmt.Fprintf(log, "%s\n", backup)
			converted, err := ConvertResource(r)
			if err == nil {
				converted.Metadata = r.Metadata
				converted.Status = nil
				err = api.request(ctx, "PUT", "/apis/"+Versions[kind]+"/namespaces/"+url.PathEscape(r.Metadata.Namespace)+"/"+Plurals[kind]+"/"+url.PathEscape(r.Metadata.Name), converted, nil)
			}
			if err != nil {
				failed++
				_, _ = fmt.Fprintf(log, "%s %s/%s: %v\n", kind, r.Metadata.Namespace, r.Metadata.Name, err)
			}
		}
		if failed > 0 {
			return fmt.Errorf("%s: %d of %d resources were not migrated; fix them and run migrate again", kind, failed, len(items))
		}
		crd.Status.StoredVersions = []string{"v1beta1"}
		if err = api.request(ctx, "PUT", "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/"+crd.Metadata.Name+"/status", crd.raw(), nil); err != nil {
			return fmt.Errorf("%s: record stored versions: %w", kind, err)
		}
		_, _ = fmt.Fprintf(log, "%s: migrated %d resources\n", kind, len(items))
	}
	return nil
}

// ConvertResource rewrites a v1alpha1 SLO, journey or binding resource as
// v1beta1, keeping its metadata. Other resources are returned unchanged.
func ConvertResource(r k.Resource) (k.Resource, error) {
	if r.APIVersion != domain.APIVersion || !slices.Contains(migrated, r.Kind) {
		return r, nil
	}
	b, err := json.Marshal(map[string]any{"apiVersion": r.APIVersion, "kind": r.Kind, "metadata": domain.Metadata{Name: r.Metadata.Name, Project: r.Metadata.Annotations[k.ProjectAnnotation]}, "spec": r.Spec})
	if err != nil {
		return r, err
	}
	d, err := model.ParseDefinition(b)
	if err != nil {
		return r, err
	}
	if d.SLO != nil && kubeSLO(domain.ServiceLevelObjective(*d.SLO)).Spec.MeasurementBinding != nil {
		return r, model.Invalid("the SLO references a binding without its queries; add measurement or queries")
	}
	out, err := FromDefinition(d, r.Metadata.Namespace)
	if err != nil {
		return r, err
	}
	out.Metadata = r.Metadata
	return out, nil
}

type crdObject struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		Versions []struct {
			Name    string `json:"name"`
			Served  bool   `json:"served"`
			Storage bool   `json:"storage"`
		} `json:"versions"`
	} `json:"spec"`
	Status struct {
		StoredVersions []string `json:"storedVersions"`
	} `json:"status"`
	full map[string]any
}

func (c crdObject) serves(apiVersion string) bool {
	for _, v := range c.Spec.Versions {
		if "nines.dev/"+v.Name == apiVersion && v.Served {
			return true
		}
	}
	return false
}

func (c crdObject) storage() string {
	for _, v := range c.Spec.Versions {
		if v.Storage {
			return "nines.dev/" + v.Name
		}
	}
	return ""
}

// raw is the full object with the new stored versions, for a status update.
func (c crdObject) raw() map[string]any {
	status, _ := c.full["status"].(map[string]any)
	if status == nil {
		status = map[string]any{}
	}
	status["storedVersions"] = c.Status.StoredVersions
	c.full["status"] = status
	return c.full
}

func (a *API) crd(ctx context.Context, kind string) (crdObject, error) {
	var c crdObject
	var full map[string]any
	path := "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/" + Plurals[kind] + ".nines.dev"
	if err := a.request(ctx, "GET", path, nil, &full); err != nil {
		return c, fmt.Errorf("%s: read CRD: %w", kind, err)
	}
	b, _ := json.Marshal(full)
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	c.full = full
	return c, nil
}

// listAll lists a kind in every namespace at the given version.
func (a *API) listAll(ctx context.Context, apiVersion, kind string) ([]k.Resource, error) {
	out := []k.Resource{}
	next := ""
	for page := 0; page < 1000; page++ {
		var response struct {
			Items    []k.Resource `json:"items"`
			Metadata struct {
				Continue string `json:"continue"`
			} `json:"metadata"`
		}
		path := "/apis/" + apiVersion + "/" + Plurals[kind] + "?limit=100"
		if next != "" {
			path += "&continue=" + url.QueryEscape(next)
		}
		if err := a.request(ctx, "GET", path, nil, &response); err != nil {
			return nil, fmt.Errorf("%s: list: %w", kind, err)
		}
		for _, item := range response.Items {
			item.APIVersion, item.Kind = apiVersion, kind
			out = append(out, item)
		}
		if next = response.Metadata.Continue; next == "" {
			return out, nil
		}
	}
	return nil, fmt.Errorf("%s: listing is incomplete", kind)
}

// lastWrite is the API version of an object's most recent spec write, from
// its managed fields; status writes are ignored.
func lastWrite(r k.Resource) string {
	latest, version := "", ""
	for _, raw := range r.Metadata.ManagedFields {
		var f struct {
			APIVersion  string `json:"apiVersion"`
			Time        string `json:"time"`
			Subresource string `json:"subresource"`
		}
		if json.Unmarshal(raw, &f) == nil && f.Subresource == "" && f.Time >= latest {
			latest, version = f.Time, f.APIVersion
		}
	}
	return version
}

// ConvertDocuments converts every v1alpha1 SLO, journey and binding in a
// multi-document Kubernetes YAML file to v1beta1, for GitOps repositories.
// Other documents are kept; comments are not.
func ConvertDocuments(body []byte) ([]byte, error) {
	if len(body) > 8<<20 {
		return nil, model.Invalid("input must be at most 8 MiB")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	var out bytes.Buffer
	for i := 0; ; i++ {
		var doc any
		err := decoder.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if doc == nil {
			continue
		}
		raw, err := json.Marshal(doc)
		if err != nil {
			return nil, err
		}
		var r k.Resource
		if json.Unmarshal(raw, &r) == nil && r.APIVersion == domain.APIVersion && slices.Contains(migrated, r.Kind) {
			if r, err = ConvertResource(r); err != nil {
				return nil, fmt.Errorf("document %d (%s %s): %w", i+1, r.Kind, r.Metadata.Name, err)
			}
			if raw, err = json.Marshal(r); err != nil {
				return nil, err
			}
			doc = nil
			if err = json.Unmarshal(raw, &doc); err != nil {
				return nil, err
			}
		}
		if out.Len() > 0 {
			out.WriteString("---\n")
		}
		b, err := YAML(doc)
		if err != nil {
			return nil, err
		}
		out.Write(b)
	}
	return out.Bytes(), nil
}
