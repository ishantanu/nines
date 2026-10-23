package operator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
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
	// Fence each mutation against the current parent UID, spec generation,
	// project and deletion state as well as the namespace lease. A stale list
	// snapshot may not overwrite rules for a newer reviewed desired definition.
	scoped := *c
	api := *c.API
	leaseGuard := api.BeforeWrite
	api.BeforeWrite = func() error {
		if leaseGuard != nil {
			if err := leaseGuard(); err != nil {
				return err
			}
		}
		current, err := c.API.Get(ctx, r.Kind, r.Metadata.Name)
		if err != nil {
			return err
		}
		if current.Metadata.UID != r.Metadata.UID || current.Metadata.Generation != r.Metadata.Generation || current.Metadata.Annotations[k.ProjectAnnotation] != r.Metadata.Annotations[k.ProjectAnnotation] || current.Metadata.Annotations[k.PortalPublisher] != r.Metadata.Annotations[k.PortalPublisher] || current.Metadata.Annotations[k.PortalTenant] != r.Metadata.Annotations[k.PortalTenant] || current.Metadata.Annotations[k.PortalRevision] != r.Metadata.Annotations[k.PortalRevision] || current.Metadata.Annotations[k.PortalDefinition] != r.Metadata.Annotations[k.PortalDefinition] || current.Metadata.Annotations[k.PortalProvider] != r.Metadata.Annotations[k.PortalProvider] || current.Metadata.Annotations[k.PortalOrg] != r.Metadata.Annotations[k.PortalOrg] || (current.Metadata.DeletionTimestamp == nil) != (r.Metadata.DeletionTimestamp == nil) {
			return model.ErrRevisionConflict
		}
		if leaseGuard != nil {
			return leaseGuard()
		}
		return nil
	}
	scoped.API = &api
	return scoped.reconcileResource(ctx, r)
}

func (c *Controller) reconcileResource(ctx context.Context, r k.Resource) error {
	if r.Metadata.DeletionTimestamp != nil {
		if err := c.cleanup(ctx, r); err != nil {
			status := ready(r, prior(r), false, "CleanupBlocked", "Owned rule cleanup is blocked. Restore the original route and credentials; inspect ownership before retrying. Do not remove the finalizer to bypass cleanup.")
			if write := c.API.Status(ctx, r, status); write != nil {
				return write
			}
			return err
		}
		return nil
	}
	if !c.Config.Allows(r.Metadata.Annotations[k.ProjectAnnotation]) {
		status := prior(r)
		status.Deployment = nil
		status.Evaluation = nil
		status.RuleOutput = nil
		return c.API.Status(ctx, r, ready(r, status, false, "ScopeDenied", "Project is not allowed in this operator namespace."))
	}
	status, e := c.reconcile(ctx, r)
	if errors.Is(e, errRequeue) {
		return nil
	}
	if e != nil {
		status = ready(r, status, false, "ReconcileFailed", e.Error())
		status.Deployment = nil
		status.Evaluation = nil
		status.RuleOutput = nil
	}
	if write := c.API.Status(ctx, r, status); write != nil {
		return write
	}
	return e
}
func (c *Controller) reconcile(ctx context.Context, r k.Resource) (k.Status, error) {
	status := prior(r)
	status.Deployment = nil
	status.Evaluation = nil
	status.RuleOutput = nil
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
	revision, err := RuleRevision(r, c.Config.Tenant)
	if err != nil {
		return status, err
	}
	measurement := service.MeasurementDigest(d.SLO.SLO())
	status.MeasurementDigest = measurement
	source := rules.ConnectionID(d.SLO.SLO())
	target, ok := c.Config.Sources[source]
	if !ok {
		return status, model.Invalid("source is not configured for this namespace")
	}
	if r.Metadata.Annotations[k.PortalPublisher] != "" {
		provider := target.Kind
		if provider == "prometheus-operator" {
			provider = "prometheus"
		}
		if r.Metadata.Annotations[k.PortalProvider] != provider || r.Metadata.Annotations[k.PortalOrg] != target.MimirOrgID {
			return status, model.Invalid("portal source provider or Mimir organization does not match this operator destination")
		}
	}
	route := model.DefinitionDigest(struct{ Source, Destination string }{source, target.Digest()})
	if status.ManagedGroup != "" && (status.DestinationDigest != route || status.Source != source) {
		return status, model.Invalid("source/destination changes require deleting this resource with its original route configuration before recreating it")
	}
	if target.Kind == "prometheus-operator" {
		rendered, e := PrometheusRule(*d.SLO, c.Config.Tenant, c.Config.Namespace, r.Metadata.UID, target.PrometheusLabels, revision)
		if e != nil {
			return status, e
		}
		_, inspection := c.API.Get(ctx, "PrometheusRule", rendered.Metadata.Name)
		if inspection != nil && !code(inspection, 404) {
			return status, inspection
		}
		if e = c.API.ApplyOwned(ctx, rendered, r.Metadata.UID); e != nil {
			return status, e
		}
		status.DestinationDigest = route
		status.Source = source
		status.ManagedGroup = rendered.Metadata.Name
		status.RuleDigest = CanonicalDigest(rendered.Spec)
		slo := d.SLO.SLO()
		slo.Revision = revision
		var file rules.File
		if err := json.Unmarshal(rendered.Spec, &file); err != nil {
			return status, err
		}
		deployed := v1.IntegrationStage{State: "configured", Code: "prometheus_rule_configured", Message: "The owned PrometheusRule matches the current generation.", NextAction: "Check selector, runtime and query-output evidence separately."}
		status.Deployment = &deployed
		status.RuleRevision = revision
		c.observe(ctx, slo, target, file.Groups[0], r.Metadata.UID, &status)
		return ready(r, status, true, "RulesConfigured", "The current PrometheusRule is configured. Evaluation and query-output evidence are reported separately; Ready does not mean reliability health or alert delivery."), nil
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
	slo.Revision = revision
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
	status.RuleRevision = revision
	deployment, _ := manager.Inspect(ctx, slo, time.Now().UTC())
	status.Deployment = &deployment
	c.observe(ctx, slo, target, plan.Group, r.Metadata.UID, &status)
	return ready(r, status, true, "RulesAccepted", "The current owned Mimir rule group is configured. Evaluation and query-output evidence are reported separately."), nil
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
	if goal.Spec.Measurement != nil && (spec.MeasurementSelection.Name != "" || spec.MeasurementSelection.ExpectedGeneration != 0) {
		return status, model.Invalid("inline measurement cannot be combined with a binding selection")
	}
	var binding k.Resource
	var bd model.Definition
	var e error
	if goal.Spec.Measurement == nil {
		binding, e = c.API.Get(ctx, v1.BindingKind, spec.MeasurementSelection.Name)
		if e != nil {
			return status, fmt.Errorf("selected measurement binding is unavailable")
		}
		if spec.MeasurementSelection.ExpectedGeneration < 1 || binding.Metadata.Generation != spec.MeasurementSelection.ExpectedGeneration {
			return status, model.Invalid("select the binding's exact Kubernetes generation")
		}
		bd, e = Native(binding)
		if e != nil {
			return status, e
		}
		if bd.Binding == nil || !model.IntentMatchesBinding(goal, *bd.Binding) {
			return status, model.Invalid("measurement does not match the exact intent population and semantics")
		}

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
		Binding    *model.MeasurementBinding
		Generation int64
		Route      string
	}{"operator/1", goal, bd.Binding, binding.Metadata.Generation, source.Digest()})
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
		var st store.Store = store.NewMemory()
		if goal.Spec.Measurement == nil {
			manifest, _ := json.Marshal(bd.Binding)
			st = &bindingSnapshot{Memory: store.NewMemory(), row: model.TopologyRevision{Kind: v1.BindingKind, ID: binding.Metadata.Name, Revision: binding.Metadata.Generation, Manifest: manifest, Digest: model.DefinitionDigest(*bd.Binding)}}
		}
		platform := service.New(st, collector)
		platform.TenantID = c.Config.Tenant
		platform.Sources = map[string]service.MetricSource{goal.Spec.Source: {Kind: source.Kind, Collector: collector}}
		var compiled v1.CompiledBinding
		if goal.Spec.Measurement != nil {
			manifest := v1.NewSLO(goal.Metadata.Name, goal.Metadata.Project, v1.SLOSpec{Measurement: goal.Spec.Measurement, Intent: &goal.Spec, DisplayName: goal.Metadata.Name, Service: goal.Spec.Service, Source: goal.Spec.Source, Target: goal.Spec.Target, WindowSeconds: goal.Spec.WindowSeconds, StepSeconds: goal.Spec.StepSeconds, Policies: goal.Spec.Policies, Policy: goal.Spec.Policy})
			inline, err := platform.CompileMeasurement(ctx, manifest)
			e = err
			compiled.Manifest = inline.Manifest
		} else {
			compiled, e = platform.CompileBinding(ctx, binding.Metadata.Name, v1.CompileBindingInput{ExpectedRevision: binding.Metadata.Generation, Name: goal.Metadata.Name, DisplayName: goal.Metadata.Name, Target: goal.Spec.Target, WindowSeconds: goal.Spec.WindowSeconds, Policies: goal.Spec.Policies, Policy: goal.Spec.Policy})
		}
		if e != nil {
			return status, e
		}
		compiled.Manifest.Spec.Intent = &goal.Spec
		definition := model.SLODefinition(compiled.Manifest)
		if e = definition.Validate(); e != nil {
			return status, e
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

// observe adds independent read-only provider evidence. Configuration readiness
// remains useful when a backend is unavailable; old-generation evidence is cleared.
func (c *Controller) observe(ctx context.Context, slo model.SLO, source Source, group rules.Group, uid string, status *k.Status) {
	probe, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	now := time.Now().UTC()
	var checks sync.WaitGroup
	checks.Go(func() {
		target, err := source.Target(rules.ConnectionID(slo))
		if source.Kind == "prometheus-operator" {
			path := source.QueryBearerTokenFile
			if path == "" {
				path = source.BearerTokenFile
			}
			token, tokenErr := secret(path)
			err = tokenErr
			target = rules.Target{Kind: "prometheus", QueryURL: source.QueryURL, BearerToken: token}
		}
		stage := v1.IntegrationStage{State: "unknown", Code: "evaluator_unavailable", Message: "Runtime evidence could not be verified.", NextAction: "Check the configured evaluator route and credentials."}
		if err == nil {
			stage = rules.InspectEvaluation(probe, slo, group, target, now)
		}
		status.Evaluation = &stage
	})
	checks.Go(func() {
		stage := v1.IntegrationStage{State: "unknown", Code: "rule_output_unavailable", Message: "Query output could not be verified.", NextAction: "Check query credentials, routing and ingestion."}
		collector, err := source.Collector()
		if err == nil {
			stage = collector.RuleOutput(probe, rules.OutputIdentity(slo, c.Config.Tenant, uid), now)
		}
		status.RuleOutput = &stage
	})
	checks.Wait()
}
