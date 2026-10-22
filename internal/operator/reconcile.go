package operator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	k "github.com/ishantanu/nines/api/kubernetes/v1alpha1"
	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

var errRequeue = errors.New("resource metadata updated")

type Controller struct {
	Config Config
	API    *API
}

func ready(r k.Resource, status k.Status, ok bool, reason, message string) k.Status {
	value := "False"
	if ok {
		value = "True"
	}
	status.ObservedGeneration = r.Metadata.Generation
	status.Conditions = []k.Condition{{Type: "Ready", Status: value, Reason: reason, Message: message, ObservedGeneration: r.Metadata.Generation, LastTransitionTime: time.Now().UTC()}}
	return status
}
func prior(r k.Resource) k.Status {
	if r.Status == nil {
		return k.Status{}
	}
	return *r.Status
}
func (c *Controller) Reconcile(ctx context.Context, r k.Resource) error {
	if r.Metadata.Namespace != c.Config.Namespace || r.Metadata.UID == "" {
		return fmt.Errorf("resource is outside configured namespace/project scope")
	}
	if r.Metadata.DeletionTimestamp != nil {
		return c.cleanup(ctx, r)
	}
	if !c.Config.Allows(r.Metadata.Annotations[k.ProjectAnnotation]) {
		return c.API.Status(ctx, r, ready(r, prior(r), false, "ScopeDenied", "Project is not allowed in this operator namespace."))
	}
	status, e := c.reconcile(ctx, r)
	if errors.Is(e, errRequeue) {
		return nil
	}
	if e != nil {
		status = ready(r, prior(r), false, "ReconcileFailed", e.Error())
	}
	if write := c.API.Status(ctx, r, status); write != nil {
		return write
	}
	return e
}
func (c *Controller) reconcile(ctx context.Context, r k.Resource) (k.Status, error) {
	status := prior(r)
	if r.APIVersion != v1.APIVersion {
		return status, model.Invalid("unsupported API version")
	}
	if r.Kind == v1.IntentKind {
		return c.intent(ctx, r, status)
	}
	d, e := Native(r)
	if e != nil {
		return status, e
	}
	status.DefinitionDigest = model.DefinitionDigest(d.Manifest())
	if d.Binding != nil {
		return ready(r, status, true, "DefinitionValid", "Measurement declared. Live telemetry is checked when an intent compiles; no reliability health is inferred."), nil
	}
	if d.Journey != nil {
		for _, link := range d.Journey.Spec.SLOs {
			linked, e := c.API.Get(ctx, v1.SLOKind, link.ID)
			if e != nil {
				return status, fmt.Errorf("linked SLO %s is unavailable", link.ID)
			}
			if linked.Metadata.Annotations[k.ProjectAnnotation] != d.Metadata().Project {
				return status, model.Invalid("linked SLO project does not match this critical user journey")
			}
			if _, e = Native(linked); e != nil {
				return status, e
			}
		}
		return ready(r, status, true, "ReferencesValid", "Critical user journey references resolve. This is configuration status, not measured journey health."), nil
	}
	measurement := service.MeasurementDigest(d.SLO.SLO())
	if status.MeasurementDigest != "" && status.MeasurementDigest != measurement {
		return status, model.Invalid("measurement identity changed; create a new metadata.name instead of reusing interval history")
	}
	status.MeasurementDigest = measurement
	source := rules.ConnectionID(d.SLO.SLO())
	target, ok := c.Config.Sources[source]
	if !ok {
		return status, model.Invalid("source is not configured for this namespace")
	}
	route := model.DefinitionDigest(struct{ Source, Destination string }{source, target.Digest()})
	if status.ManagedGroup != "" && (status.DestinationDigest != route || status.Source != source) {
		return status, model.Invalid("source/destination changes require deleting this resource with its original route configuration before recreating it")
	}
	if target.Kind == "prometheus-operator" {
		rendered, e := PrometheusRule(*d.SLO, c.Config.Tenant, c.Config.Namespace, r.Metadata.UID, target.PrometheusLabels, r.Metadata.Generation)
		if e != nil {
			return status, e
		}
		existing, inspection := c.API.Get(ctx, "PrometheusRule", rendered.Metadata.Name)
		if inspection != nil && !code(inspection, 404) {
			return status, inspection
		}
		if inspection == nil && owned(existing, r.Metadata.UID) && existing.Metadata.Annotations["nines.dev/measurement-digest"] != measurement {
			return status, model.Invalid("existing rule measurement differs; create a new SLO identity")
		}
		if e = c.API.ApplyOwned(ctx, rendered, r.Metadata.UID); e != nil {
			return status, e
		}
		status.DestinationDigest = route
		status.Source = source
		status.ManagedGroup = rendered.Metadata.Name
		status.RuleDigest = CanonicalDigest(rendered.Spec)
		return ready(r, status, true, "RulesConfigured", "PrometheusRule is configured. Prometheus selection, evaluation and sample availability must be monitored separately."), nil
	}
	if !slices.Contains(r.Metadata.Finalizers, k.Finalizer) {
		if e = c.API.Finalizer(ctx, r, true); e != nil {
			return status, e
		}
		return status, errRequeue
	}
	configured, e := target.Target(source)
	if e != nil {
		return status, e
	}
	manager := rules.Manager{BeforeWrite: c.API.BeforeWrite, Tenant: c.Config.Tenant, OwnerUID: r.Metadata.UID, Targets: map[string]rules.Target{source: configured}}
	slo := d.SLO.SLO()
	slo.Revision = r.Metadata.Generation
	plan, e := manager.Plan(slo)
	if e != nil {
		return status, e
	}
	if status.ManagedGroup == "" || status.RuleDigest != plan.Digest || status.ObservedGeneration != r.Metadata.Generation {
		status.DestinationDigest = route
		status.Source = source
		status.ManagedGroup = plan.Group.Name
		status.RuleDigest = plan.Digest
		// The caller persists this destination intent before any Mimir write. A crash
		// after acceptance can therefore always recover the exact cleanup destination.
		return ready(r, status, false, "ApplyingRules", "Rule destination and digest recorded; awaiting Mimir acceptance."), nil
	}
	if _, e = manager.Apply(ctx, slo, plan.Digest); e != nil {
		return status, e
	}
	return ready(r, status, true, "RulesAccepted", "Mimir accepted the owned rule group. This does not establish evaluation success or measured reliability."), nil
}
func (c *Controller) cleanup(ctx context.Context, r k.Resource) error {
	if !slices.Contains(r.Metadata.Finalizers, k.Finalizer) {
		return nil
	}
	if r.Kind != v1.SLOKind {
		return model.Invalid("unexpected cleanup finalizer")
	}
	status := prior(r)
	if status.ManagedGroup != "" {
		source, ok := c.Config.Sources[status.Source]
		if !ok || source.Kind != "mimir" || status.DestinationDigest != model.DefinitionDigest(struct{ Source, Destination string }{status.Source, source.Digest()}) {
			return model.Invalid("restore the original configured Mimir route before cleanup")
		}
		target, e := source.Target(status.Source)
		if e != nil {
			return e
		}
		manager := rules.Manager{BeforeWrite: c.API.BeforeWrite, Tenant: c.Config.Tenant, OwnerUID: r.Metadata.UID, Targets: map[string]rules.Target{status.Source: target}}
		if e = manager.RemoveOwned(ctx, status.Source, status.ManagedGroup, r.Metadata.Name); e != nil {
			return e
		}
	}
	return c.API.Finalizer(ctx, r, false)
}
func (c *Controller) intent(ctx context.Context, r k.Resource, status k.Status) (k.Status, error) {
	var spec k.IntentSpec
	decoder := json.NewDecoder(bytes.NewReader(r.Spec))
	decoder.DisallowUnknownFields()
	if e := decoder.Decode(&spec); e != nil {
		return status, model.Invalid("invalid intent specification")
	}
	goal := v1.SLOIntent{APIVersion: r.APIVersion, Kind: r.Kind, Metadata: v1.Metadata{Name: r.Metadata.Name, Project: r.Metadata.Annotations[k.ProjectAnnotation]}, Spec: spec.IntentSpec}
	if e := model.ValidateIntent(goal); e != nil {
		return status, e
	}
	binding, e := c.API.Get(ctx, v1.BindingKind, spec.MeasurementSelection.Name)
	if e != nil {
		return status, fmt.Errorf("selected measurement binding is unavailable")
	}
	if spec.MeasurementSelection.ExpectedGeneration < 1 || binding.Metadata.Generation != spec.MeasurementSelection.ExpectedGeneration {
		return status, model.Invalid("select the binding's exact Kubernetes generation")
	}
	bd, e := Native(binding)
	if e != nil {
		return status, e
	}
	if bd.Binding == nil || !model.IntentMatchesBinding(goal, *bd.Binding) {
		return status, model.Invalid("measurement does not match the exact intent population and semantics")
	}
	if ref := goal.Spec.Journey; ref != nil {
		j, e := c.API.Get(ctx, v1.JourneyKind, ref.Name)
		if e != nil {
			return status, fmt.Errorf("selected critical user journey is unavailable")
		}
		if j.Metadata.Generation != ref.ExpectedRevision {
			return status, model.Invalid("critical user journey generation changed")
		}
		jd, e := Native(j)
		if e != nil {
			return status, e
		}
		if jd.Metadata().Project != goal.Metadata.Project {
			return status, model.Invalid("critical user journey project differs")
		}
		stepOK, linkOK := false, false
		for _, step := range jd.Journey.Spec.Steps {
			if step.ID == ref.Step && slices.Contains(step.SLORefs, goal.Metadata.Name) {
				stepOK = true
			}
		}
		for _, link := range jd.Journey.Spec.SLOs {
			if link.ID == goal.Metadata.Name && link.Dimension == goal.Spec.Outcome && link.Required == ref.Required {
				linkOK = true
			}
		}
		if !stepOK || !linkOK {
			return status, model.Invalid("commit the reviewed critical user journey and step link in GitOps desired state")
		}
	}
	source, ok := c.Config.Sources[goal.Spec.Source]
	if !ok {
		return status, model.Invalid("intent source is not configured")
	}
	input := model.DefinitionDigest(struct {
		Version    string
		Goal       v1.SLOIntent
		Binding    model.MeasurementBinding
		Generation int64
		Route      string
	}{"operator/1", goal, *bd.Binding, binding.Metadata.Generation, source.Digest()})
	child, childErr := c.API.Get(ctx, v1.SLOKind, r.Metadata.Name)
	if childErr == nil && !owned(child, r.Metadata.UID) {
		return status, ErrOwnership
	}
	if childErr != nil && !code(childErr, 404) {
		return status, childErr
	}
	cached := false
	if childErr == nil && status.InputDigest == input && status.ValidatedAt != nil && time.Since(*status.ValidatedAt) < 5*time.Minute {
		native, e := Native(child)
		cached = e == nil && model.DefinitionDigest(native.Manifest()) == status.DefinitionDigest
	}
	if !cached {
		collector, e := source.Collector()
		if e != nil {
			return status, e
		}
		manifest, _ := json.Marshal(bd.Binding)
		// The immutable compiler snapshot exposes the Kubernetes generation as
		// the binding revision without manufacturing intermediate revisions.
		st := &bindingSnapshot{Memory: store.NewMemory(), row: model.TopologyRevision{Kind: v1.BindingKind, ID: binding.Metadata.Name, Revision: binding.Metadata.Generation, Manifest: manifest, Digest: model.DefinitionDigest(*bd.Binding)}}
		platform := service.New(st, collector)
		platform.TenantID = c.Config.Tenant
		platform.Sources = map[string]service.MetricSource{goal.Spec.Source: {Kind: source.Kind, Collector: collector}}
		compiled, e := platform.CompileBinding(ctx, binding.Metadata.Name, v1.CompileBindingInput{ExpectedRevision: binding.Metadata.Generation, Name: goal.Metadata.Name, DisplayName: goal.Metadata.Name, Target: goal.Spec.Target, WindowSeconds: goal.Spec.WindowSeconds, Policies: goal.Spec.Policies, Policy: goal.Spec.Policy})
		if e != nil {
			return status, e
		}
		compiled.Manifest.Spec.Intent = &goal.Spec
		definition := model.SLODefinition(compiled.Manifest)
		if e = definition.Validate(); e != nil {
			return status, e
		}
		if childErr == nil {
			previous, err := Native(child)
			if err != nil {
				return status, err
			}
			if previous.SLO == nil || service.MeasurementDigest(previous.SLO.SLO()) != service.MeasurementDigest(definition.SLO()) {
				return status, model.Invalid("measurement changed; start a new intent and SLO identity")
			}
		}
		desired, e := FromDefinition(model.Definition{SLO: &definition}, c.Config.Namespace)
		if e != nil {
			return status, e
		}
		desired.Metadata.OwnerReferences = []k.OwnerReference{{APIVersion: r.APIVersion, Kind: r.Kind, Name: r.Metadata.Name, UID: r.Metadata.UID, Controller: true}}
		if e = c.API.ApplyOwned(ctx, desired, r.Metadata.UID); e != nil {
			return status, e
		}
		now := time.Now().UTC()
		status.ValidatedAt = &now
		status.BindingGeneration = binding.Metadata.Generation
		status.InputDigest = input
		status.DefinitionDigest = model.DefinitionDigest(definition)
	}
	return ready(r, status, true, "IntentCompiled", "Verified measurement compiled to an owned SLO. Critical user journey links remain GitOps-owned; child SLO status reports rule deployment."), nil
}

type bindingSnapshot struct {
	*store.Memory
	row model.TopologyRevision
}

func (s *bindingSnapshot) ReadTopology(ctx context.Context, kind, id string, revision int64) (model.TopologyRevision, error) {
	if err := ctx.Err(); err != nil {
		return model.TopologyRevision{}, err
	}
	if kind != s.row.Kind || id != s.row.ID || (revision != 0 && revision != s.row.Revision) {
		return model.TopologyRevision{}, model.ErrNotFound
	}
	return s.row, nil
}
