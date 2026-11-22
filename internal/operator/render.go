// Package operator is an optional Kubernetes adapter around the platform's
// deterministic definitions and rule compiler. It has no model dependencies.
package operator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	k "github.com/ishantanu/nines/api/kubernetes/v1alpha1"
	domain "github.com/ishantanu/nines/api/v1alpha1"
	beta "github.com/ishantanu/nines/api/v1beta1"
	"github.com/ishantanu/nines/internal/deprecation"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/tenancy"
)

var Plurals = map[string]string{domain.SLOKind: "servicelevelobjectives", domain.JourneyKind: "criticaluserjourneys", domain.BindingKind: "measurementbindings", domain.IntentKind: "servicelevelobjectiveintents"}

func YAML(v any) ([]byte, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return nil, e
	}
	var value any
	if e = json.Unmarshal(b, &value); e != nil {
		return nil, e
	}
	return yaml.Marshal(value)
}
func FromDefinition(d model.Definition, namespace string) (k.Resource, error) {
	if Plurals[d.Kind()] == "" {
		return k.Resource{}, model.Invalid("this resource kind has no operator CRD")
	}
	if !tenancy.ValidID(namespace) {
		return k.Resource{}, model.Invalid("namespace must be a DNS label")
	}
	var spec any
	switch {
	case d.SLO != nil:
		spec = beta.FromHub(kubeSLO(domain.ServiceLevelObjective(*d.SLO))).Spec
	case d.Journey != nil:
		spec = beta.JourneyFromHub(domain.CriticalUserJourney(*d.Journey), nil).Spec
	case d.Binding != nil:
		spec = beta.BindingFromHub(domain.MeasurementBinding(*d.Binding)).Spec
	}
	b, e := json.Marshal(spec)
	if e != nil {
		return k.Resource{}, e
	}
	return k.Resource{APIVersion: Versions[d.Kind()], Kind: d.Kind(), Metadata: k.Metadata{Name: d.Metadata().Name, Namespace: namespace, Annotations: map[string]string{k.ProjectAnnotation: d.Metadata().Project}}, Spec: b}, nil
}

// kubeSLO is the self-contained form of an SLO for Kubernetes. A binding
// reference is replaced by the queries or window derived from it, which is
// what rules are generated from, because the binding may live only on the
// server. Intent provenance is carried by the owning intent resource.
func kubeSLO(slo domain.ServiceLevelObjective) domain.ServiceLevelObjective {
	slo.Spec.Intent = nil
	if slo.Spec.MeasurementBinding != nil && (slo.Spec.ProviderWindow != nil || slo.Spec.GoodQuery != "") {
		slo.Spec.MeasurementBinding = nil
	}
	return slo
}
func Native(r k.Resource) (model.Definition, error) {
	if Plurals[r.Kind] == "" || r.APIVersion != Versions[r.Kind] {
		return model.Definition{}, model.Invalid("unsupported Kubernetes resource version or kind; run nines operator migrate after upgrading")
	}
	if r.Kind == domain.SLOKind {
		var spec struct {
			Binding json.RawMessage `json:"binding"`
		}
		if json.Unmarshal(r.Spec, &spec) == nil && len(spec.Binding) > 0 && string(spec.Binding) != "null" {
			return model.Definition{}, model.Invalid("a Kubernetes SLO cannot reference a binding; use measurement or queries, or compile the binding through an intent")
		}
	}
	b, e := json.Marshal(map[string]any{"apiVersion": r.APIVersion, "kind": r.Kind, "metadata": domain.Metadata{Name: r.Metadata.Name, Project: r.Metadata.Annotations[k.ProjectAnnotation]}, "spec": r.Spec})
	if e != nil {
		return model.Definition{}, e
	}
	return model.ParseDefinition(b)
}
func CanonicalDigest(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	return model.DefinitionDigest(v)
}
func RuleName(id string) string {
	prefix := id
	if len(prefix) > 40 {
		prefix = prefix[:40]
	}
	return "nines-" + prefix + "-" + model.DefinitionDigest(id)[:8]
}
func PrometheusRule(d model.SLODefinition, tenant, namespace, ownerUID string, labels map[string]string, generation int64) (k.Resource, error) {
	if !tenancy.ValidID(tenant) || !tenancy.ValidID(namespace) {
		return k.Resource{}, model.Invalid("explicit tenant and namespace are required")
	}
	if e := d.Validate(); e != nil {
		return k.Resource{}, e
	}
	slo := d.SLO()
	slo.Revision = generation
	group, e := rules.Generate(slo, tenant)
	if ownerUID != "" {
		group, e = rules.GenerateOwned(slo, tenant, ownerUID)
	}
	if e != nil {
		return k.Resource{}, e
	}
	spec, _ := json.Marshal(rules.File{Groups: []rules.Group{group}})
	r := k.Resource{APIVersion: "monitoring.coreos.com/v1", Kind: "PrometheusRule", Metadata: k.Metadata{Name: RuleName(d.Metadata.Name), Namespace: namespace, Annotations: map[string]string{"nines.dev/measurement-digest": service.MeasurementDigest(slo)}, Labels: map[string]string{"app.kubernetes.io/managed-by": "nines-operator"}}, Spec: spec}
	for key, v := range labels {
		r.Metadata.Labels[key] = v
	}
	if ownerUID != "" {
		r.Metadata.OwnerReferences = []k.OwnerReference{{APIVersion: Versions[domain.SLOKind], Kind: domain.SLOKind, Name: d.Metadata.Name, UID: ownerUID, Controller: true, BlockOwnerDeletion: func() *bool { value := true; return &value }()}}
	}
	return r, nil
}
func DecodeResource(body []byte) (k.Resource, error) {
	if len(body) == 0 || len(body) > 1<<20 {
		return k.Resource{}, model.Invalid("resource must be at most 1 MiB")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	var value any
	if e := decoder.Decode(&value); e != nil {
		return k.Resource{}, e
	}
	if decoder.Decode(new(any)) != io.EOF {
		return k.Resource{}, model.Invalid("provide one Kubernetes resource")
	}
	b, e := json.Marshal(value)
	if e != nil {
		return k.Resource{}, e
	}
	var r k.Resource
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	e = d.Decode(&r)
	return r, e
}
func schemaFor(t reflect.Type) map[string]any {
	if t == reflect.TypeOf(beta.Duration{}) {
		return map[string]any{"type": "string", "maxLength": 32, "pattern": `^([0-9]+(d|h|m|s|ms))+$`}
	}
	if t == reflect.TypeOf(domain.SLOLifecycle("")) {
		return map[string]any{"type": "string", "enum": []string{"experimental", "aspirational", "committed", "deprecated"}}
	}
	if t.Kind() == reflect.Pointer {
		v := schemaFor(t.Elem())
		v["nullable"] = true
		return v
	}
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int64:
		return map[string]any{"type": "integer", "format": "int64"}
	case reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": schemaFor(t.Elem()), "maxItems": 1000}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": schemaFor(t.Elem()), "maxProperties": 1000}
	case reflect.Struct:
		properties := map[string]any{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous {
				nested := schemaFor(f.Type)
				for key, val := range nested["properties"].(map[string]any) {
					properties[key] = val
				}
				required = append(required, nested["required"].([]string)...)
				continue
			}
			tag := f.Tag.Get("json")
			name, opts, _ := strings.Cut(tag, ",")
			if name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			properties[name] = schemaFor(f.Type)
			if t == reflect.TypeOf(beta.QualityPolicy{}) {
				switch name {
				case "minEvents":
					properties[name] = map[string]any{"type": "number", "minimum": 0, "maximum": 1e18}
				case "cohortLabels":
					properties[name] = map[string]any{"type": "array", "maxItems": 3, "x-kubernetes-list-type": "set", "items": map[string]any{"type": "string", "pattern": model.LabelName.String()}}
				}
			}
			if t == reflect.TypeOf(domain.MeasurementQualityPolicy{}) {
				switch name {
				case "minEvents":
					properties[name] = map[string]any{"type": "number", "minimum": 0, "maximum": 1e18}
				case "maxSampleAgeSeconds":
					properties[name] = map[string]any{"type": "integer", "format": "int64", "minimum": 0, "maximum": 31 * 86400}
				case "cohortLabels":
					properties[name] = map[string]any{"type": "array", "maxItems": 3, "x-kubernetes-list-type": "set", "items": map[string]any{"type": "string", "pattern": model.LabelName.String()}}
				}
			}
			if t == reflect.TypeOf(domain.ErrorBudgetPolicy{}) {
				switch name {
				case "reviewBelow":
					properties[name] = map[string]any{"type": "number", "minimum": 0, "maximum": 1}
				case "onExhausted":
					properties[name] = map[string]any{"type": "string", "enum": []string{"blocked", "review_required"}}
				}
			}
			if (t == reflect.TypeOf(domain.BurnRatePolicy{}) || t == reflect.TypeOf(beta.BurnRatePolicy{})) && name == "minEvents" {
				properties[name] = map[string]any{"type": "number", "minimum": 0, "maximum": 1e18}
			}
			if (t == reflect.TypeOf(domain.SLOSpec{}) || t == reflect.TypeOf(beta.SLOSpec{})) && name == "labels" {
				properties[name] = map[string]any{"type": "object", "maxProperties": model.MaxSLOLabels, "additionalProperties": map[string]any{"type": "string", "minLength": 1, "maxLength": model.MaxLabelValueLen}}
			}
			if (t == reflect.TypeOf(domain.BurnRatePolicy{}) || t == reflect.TypeOf(beta.BurnRatePolicy{})) && name == "confidence" {
				// Zero turns it off; the server rejects values between 0 and 0.5.
				properties[name] = map[string]any{"type": "number", "minimum": 0, "maximum": 0.9999}
			}
			if !strings.Contains(opts, "omitempty") {
				required = append(required, name)
			}
		}
		if t == reflect.TypeOf(domain.BurnRatePolicy{}) {
			filtered := []string{}
			for _, field := range required {
				if field != "shortWindowSeconds" && field != "longWindowSeconds" {
					filtered = append(filtered, field)
				}
			}
			for _, field := range []string{"shortWindow", "longWindow"} {
				properties[field] = map[string]any{"type": "string", "maxLength": 64, "pattern": domain.BurnWindowPattern}
			}
			return map[string]any{"type": "object", "properties": properties, "required": filtered, "x-kubernetes-validations": []any{
				map[string]any{"rule": "has(self.shortWindow) != has(self.shortWindowSeconds)", "message": "Specify exactly one of shortWindow or shortWindowSeconds."},
				map[string]any{"rule": "has(self.longWindow) != has(self.longWindowSeconds)", "message": "Specify exactly one of longWindow or longWindowSeconds."},
			}}
		}
		return map[string]any{"type": "object", "properties": properties, "required": required}
	default:
		panic(fmt.Sprintf("unsupported CRD schema type %s", t))
	}
}

// Versions is the API version each kind is served and stored as. Intents
// have no v1beta1 form and stay v1alpha1.
var Versions = map[string]string{domain.SLOKind: beta.APIVersion, domain.JourneyKind: beta.APIVersion, domain.BindingKind: beta.APIVersion, domain.IntentKind: domain.APIVersion}

// CRDs are the custom resource definitions to install.
func CRDs() []any { return crds(false, deprecation.ForCRDVersion) }

// TransitionalCRDs also serve the v1alpha1 schema, not stored, for
// `nines operator migrate` to read objects stored before v1beta1.
func TransitionalCRDs() []any { return crds(true, deprecation.ForCRDVersion) }

func crds(transitional bool, deprecated func(crd, version string) (deprecation.Deprecation, bool)) []any {
	kinds := []struct {
		Kind      string
		Spec, Old any
		Short     string
	}{{domain.SLOKind, beta.SLOSpec{}, domain.SLOSpec{}, "slo"}, {domain.JourneyKind, beta.JourneySpec{}, domain.JourneySpec{}, "cuj"}, {domain.BindingKind, beta.BindingSpec{}, domain.BindingSpec{}, "mb"}, {domain.IntentKind, k.IntentSpec{}, nil, "sloi"}}
	out := []any{}
	for _, item := range kinds {
		specSchema := schemaFor(reflect.TypeOf(item.Spec))
		switch item.Kind {
		case domain.IntentKind:
			specSchema["x-kubernetes-validations"] = []any{
				map[string]string{"rule": "has(self.measurement) != has(self.measurementSelection)", "message": "define spec.measurement; a legacy measurementSelection is an alternative, never an additional input"},
				map[string]string{"rule": "!has(self.measurement) || (has(self.stepSeconds) && self.stepSeconds >= 1 && self.stepSeconds <= 3600)", "message": "inline measurement requires stepSeconds from 1 to 3600"},
			}
		case domain.SLOKind:
			specSchema["x-kubernetes-validations"] = []any{map[string]string{"rule": "[has(self.measurement), has(self.queries), has(self.binding)].filter(x, x).size() <= 1", "message": "use at most one of measurement, queries or binding"}}
		}
		versions := []map[string]any{version(Versions[item.Kind], specSchema, true)}
		if transitional && item.Old != nil {
			old := schemaFor(reflect.TypeOf(item.Old))
			versions = append(versions, version(domain.APIVersion, old, false))
		}
		for _, v := range versions {
			if d, ok := deprecated(Plurals[item.Kind]+".nines.dev", v["name"].(string)); ok {
				v["deprecated"] = true
				v["deprecationWarning"] = d.Message()
			}
		}
		out = append(out, map[string]any{"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition", "metadata": map[string]string{"name": Plurals[item.Kind] + ".nines.dev"}, "spec": map[string]any{"group": "nines.dev", "scope": "Namespaced", "names": map[string]any{"kind": item.Kind, "plural": Plurals[item.Kind], "singular": strings.ToLower(item.Kind), "shortNames": []string{item.Short}}, "versions": versions}})
	}
	return out
}

func version(apiVersion string, spec map[string]any, storage bool) map[string]any {
	_, name, _ := strings.Cut(apiVersion, "/")
	schema := map[string]any{"type": "object", "required": []string{"spec"}, "properties": map[string]any{"apiVersion": map[string]any{"type": "string"}, "kind": map[string]any{"type": "string"}, "metadata": map[string]any{"type": "object"}, "spec": spec, "status": map[string]any{"type": "object", "x-kubernetes-preserve-unknown-fields": true}}}
	return map[string]any{"name": name, "served": true, "storage": storage, "schema": map[string]any{"openAPIV3Schema": schema}, "subresources": map[string]any{"status": map[string]any{}}, "additionalPrinterColumns": []any{map[string]any{"name": "Ready", "type": "string", "jsonPath": ".status.conditions[?(@.type=='Ready')].status"}, map[string]any{"name": "Reason", "type": "string", "jsonPath": ".status.conditions[?(@.type=='Ready')].reason"}}}
}

func FromIntent(goal domain.SLOIntent, namespace string, selection k.MeasurementSelection) (k.Resource, error) {
	if e := model.ValidateIntent(goal); e != nil {
		return k.Resource{}, e
	}
	if !tenancy.ValidID(namespace) {
		return k.Resource{}, model.Invalid("provide a Kubernetes namespace")
	}
	if goal.Spec.Measurement == nil && (!tenancy.ValidID(selection.Name) || selection.ExpectedGeneration < 1) {
		return k.Resource{}, model.Invalid("legacy intents require a binding name and expected generation; prefer spec.measurement")
	}
	if goal.Spec.Measurement != nil && (selection.Name != "" || selection.ExpectedGeneration != 0) {
		return k.Resource{}, model.Invalid("inline measurement cannot be combined with a binding selection")
	}
	spec, e := json.Marshal(k.IntentSpec{IntentSpec: goal.Spec, MeasurementSelection: selection})
	if e != nil {
		return k.Resource{}, e
	}
	return k.Resource{APIVersion: goal.APIVersion, Kind: goal.Kind, Metadata: k.Metadata{Name: goal.Metadata.Name, Namespace: namespace, Annotations: map[string]string{k.ProjectAnnotation: goal.Metadata.Project}}, Spec: spec}, nil
}
