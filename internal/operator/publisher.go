package operator

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"sync"
	"time"

	k "github.com/ishantanu/nines/api/kubernetes/v1alpha1"
	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

// Publisher connects reviewed server rule applies to namespace-scoped SLO CRs.
// Cluster routes and credentials come from the server, never a portal request.
type Publisher struct {
	Store   store.Store
	Tenant  string
	APIFor  func(string) (*API, error) // Tests may supply only their disposable API.
	clients sync.Map
}

func (p *Publisher) client(target rules.Target) (*API, error) {
	if !tenancy.ValidID(p.Tenant) || !tenancy.ValidID(target.Namespace) || !tenancy.ValidID(target.Publisher) || target.Kind != "operator" || (target.ProviderKind != "prometheus" && target.ProviderKind != "mimir") {
		return nil, model.Invalid("invalid trusted operator destination")
	}
	if p.APIFor != nil {
		api, err := p.APIFor(target.Namespace)
		if err != nil {
			return nil, err
		}
		if api == nil || api.Namespace != target.Namespace {
			return nil, model.Invalid("operator publisher namespace mismatch")
		}
		return api, nil
	}
	if cached, ok := p.clients.Load(target.Namespace); ok {
		return cached.(*API), nil
	}
	api, err := InCluster(target.Namespace)
	if err != nil {
		return nil, fmt.Errorf("operator publishing requires in-cluster service-account configuration: %w", err)
	}
	cached, _ := p.clients.LoadOrStore(target.Namespace, api)
	return cached.(*API), nil
}
func (p *Publisher) desired(s model.SLO, target rules.Target) (k.Resource, error) {
	definition := model.DefinitionFromSLO(s)
	resource, err := FromDefinition(model.Definition{SLO: &definition}, target.Namespace)
	if err != nil {
		return resource, err
	}
	resource.Metadata.Annotations[k.PortalProvider] = target.ProviderKind
	resource.Metadata.Annotations[k.PortalOrg] = target.MimirOrgID
	resource.Metadata.Annotations[k.PortalPublisher] = target.Publisher
	resource.Metadata.Annotations[k.PortalTenant] = p.Tenant
	resource.Metadata.Annotations[k.PortalRevision] = strconv.FormatInt(model.ConfigRevision(s), 10)
	resource.Metadata.Annotations[k.PortalDefinition] = model.DefinitionDigest(definition)
	return resource, nil
}
func (p *Publisher) owns(r k.Resource, target rules.Target, s model.SLO) bool {
	return r.APIVersion == v1.APIVersion && r.Kind == v1.SLOKind && r.Metadata.Namespace == target.Namespace && r.Metadata.Name == s.ID && r.Metadata.UID != "" && r.Metadata.Annotations[k.PortalPublisher] == target.Publisher && r.Metadata.Annotations[k.PortalTenant] == p.Tenant && r.Metadata.Annotations[k.ProjectAnnotation] == s.Project && len(r.Metadata.OwnerReferences) == 0
}
func samePublished(current, desired k.Resource) bool {
	if CanonicalDigest(current.Spec) != CanonicalDigest(desired.Spec) {
		return false
	}
	for _, key := range []string{k.ProjectAnnotation, k.PortalPublisher, k.PortalTenant, k.PortalRevision, k.PortalDefinition, k.PortalProvider, k.PortalOrg} {
		if current.Metadata.Annotations[key] != desired.Metadata.Annotations[key] {
			return false
		}
	}
	return current.Metadata.DeletionTimestamp == nil
}
func (p *Publisher) Plan(ctx context.Context, s model.SLO, target rules.Target) (rules.Plan, error) {
	api, err := p.client(target)
	if err != nil {
		return rules.Plan{}, err
	}
	desired, err := p.desired(s, target)
	if err != nil {
		return rules.Plan{}, err
	}
	plan := rules.Plan{Target: target, Resource: &desired, SLORevision: model.ConfigRevision(s)}
	current, err := api.Get(ctx, v1.SLOKind, s.ID)
	if err != nil && !code(err, 404) {
		return plan, err
	}
	if err == nil {
		if !p.owns(current, target, s) {
			return plan, model.Invalid("refusing to replace a Kubernetes SLO owned by GitOps, another tenant or another publisher")
		}
		if current.Metadata.DeletionTimestamp != nil {
			return plan, model.Invalid("operator cleanup is in progress; wait before publishing")
		}
		if _, parseErr := Native(current); parseErr != nil {
			return plan, parseErr
		}
		revision, parseErr := strconv.ParseInt(current.Metadata.Annotations[k.PortalRevision], 10, 64)
		if parseErr != nil || revision > model.ConfigRevision(s) {
			return plan, model.ErrRevisionConflict
		}
		plan.PriorUID = current.Metadata.UID
		plan.PriorGeneration = current.Metadata.Generation
		plan.PriorSpecDigest = CanonicalDigest(current.Spec)
		plan.PriorRevision = current.Metadata.Annotations[k.PortalRevision]
	}
	slo := s
	slo.Revision = model.ConfigRevision(s)
	plan.Group, err = rules.Generate(slo, p.Tenant)
	if plan.PriorUID != "" {
		plan.Group, err = rules.GenerateOwned(slo, p.Tenant, plan.PriorUID)
	}
	if err != nil {
		return plan, err
	}
	body, err := YAML(rules.File{Groups: []rules.Group{plan.Group}})
	if err != nil {
		return plan, err
	}
	plan.YAML = string(body)
	body, err = YAML(desired)
	if err != nil {
		return plan, err
	}
	plan.ResourceYAML = string(body)
	plan.Digest = model.DefinitionDigest(struct {
		Tenant string
		Plan   rules.Plan
	}{p.Tenant, plan})
	return plan, nil
}
func (p *Publisher) Apply(ctx context.Context, s model.SLO, plan rules.Plan) (rules.Deployment, error) {
	result := rules.Deployment{State: "submitted", Digest: plan.Digest, Target: plan.Target, Group: plan.Group.Name, Message: "The reviewed SLO was submitted to Kubernetes. Operator reconciliation and provider evaluation are separate."}
	if p.Store == nil {
		return result, model.ErrUnavailable
	}
	if err := p.Store.Update(ctx, s.ID, func(record *model.Record) error {
		if model.ConfigRevision(record.SLO) != model.ConfigRevision(s) {
			return model.ErrRevisionConflict
		}
		route := model.DefinitionDigest(plan.Target)
		if record.OperatorRoute != "" && record.OperatorRoute != route {
			return model.Invalid("operator destination is pinned; withdraw its owned resource before changing routes")
		}
		record.OperatorRoute = route
		return nil
	}); err != nil {
		return result, err
	}
	api, err := p.client(plan.Target)
	if err != nil {
		return result, err
	}
	desired, err := p.desired(s, plan.Target)
	if err != nil {
		return result, err
	}
	current, err := api.Get(ctx, v1.SLOKind, s.ID)
	method, path := "POST", api.path(v1.SLOKind)
	if code(err, 404) {
		if plan.PriorUID != "" {
			return result, model.ErrRevisionConflict
		}
	} else if err != nil {
		return result, err
	} else {
		if !p.owns(current, plan.Target, s) {
			return result, ErrOwnership
		}
		if current.Metadata.UID != plan.PriorUID || current.Metadata.Generation != plan.PriorGeneration || CanonicalDigest(current.Spec) != plan.PriorSpecDigest || current.Metadata.Annotations[k.PortalRevision] != plan.PriorRevision || current.Metadata.DeletionTimestamp != nil {
			return result, model.ErrRevisionConflict
		}
		if samePublished(current, desired) {
			result.State = "unchanged"
			result.Message = "The published Kubernetes SLO already matches this portal revision."
			return p.result(ctx, s, plan.Target, result), nil
		}
		metadata := current.Metadata
		metadata.Annotations = maps.Clone(current.Metadata.Annotations)
		for key, value := range desired.Metadata.Annotations {
			metadata.Annotations[key] = value
		}
		desired.Metadata = metadata
		method, path = "PUT", api.path(v1.SLOKind)+"/"+s.ID
	}
	if err := api.request(ctx, method, path, desired, nil); err != nil {
		// A failed acknowledgement is inspected once. Never repeat the mutation.
		probe, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		observed, readErr := api.Get(probe, v1.SLOKind, s.ID)
		if readErr != nil || !p.owns(observed, plan.Target, s) || !samePublished(observed, desired) || (plan.PriorUID != "" && observed.Metadata.UID != plan.PriorUID) {
			return result, fmt.Errorf("operator publication outcome is unverified; inspect current resources before retrying: %w", model.ErrUnavailable)
		}
		result.Message = "Kubernetes acknowledgement was ambiguous; exact owned publication was verified by readback. No write was repeated."
	}
	return p.result(ctx, s, plan.Target, result), nil
}
func stage(state, code, message, action string) v1.IntegrationStage {
	return v1.IntegrationStage{State: state, Code: code, Message: message, NextAction: action}
}
func (p *Publisher) result(ctx context.Context, s model.SLO, target rules.Target, result rules.Deployment) rules.Deployment {
	if identity, err := p.Identity(ctx, s, target); err == nil {
		if group, err := rules.GenerateOwned(s, p.Tenant, identity.ResourceUID); err == nil {
			result.Group = group.Name
		}
	} else {
		result.Group = ""
	}
	deployment, evaluation := p.Inspect(ctx, s, target, time.Now().UTC())
	activation := stage("accepted", "operator_submission_pending", "The operator will reconcile the reviewed Kubernetes resource.", "Check reconciliation, runtime evaluation and recorded output separately.")
	if deployment.State == "configured" || deployment.State == "stored" {
		activation = stage("verified", "operator_reconciled", "The operator reports this portal revision configured.", "Provider evaluation, measurement coverage and alert delivery remain separate.")
	}
	if deployment.State == "blocked" || deployment.State == "failed" {
		activation = deployment
	}
	result.Activation = &activation
	result.Evaluation = &evaluation
	return result
}
func (p *Publisher) Inspect(ctx context.Context, s model.SLO, target rules.Target, now time.Time) (v1.IntegrationStage, v1.IntegrationStage) {
	unknown := stage("unknown", "operator_evaluation_unverified", "Current operator evaluation has not been verified.", "Check namespace RBAC, operator availability and the current portal revision.")
	if err := p.CheckRoute(ctx, s, target); err != nil {
		return stage("blocked", "operator_route_pinned", "Operator publication is bound to its original destination.", "Restore the original connection route and withdraw its owned publication before changing destinations."), unknown
	}
	api, err := p.client(target)
	if err != nil {
		return stage("unknown", "operator_api_unavailable", "The configured Kubernetes API is unavailable.", "Check server service-account configuration and namespace permissions."), unknown
	}
	current, err := api.Get(ctx, v1.SLOKind, s.ID)
	if code(err, 404) {
		return stage("not_deployed", "operator_resource_missing", "This SLO has not been published to the configured namespace.", "Review and apply its operator deployment plan."), unknown
	}
	if err != nil {
		return stage("unknown", "operator_api_unavailable", "The configured Kubernetes resource could not be inspected.", "Check namespace RBAC and API availability."), unknown
	}
	if !p.owns(current, target, s) {
		return stage("blocked", "operator_ownership_conflict", "This Kubernetes SLO belongs to another owner.", "Resolve ownership explicitly; Nines will not overwrite GitOps or another publisher."), unknown
	}
	desired, err := p.desired(s, target)
	if err != nil {
		return stage("unknown", "operator_plan_invalid", "The current definition cannot be published.", "Validate the definition and trusted destination."), unknown
	}
	if current.Metadata.DeletionTimestamp != nil {
		return stage("blocked", "operator_cleanup_pending", "Operator resource cleanup is in progress.", "Keep its original route and credentials available until finalizers complete."), unknown
	}
	if !samePublished(current, desired) {
		return stage("out_of_date", "operator_resource_out_of_date", "The published resource differs from this portal revision.", "Review and apply the current definition. Saving an edit does not deploy it."), unknown
	}
	pending := stage("submitted", "operator_reconciliation_pending", "The current portal revision is published; reconciliation is pending.", "Wait for the operator to observe this revision; inspect controller availability and project scope if it persists.")
	status := current.Status
	if status == nil || status.ObservedGeneration != current.Metadata.Generation {
		return pending, unknown
	}
	for _, condition := range status.Conditions {
		if condition.Type != "Ready" || condition.ObservedGeneration != current.Metadata.Generation {
			continue
		}
		age := now.Sub(condition.LastTransitionTime)
		if condition.LastTransitionTime.IsZero() || age > 2*time.Minute || age < -5*time.Second {
			return stage("submitted", "operator_status_stale", "Published configuration exists, but operator evidence is stale.", "Check controller leadership and scheduling, then refresh status."), unknown
		}
		if condition.Status != "True" {
			return stage("failed", "operator_reconcile_failed", "The operator has not configured this revision successfully.", "Inspect the SLO conditions and controller logs for project, route or ownership failures."), unknown
		}
		if status.RuleRevision != model.ConfigRevision(s) || status.DefinitionDigest != desired.Metadata.Annotations[k.PortalDefinition] || status.Deployment == nil || status.Evaluation == nil {
			return pending, unknown
		}
		evaluation := *status.Evaluation
		if evaluation.State == "evaluating" && (evaluation.ObservedAt == nil || now.Sub(*evaluation.ObservedAt) > max(time.Minute, 3*time.Duration(s.StepSeconds)*time.Second) || evaluation.ObservedAt.After(now.Add(5*time.Second))) {
			evaluation = stage("stale", "operator_evaluation_stale", "Operator-reported runtime evidence is stale.", "Check provider scheduling and refresh operator status.")
		}
		return *status.Deployment, evaluation
	}
	return pending, unknown
}
func (p *Publisher) Identity(ctx context.Context, s model.SLO, target rules.Target) (model.RuleOutputIdentity, error) {
	if err := p.CheckRoute(ctx, s, target); err != nil {
		return model.RuleOutputIdentity{}, err
	}
	api, err := p.client(target)
	if err != nil {
		return model.RuleOutputIdentity{}, err
	}
	current, err := api.Get(ctx, v1.SLOKind, s.ID)
	if err != nil {
		return model.RuleOutputIdentity{}, err
	}
	desired, err := p.desired(s, target)
	if err != nil || !p.owns(current, target, s) || !samePublished(current, desired) {
		return model.RuleOutputIdentity{}, model.ErrRevisionConflict
	}
	return rules.OutputIdentity(s, p.Tenant, current.Metadata.UID), nil
}

// RuleRevision binds server-published resources to their reviewed portal
// revision. Standalone GitOps resources continue using Kubernetes generation.
func RuleRevision(r k.Resource, tenant string) (int64, error) {
	annotations := r.Metadata.Annotations
	if annotations[k.PortalPublisher] == "" {
		if annotations[k.PortalTenant] != "" || annotations[k.PortalRevision] != "" || annotations[k.PortalDefinition] != "" || annotations[k.PortalProvider] != "" || annotations[k.PortalOrg] != "" {
			return 0, model.Invalid("incomplete portal publication identity")
		}
		return r.Metadata.Generation, nil
	}
	if !tenancy.ValidID(annotations[k.PortalPublisher]) || annotations[k.PortalTenant] != tenant {
		return 0, model.Invalid("portal publication tenant does not match this operator")
	}
	revision, err := strconv.ParseInt(annotations[k.PortalRevision], 10, 64)
	if err != nil || revision < 1 {
		return 0, model.Invalid("invalid portal publication revision")
	}
	native, err := Native(r)
	if err != nil || native.SLO == nil || annotations[k.PortalDefinition] != model.DefinitionDigest(native.Manifest()) {
		return 0, model.Invalid("portal definition digest does not match the resource")
	}
	return revision, nil
}

// CheckRoute preserves the original destination through restarts and failed
// acknowledgements. It also fences switching back to a direct provider writer.
func (p *Publisher) CheckRoute(ctx context.Context, s model.SLO, target rules.Target) error {
	if p.Store == nil {
		return model.ErrUnavailable
	}
	var route string
	var err error
	if reader, ok := p.Store.(interface {
		OperatorRoute(context.Context, string) (string, error)
	}); ok {
		route, err = reader.OperatorRoute(ctx, s.ID)
	} else {
		var record model.Record
		record, err = p.Store.Read(ctx, s.ID)
		route = record.OperatorRoute
	}
	if err != nil {
		return err
	}
	if route != "" && route != model.DefinitionDigest(target) {
		return model.Invalid("operator destination is pinned; withdraw its owned resource before changing routes")
	}
	return nil
}

// Withdraw removes only the reviewed parent. Mimir finalizers and Kubernetes
// garbage collection remain owned by the operator. The server route stays pinned
// until the parent is confirmed absent, including after ambiguous responses.
func (p *Publisher) Withdraw(ctx context.Context, s model.SLO, plan rules.Plan) (rules.Deployment, error) {
	result := rules.Deployment{State: "withdrawing", Digest: plan.Digest, Target: plan.Target, Group: plan.Group.Name, Message: "Operator withdrawal is pending. The original route remains pinned until owned cleanup completes."}
	activation := stage("pending", "operator_withdrawal_pending", "Owned cleanup is pending; the destination remains pinned.", "Wait for operator finalizers, then refresh/review the plan and withdraw again to confirm absence and release the route.")
	evaluation := stage("unknown", "operator_withdrawal_runtime_unverified", "Provider runtime removal has not been verified.", "Check backend rule removal separately; recorded historical samples can remain after withdrawal.")
	result.Activation = &activation
	result.Evaluation = &evaluation
	if err := p.CheckRoute(ctx, s, plan.Target); err != nil {
		return result, err
	}
	api, err := p.client(plan.Target)
	if err != nil {
		return result, err
	}
	current, err := api.Get(ctx, v1.SLOKind, s.ID)
	if err == nil {
		if !p.owns(current, plan.Target, s) {
			return result, ErrOwnership
		}
		if current.Metadata.UID != plan.PriorUID || current.Metadata.Generation != plan.PriorGeneration || CanonicalDigest(current.Spec) != plan.PriorSpecDigest || current.Metadata.Annotations[k.PortalRevision] != plan.PriorRevision {
			return result, model.ErrRevisionConflict
		}
		if current.Metadata.DeletionTimestamp == nil {
			options := map[string]any{"apiVersion": "v1", "kind": "DeleteOptions", "preconditions": map[string]string{"uid": current.Metadata.UID, "resourceVersion": current.Metadata.ResourceVersion}, "propagationPolicy": "Foreground"}
			if err := api.request(ctx, "DELETE", api.path(v1.SLOKind)+"/"+s.ID, options, nil); err != nil {
				return result, fmt.Errorf("withdrawal acknowledgement is unverified; inspect cleanup before retrying: %w", model.ErrUnavailable)
			}
		}
	} else if !code(err, 404) {
		return result, err
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for {
		_, err = api.Get(bounded, v1.SLOKind, s.ID)
		if code(err, 404) {
			if err := p.Store.Update(ctx, s.ID, func(record *model.Record) error {
				if model.ConfigRevision(record.SLO) != model.ConfigRevision(s) {
					return model.ErrRevisionConflict
				}
				if record.OperatorRoute != "" && record.OperatorRoute != model.DefinitionDigest(plan.Target) {
					return model.ErrRevisionConflict
				}
				record.OperatorRoute = ""
				return nil
			}); err != nil {
				return result, err
			}
			activation = stage("verified", "operator_withdrawn", "The owned Kubernetes publication is absent and its destination pin is released.", "Inspect provider runtime removal separately. The portal definition and historical measurements are retained.")
			result.State = "withdrawn"
			result.Message = "The operator resource and its foreground cleanup are complete. The portal definition is retained and its deployment route is released."
			return result, nil
		}
		if err != nil {
			return result, nil
		}
		select {
		case <-bounded.Done():
			return result, nil
		case <-time.After(300 * time.Millisecond):
		}
	}
}
