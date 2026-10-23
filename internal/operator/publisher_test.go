package operator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	k "github.com/ishantanu/nines/api/kubernetes/v1alpha1"
	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/store"
)

func publisherFixture(t *testing.T) (*Publisher, *rules.Manager, *kubeFixture, model.SLO) {
	t.Helper()
	api, kube := newKube(t)
	st := store.NewMemory()
	s := definition().SLO()
	s.Revision = 7
	if err := st.Create(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	p := &Publisher{Tenant: "commerce", Store: st, APIFor: func(namespace string) (*API, error) { return api, nil }}
	manager := &rules.Manager{Tenant: p.Tenant, External: p, Targets: map[string]rules.Target{"primary": {Kind: "operator", ProviderKind: "prometheus", Connection: "primary", Namespace: "team-commerce", Publisher: "portal", QueryURL: "https://metrics.example"}}}
	return p, manager, kube, s
}
func TestPortalPublicationReviewOwnershipAndRevisionMapping(t *testing.T) {
	ctx := context.Background()
	p, manager, kube, s := publisherFixture(t)
	plan, err := manager.PlanContext(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Resource == nil || plan.ResourceYAML == "" || kube.writeCount() != 0 {
		t.Fatal("planning mutated Kubernetes", plan)
	}
	if _, err := manager.Deploy(ctx, s, "stale"); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal(err)
	}
	result, err := manager.Deploy(ctx, s, plan.Digest)
	if err != nil || result.State != "submitted" || result.Activation.State != "accepted" || result.Evaluation.State != "unknown" {
		t.Fatal(result, err)
	}
	current := kube.get(v1.SLOKind, s.ID)
	revision, err := RuleRevision(current, p.Tenant)
	if err != nil || revision != 7 || current.Metadata.Generation != 1 {
		t.Fatal("portal revision conflated with Kubernetes generation", revision, current, err)
	}
	identity, err := manager.OutputIdentity(ctx, s)
	if err != nil || identity.ResourceUID != current.Metadata.UID || identity.Revision != 7 {
		t.Fatal(identity, err)
	}
	controller := Controller{API: func() *API { api, _ := p.APIFor("team-commerce"); return api }(), Config: Config{Tenant: p.Tenant, Namespace: "team-commerce", Projects: []string{"commerce"}, Sources: map[string]Source{"primary": {Kind: "prometheus-operator", QueryURL: ""}}}}
	if err := controller.Reconcile(ctx, current); err != nil {
		t.Fatal(err)
	}
	current = kube.get(v1.SLOKind, s.ID)
	if current.Status.RuleRevision != 7 || current.Status.ObservedGeneration != 1 {
		t.Fatal(current.Status)
	}
	child := kube.get("PrometheusRule", RuleName(s.ID))
	var file rules.File
	if err := json.Unmarshal(child.Spec, &file); err != nil {
		t.Fatal(err)
	}
	for _, rule := range file.Groups[0].Rules {
		if (rule.Record == model.RuleOutputMetric || rule.Alert != "") && rule.Labels["nines_revision"] != "7" {
			t.Fatal("wrong provider revision", rule)
		}
	}
	stored, evaluation := manager.Inspect(ctx, s, time.Now().UTC())
	if stored.State != "configured" || evaluation.State != "unknown" {
		t.Fatal(stored, evaluation)
	}
	plan, err = manager.PlanContext(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	before := kube.writeCount()
	result, err = manager.Deploy(ctx, s, plan.Digest)
	if err != nil || result.State != "unchanged" || before != kube.writeCount() {
		t.Fatal("unchanged publication wrote again", result, err)
	}
	// A metadata-only portal revision must invalidate prior positive status even
	// though Kubernetes does not increment generation for annotation changes.
	s.Revision = 8
	if err := p.Store.Update(ctx, s.ID, func(r *model.Record) error { r.SLO = s; return nil }); err != nil {
		t.Fatal(err)
	}
	plan, err = manager.PlanContext(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Deploy(ctx, s, plan.Digest); err != nil {
		t.Fatal(err)
	}
	current = kube.get(v1.SLOKind, s.ID)
	if current.Metadata.Generation != 1 {
		t.Fatal("annotation change affected generation")
	}
	if err := controller.Reconcile(ctx, current); err != nil {
		t.Fatal(err)
	}
	if got := kube.get(v1.SLOKind, s.ID).Status.RuleRevision; got != 8 {
		t.Fatal(got)
	}
}

func TestPortalPublisherRejectsForeignResourcesAndReviewedDrift(t *testing.T) {
	ctx := context.Background()
	_, manager, kube, s := publisherFixture(t)
	plan, err := manager.PlanContext(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	foreign := sloResource(t)
	foreign.Metadata.Name = s.ID
	kube.put(foreign)
	if _, err := manager.Deploy(ctx, s, plan.Digest); err == nil || kube.writeCount() != 0 {
		t.Fatal("foreign GitOps resource replaced", err)
	}
	if _, err := manager.PlanContext(ctx, s); err == nil {
		t.Fatal("foreign ownership not surfaced during review")
	}
}

func TestPortalRoutePinSurvivesPublisherRestartAndBlocksDirectWriter(t *testing.T) {
	ctx := context.Background()
	publisher, manager, _, s := publisherFixture(t)
	plan, err := manager.PlanContext(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Deploy(ctx, s, plan.Digest); err != nil {
		t.Fatal(err)
	}
	restarted := &Publisher{Tenant: publisher.Tenant, Store: publisher.Store, APIFor: publisher.APIFor}
	manager.External = restarted
	for _, kind := range []string{"operator", "prometheus", "mimir"} {
		target := manager.Targets["primary"]
		target.Kind = kind
		target.Namespace = "another"
		manager.Targets["primary"] = target
		if _, err := manager.PlanContext(ctx, s); err == nil {
			t.Fatal("pinned route bypassed", kind)
		}
	}
}

func TestPortalStatusCannotReusePreviousRevisionOrOldEvidence(t *testing.T) {
	ctx := context.Background()
	publisher, manager, kube, s := publisherFixture(t)
	plan, _ := manager.PlanContext(ctx, s)
	if _, err := manager.Deploy(ctx, s, plan.Digest); err != nil {
		t.Fatal(err)
	}
	current := kube.get(v1.SLOKind, s.ID)
	now := time.Now().UTC()
	current.Status = &k.Status{ObservedGeneration: current.Metadata.Generation, RuleRevision: 6, DefinitionDigest: current.Metadata.Annotations[k.PortalDefinition], Conditions: []k.Condition{{Type: "Ready", Status: "True", ObservedGeneration: current.Metadata.Generation, LastTransitionTime: now}}, Deployment: &v1.IntegrationStage{State: "configured"}, Evaluation: &v1.IntegrationStage{State: "evaluating", ObservedAt: &now}}
	kube.put(current)
	d, e := manager.Inspect(ctx, s, now)
	if d.State != "submitted" || e.State != "unknown" {
		t.Fatal("old revision looked current", d, e)
	}
	current.Status.RuleRevision = 7
	old := now.Add(-time.Hour)
	current.Status.Evaluation.ObservedAt = &old
	kube.put(current)
	_, e = manager.Inspect(ctx, s, now)
	if e.State != "stale" {
		t.Fatal(e)
	}
	current.Status.Conditions[0].LastTransitionTime = old
	kube.put(current)
	d, e = manager.Inspect(ctx, s, now)
	if d.Code != "operator_status_stale" || e.State != "unknown" {
		t.Fatal(d, e)
	}
	current.Metadata.Annotations[k.PortalTenant] = "another"
	if _, err := RuleRevision(current, publisher.Tenant); err == nil {
		t.Fatal("cross-tenant publication reconciled")
	}
}

func TestPortalWithdrawalRetainsRouteUntilCleanupAndPreservesDefinition(t *testing.T) {
	ctx := context.Background()
	publisher, manager, kube, s := publisherFixture(t)
	plan, err := manager.PlanContext(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Deploy(ctx, s, plan.Digest); err != nil {
		t.Fatal(err)
	}
	current := kube.get(v1.SLOKind, s.ID)
	current.Metadata.Finalizers = []string{k.Finalizer}
	kube.put(current)
	plan, err = manager.PlanContext(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Withdraw(ctx, s, plan.Digest)
	if err != nil || result.State != "withdrawing" {
		t.Fatal(result, err)
	}
	record, err := publisher.Store.Read(ctx, s.ID)
	if err != nil || record.OperatorRoute == "" {
		t.Fatal("route released before finalizer cleanup", err)
	}
	kube.mu.Lock()
	delete(kube.objects, kube.key(v1.SLOKind, s.ID))
	kube.mu.Unlock()
	plan, err = manager.PlanContext(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	result, err = manager.Withdraw(ctx, s, plan.Digest)
	if err != nil || result.State != "withdrawn" {
		t.Fatal(result, err)
	}
	record, err = publisher.Store.Read(ctx, s.ID)
	if err != nil || record.OperatorRoute != "" || record.SLO.ID != s.ID {
		t.Fatal("withdrawal lost definition or retained route", err)
	}
}

type publicationTransport func(*http.Request) (*http.Response, error)

func (f publicationTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPortalAmbiguousWriteUsesExactReadbackWithoutRepeatingMutation(t *testing.T) {
	for _, readable := range []bool{true, false} {
		t.Run(fmt.Sprint(readable), func(t *testing.T) {
			ctx := context.Background()
			publisher, manager, kube, s := publisherFixture(t)
			api, _ := publisher.APIFor("team-commerce")
			transport := api.HTTP.Transport
			if transport == nil {
				transport = http.DefaultTransport
			}
			var posts atomic.Int64
			api.HTTP = &http.Client{Transport: publicationTransport(func(request *http.Request) (*http.Response, error) {
				if request.Method == http.MethodGet && !readable && posts.Load() > 0 {
					return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader("private-provider-body")), Header: http.Header{}}, nil
				}
				response, err := transport.RoundTrip(request)
				if request.Method == http.MethodPost {
					posts.Add(1)
					route, routeErr := publisher.Store.(interface {
						OperatorRoute(context.Context, string) (string, error)
					}).OperatorRoute(ctx, s.ID)
					if routeErr != nil || route == "" {
						t.Error("publication happened before durable route pin", routeErr)
					}
					if response != nil {
						_ = response.Body.Close()
					}
					return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("private-provider-body")), Header: http.Header{}}, nil
				}
				return response, err
			})}
			plan, err := manager.PlanContext(ctx, s)
			if err != nil {
				t.Fatal(err)
			}
			result, err := manager.Deploy(ctx, s, plan.Digest)
			if readable {
				if err != nil || !strings.Contains(result.Message, "readback") {
					t.Fatal(result, err)
				}
			} else if !errors.Is(err, model.ErrUnavailable) {
				t.Fatal(err)
			}
			if posts.Load() != 1 || kube.writeCount() != 1 {
				t.Fatal("ambiguous mutation was repeated", posts.Load(), kube.writeCount())
			}
			data, _ := json.Marshal(result)
			if strings.Contains(string(data), "private-provider-body") {
				t.Fatal("provider body leaked")
			}
		})
	}
}

func TestPortalPublisherAcceptsSameNameMeasurementEdit(t *testing.T) {
	ctx := context.Background()
	p, manager, kube, s := publisherFixture(t)
	plan, err := manager.PlanContext(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Deploy(ctx, s, plan.Digest); err != nil {
		t.Fatal(err)
	}
	prior := kube.get(v1.SLOKind, s.ID)
	if err = p.Store.Update(ctx, s.ID, func(r *model.Record) error {
		r.SLO.Revision++
		r.SLO.GoodQuery = "sum(increase(corrected_requests_total[1m]))"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	record, _ := p.Store.Read(ctx, s.ID)
	s = record.SLO
	plan, err = manager.PlanContext(ctx, s)
	if err != nil || plan.Resource == nil || plan.PriorUID != prior.Metadata.UID {
		t.Fatal(plan, err)
	}
	if _, err = manager.Deploy(ctx, s, plan.Digest); err != nil {
		t.Fatal(err)
	}
	updated := kube.get(v1.SLOKind, s.ID)
	if updated.Metadata.UID != prior.Metadata.UID || updated.Metadata.Generation != prior.Metadata.Generation+1 {
		t.Fatal("SLO identity replaced", updated)
	}
	revision, err := RuleRevision(updated, p.Tenant)
	if err != nil || revision != 8 {
		t.Fatal(revision, err)
	}
	for _, rule := range plan.Group.Rules {
		if rule.Labels["nines_revision"] != "8" {
			t.Fatal("old external evidence selected", rule)
		}
	}
}
