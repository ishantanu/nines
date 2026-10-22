package operator

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sync/atomic"
	"time"

	k "github.com/ishantanu/nines/api/kubernetes/v1alpha1"
	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

type lease struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   k.Metadata `json:"metadata"`
	Spec       struct {
		Holder    string    `json:"holderIdentity"`
		RenewTime time.Time `json:"renewTime"`
		Duration  int64     `json:"leaseDurationSeconds"`
	} `json:"spec"`
}

func (a *API) renew(ctx context.Context, holder string) error {
	path := "/apis/coordination.k8s.io/v1/namespaces/" + url.PathEscape(a.Namespace) + "/leases/nines-operator"
	var current lease
	e := a.request(ctx, "GET", path, nil, &current)
	create := code(e, 404)
	if e != nil && !create {
		return e
	}
	now := time.Now().UTC()
	if !create && current.Spec.Holder != "" && current.Spec.Holder != holder && current.Spec.RenewTime.Add(time.Duration(current.Spec.Duration)*time.Second).After(now) {
		return fmt.Errorf("another operator holds the namespace lease")
	}
	current.APIVersion = "coordination.k8s.io/v1"
	current.Kind = "Lease"
	current.Metadata.Name = "nines-operator"
	current.Metadata.Namespace = a.Namespace
	current.Spec.Holder = holder
	current.Spec.RenewTime = now
	current.Spec.Duration = 30
	if create {
		return a.request(ctx, "POST", path[:len(path)-len("/nines-operator")], current, nil)
	}
	return a.request(ctx, "PUT", path, current, nil)
}

// Run elects one reconciler per namespace. A renewal failure cancels in-flight
// reconciliation. Mutations also check the last successful monotonic renewal;
// each resource has an eight-second deadline, below the remaining lease margin.
func (c *Controller) Run(ctx context.Context, log *slog.Logger) error {
	if e := c.Config.Validate(); e != nil {
		return e
	}
	holder := model.NewID()
	// Lease writes themselves must not depend on already owning the lease.
	leaseAPI := *c.API
	leaseAPI.BeforeWrite = nil
	for ctx.Err() == nil {
		leaseStart := time.Now()
		attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
		e := leaseAPI.renew(attempt, holder)
		cancel()
		if e != nil {
			if !delay(ctx, 5*time.Second) {
				break
			}
			continue
		}
		run, stop := context.WithCancel(ctx)
		var renewed atomic.Pointer[time.Time]
		renewed.Store(&leaseStart)
		c.API.BeforeWrite = func() error {
			last := renewed.Load()
			if run.Err() != nil || last == nil || time.Since(*last) > 20*time.Second {
				return fmt.Errorf("namespace lease is not current")
			}
			return nil
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-run.Done():
					return
				case <-ticker.C:
					renewalStart := time.Now()
					renewal, end := context.WithTimeout(run, 4*time.Second)
					e := leaseAPI.renew(renewal, holder)
					end()
					if e != nil {
						stop()
						return
					}
					renewed.Store(&renewalStart)
				}
			}
		}()
		log.InfoContext(run, "Operator holds namespace lease", "namespace", c.Config.Namespace, "tenant", c.Config.Tenant)
		for run.Err() == nil {
			for _, kind := range []string{v1.BindingKind, v1.IntentKind, v1.SLOKind, v1.JourneyKind} {
				listing, end := context.WithTimeout(run, 8*time.Second)
				items, e := c.API.List(listing, kind)
				end()
				if e != nil {
					log.WarnContext(run, "Resource listing failed", "kind", kind, "error", e)
					continue
				}
				for _, r := range items {
					if run.Err() != nil {
						break
					}
					if e = c.API.BeforeWrite(); e != nil {
						stop()
						break
					}
					resource, end := context.WithTimeout(run, 8*time.Second)
					e = c.Reconcile(resource, r)
					end()
					if e != nil && run.Err() == nil {
						log.WarnContext(run, "Resource reconciliation failed", "kind", kind, "resource", r.Metadata.Name, "error", e)
					}
				}
			}
			if !delay(run, 10*time.Second) {
				break
			}
		}
		stop()
		<-done
	}
	return ctx.Err()
}
func delay(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
