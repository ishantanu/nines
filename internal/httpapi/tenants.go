package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"errors"
	"reflect"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/datasources"
	"github.com/ishantanu/nines/internal/features"
	"github.com/ishantanu/nines/internal/identity"
	"github.com/ishantanu/nines/internal/intentflow"
	"github.com/ishantanu/nines/internal/investigation"
	"github.com/ishantanu/nines/internal/observability"
	"github.com/ishantanu/nines/internal/replay"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/slotemplates"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/workflow"
)

type Workspace struct {
	Templates           *slotemplates.Registry
	ManagedSources      *datasources.Registry
	IntentConversations *intentflow.Service
	Replays             *replay.Service
	Investigations      *investigation.Service
	Connections         []tenancy.Connection
	Rules               *rules.Manager
	ID                  string
	Name                string
	Service             *service.Service
	Source              tenancy.Source
	SourceAuth          string
	AccessConfig        []tenancy.Access
	Grants              []tenancy.Grant
	// Anonymous is only configured for the explicit loopback synthetic demo or
	// the legacy local single-workspace mode; it never expands a token grant.
	Anonymous bool
	Workflows *workflow.Service
	Identity  *identity.Service
	// GitOps is nil unless the tenant configures Git delivery.
	GitOps GitDelivery
	// Features turns gated experimental capabilities on; the zero value offers
	// only the stable core.
	Features features.Set
}
type tenantDescriptor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}
type mountedTenant struct {
	workspace Workspace
	handler   http.Handler
	// retired tenants stay readable; writes are refused.
	retired bool
}

// tenantSet is the router's tenants. Requests read an immutable snapshot;
// changes copy it and publish a new one, serialized by mu.
type tenantSet struct {
	mu                 sync.Mutex
	current            atomic.Pointer[map[string]mountedTenant]
	services           map[*service.Service]bool
	stores             map[store.Store]bool
	investigationRepos map[investigation.Repository]bool
	replayRepos        map[replay.Repository]bool
	intentRepos        map[intentflow.Repository]bool
	auth               *identity.Service
	log                *slog.Logger
}

func newTenantSet(auth *identity.Service, log *slog.Logger) *tenantSet {
	t := &tenantSet{services: map[*service.Service]bool{}, stores: map[store.Store]bool{}, investigationRepos: map[investigation.Repository]bool{}, replayRepos: map[replay.Repository]bool{}, intentRepos: map[intentflow.Repository]bool{}, auth: auth, log: log}
	empty := map[string]mountedTenant{}
	t.current.Store(&empty)
	return t
}

// snapshot returns a copy of the current tenants, for a change to publish.
func (t *tenantSet) snapshot() map[string]mountedTenant {
	next := make(map[string]mountedTenant, len(*t.current.Load()))
	for id, m := range *t.current.Load() {
		next[id] = m
	}
	return next
}

// mount validates a workspace and publishes it to the router. Workspaces
// never share a service, store or repository.
func (t *tenantSet) mount(workspace Workspace) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	auth, log := t.auth, t.log
	next := t.snapshot()
	if !tenancy.ValidID(workspace.ID) || workspace.Name == "" || workspace.Service == nil || next[workspace.ID].handler != nil || t.services[workspace.Service] {
		return fmt.Errorf("workspaces require unique IDs and independent service instances")
	}
	if workspace.Service.Store == nil || !reflect.TypeOf(workspace.Service.Store).Comparable() || t.stores[workspace.Service.Store] {
		return fmt.Errorf("workspaces require independent storage instances")
	}
	t.stores[workspace.Service.Store] = true
	if inv := workspace.Investigations; inv != nil {
		if inv.Platform != workspace.Service || inv.Repository == nil || !reflect.TypeOf(inv.Repository).Comparable() || t.investigationRepos[inv.Repository] {
			return fmt.Errorf("investigations require independent tenant-bound repositories")
		}
		t.investigationRepos[inv.Repository] = true
		inv.AuthorizeRun = func(ctx context.Context, actor v1.InvestigationActor) error {
			if actor.Kind == "api_token" {
				for _, g := range workspace.Grants {
					sum := sha256.Sum256([]byte("Bearer " + g.Token))
					if actor.Subject == "token:"+hex.EncodeToString(sum[:8]) && (g.Role == "owner" || g.Role == "editor" || g.Role == "agent") {
						return nil
					}
				}
				return investigation.ErrForbidden
			}
			if actor.Kind == "anonymous" && actor.Subject == "anonymous" && workspace.Anonymous && auth == nil {
				return nil
			}
			if auth == nil {
				return investigation.ErrForbidden
			}
			err := auth.AuthorizeBackground(ctx, workspace.ID, actor.Subject, actor.Kind)
			if errors.Is(err, identity.ErrForbidden) || errors.Is(err, identity.ErrNotFound) {
				return investigation.ErrForbidden
			}
			return err
		}
		inv.AuthorizeWatch = func(ctx context.Context, actor v1.InvestigationActor) error {
			if actor.Kind == "api_token" {
				for _, g := range workspace.Grants {
					sum := sha256.Sum256([]byte("Bearer " + g.Token))
					if actor.Subject == "token:"+hex.EncodeToString(sum[:8]) && (g.Role == "owner" || g.Role == "editor" || g.Role == "agent" || g.Role == "viewer") {
						return nil
					}
				}
				return investigation.ErrForbidden
			}
			if actor.Kind == "anonymous" && actor.Subject == "anonymous" && workspace.Anonymous && auth == nil {
				return nil
			}
			if auth == nil {
				return investigation.ErrForbidden
			}
			err := auth.AuthorizeBackgroundRead(ctx, workspace.ID, actor.Subject, actor.Kind)
			if errors.Is(err, identity.ErrForbidden) || errors.Is(err, identity.ErrNotFound) {
				return investigation.ErrForbidden
			}
			return err
		}
	}

	if flow := workspace.IntentConversations; flow != nil {
		flow.Templates = workspace.Templates
		if flow.Platform != workspace.Service || flow.Repository == nil || !reflect.TypeOf(flow.Repository).Comparable() || t.intentRepos[flow.Repository] {
			return fmt.Errorf("intent conversations require an independent tenant repository")
		}
		t.intentRepos[flow.Repository] = true
		flow.AuthorizeReview = func(ctx context.Context, actor v1.InvestigationActor, owner bool) error {
			if actor.Kind == "api_token" {
				for _, g := range workspace.Grants {
					sum := sha256.Sum256([]byte("Bearer " + g.Token))
					if actor.Subject == "token:"+hex.EncodeToString(sum[:8]) && (g.Role == "owner" || (!owner && g.Role == "editor")) {
						return nil
					}
				}
				return intentflow.ErrForbidden
			}
			if actor.Kind == "anonymous" && actor.Subject == "anonymous" && workspace.Anonymous && auth == nil {
				return nil
			}
			if auth == nil {
				return intentflow.ErrForbidden
			}
			return auth.AuthorizeBackgroundReview(ctx, workspace.ID, actor.Subject, actor.Kind, owner)
		}
		flow.Authorize = func(ctx context.Context, actor v1.InvestigationActor) error {
			if actor.Kind == "api_token" {
				for _, g := range workspace.Grants {
					sum := sha256.Sum256([]byte("Bearer " + g.Token))
					if actor.Subject == "token:"+hex.EncodeToString(sum[:8]) && (g.Role == "owner" || g.Role == "editor" || g.Role == "agent") {
						return nil
					}
				}
				return intentflow.ErrForbidden
			}
			if actor.Kind == "anonymous" && actor.Subject == "anonymous" && workspace.Anonymous && auth == nil {
				return nil
			}
			if auth == nil {
				return intentflow.ErrForbidden
			}
			return auth.AuthorizeBackground(ctx, workspace.ID, actor.Subject, actor.Kind)
		}
	}

	if rp := workspace.Replays; rp != nil {
		if rp.Platform != workspace.Service || rp.Repository == nil || !reflect.TypeOf(rp.Repository).Comparable() || t.replayRepos[rp.Repository] {
			return fmt.Errorf("session replays require an independent tenant repository")
		}
		t.replayRepos[rp.Repository] = true
	}

	if len(workspace.Grants) == 0 && !workspace.Anonymous && auth == nil {
		return fmt.Errorf("tenant %s has no access grants", workspace.ID)
	}
	for _, g := range workspace.Grants {
		if auth != nil && (strings.HasPrefix(g.Token, "wp_sa_") || strings.HasPrefix(g.Token, "wp_cli_")) {
			return fmt.Errorf("wp_sa_ and wp_cli_ are reserved for managed identity tokens")
		}
		if g.Token == "" || (g.Role != "owner" && g.Role != "editor" && g.Role != "agent" && g.Role != "viewer") {
			return fmt.Errorf("invalid tenant grant")
		}
	}
	workspace.Identity = auth
	if auth != nil {
		workspace.Anonymous = false
	}
	t.services[workspace.Service] = true
	workspace.Service.RuleManager = workspace.Rules
	if workspace.Rules != nil {
		workspace.Rules.PublicationGuard = workspace.Service.PinRuleDestination
	}
	workspace.Service.TenantID = workspace.ID
	workspace.Service.TenantName = workspace.Name
	if auth != nil && workspace.IntentConversations != nil && workspace.IntentConversations.SystemOne != nil {
		// Platform administrators control System One use and budget per tenant.
		workspace.IntentConversations.SystemOne = auth.ControlSystemOne(workspace.ID, workspace.IntentConversations.SystemOne)
	}
	next[workspace.ID] = mountedTenant{workspace: workspace, handler: newSingle(workspace.Service, log, workspace.Workflows, workspace)}
	t.current.Store(&next)
	return nil

}

// refreshPlatform gives identity the tenants the platform console lists,
// and the model adapters whose credentials must never appear in its input.
func (t *tenantSet) refreshPlatform() {
	if t.auth == nil {
		return
	}
	inventory := []identity.PlatformTenant{}
	guards := []func([]byte) bool{}
	for _, m := range *t.current.Load() {
		w := m.workspace
		if w.IntentConversations != nil {
			for _, adapter := range []any{w.IntentConversations.SystemOne, w.IntentConversations.Model, w.Service.IntentDrafter} {
				if guard, ok := adapter.(interface{ ContainsCredential([]byte) bool }); ok {
					guards = append(guards, guard.ContainsCredential)
				}
			}
		}
		var route *v1.SystemOneRoute
		if w.IntentConversations != nil && w.IntentConversations.SystemOne != nil {
			descriptor := w.IntentConversations.SystemOne.Descriptor()
			route = &descriptor
		}
		connectionNames := map[string]bool{"primary": true}
		for _, connection := range w.Connections {
			if connection.Name != "" {
				connectionNames[connection.Name] = true
			}
		}
		inventory = append(inventory, identity.PlatformTenant{ID: w.ID, Name: w.Name, SourceKind: w.Source.Kind, Connections: len(connectionNames), SystemOne: route})
	}
	sort.Slice(inventory, func(i, j int) bool { return inventory[i].ID < inventory[j].ID })
	t.auth.ConfigurePlatform(inventory, guards...)
}

// NewMulti is NewRouter for callers whose tenants never change.
func NewMulti(workspaces []Workspace, log *slog.Logger, identities ...*identity.Service) (http.Handler, error) {
	router, err := NewRouter(workspaces, log, identities...)
	if err != nil {
		return nil, err
	}
	return router, nil
}

// Router serves every mounted tenant. Mount and Retire change the tenants
// while it serves: requests already running keep the view they started with.
type Router struct {
	http.Handler
	tenants *tenantSet
}

// Mount adds a tenant created while the server runs.
func (r *Router) Mount(workspace Workspace) error {
	if err := r.tenants.mount(workspace); err != nil {
		return err
	}
	r.tenants.refreshPlatform()
	return nil
}

// Retire makes a tenant read-only. Its data and handler stay.
func (r *Router) Retire(id string) error {
	r.tenants.mu.Lock()
	next := r.tenants.snapshot()
	m, ok := next[id]
	if !ok {
		r.tenants.mu.Unlock()
		return fmt.Errorf("tenant %s is not mounted", id)
	}
	m.retired = true
	next[id] = m
	r.tenants.current.Store(&next)
	r.tenants.mu.Unlock()
	r.tenants.refreshPlatform()
	return nil
}

// Mounted reports whether a tenant is served, and whether it is retired.
func (r *Router) Mounted(id string) (mounted, retired bool) {
	m, ok := (*r.tenants.current.Load())[id]
	return ok, m.retired
}

// NewRouter selects a tenant-bound handler only after authenticating membership.
// Nothing below that boundary receives a caller-controlled store or source ID.
func NewRouter(workspaces []Workspace, log *slog.Logger, identities ...*identity.Service) (*Router, error) {
	var auth *identity.Service
	if len(identities) > 1 {
		return nil, fmt.Errorf("one identity service is supported")
	}
	if len(identities) == 1 {
		auth = identities[0]
	}
	var authHandler = identity.DisabledHandler()
	if auth != nil {
		authHandler = auth.Handler()
	}
	if len(workspaces) == 0 || len(workspaces) > 100 {
		return nil, fmt.Errorf("configure 1–100 workspaces")
	}
	tenants := newTenantSet(auth, log)
	for _, workspace := range workspaces {
		if err := tenants.mount(workspace); err != nil {
			return nil, err
		}
	}
	var platformHandler http.Handler
	if auth != nil {
		tenants.refreshPlatform()
		platformHandler = auth.PlatformHandler()
	}
	return &Router{tenants: tenants, Handler: observability.HTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// One consistent view of the tenants for the whole request.
		mounted := *tenants.current.Load()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		// Static assets are shared code, without tenant data. Health is independent
		// of tenant selection and never returns tenant IDs or provider details.
		if r.URL.Path == "/healthz" {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("Allow", "GET, HEAD")
				write(w, 405, map[string]string{"error": "method not allowed"})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			for _, t := range mounted {
				if err := t.workspace.Service.Store.Ping(ctx); err != nil {
					write(w, 503, map[string]string{"status": "unavailable"})
					return
				}
			}
			if auth != nil {
				if err := auth.Ping(ctx); err != nil {
					write(w, 503, map[string]string{"status": "unavailable"})
					return
				}
			}
			write(w, 200, map[string]string{"status": "ok"})
			return
		}
		if r.URL.Path == "/api/platform" || strings.HasPrefix(r.URL.Path, "/api/platform/") {
			if platformHandler == nil {
				write(w, 403, map[string]string{"error": "Platform administration requires configured identity"})
				return
			}
			platformHandler.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/auth/") {
			authHandler.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			frontend().ServeHTTP(w, r)
			return
		}
		if origin := r.Header.Get("Origin"); auth != nil && !auth.ValidOrigin(r) || auth == nil && origin != "" && origin != "http://"+r.Host && origin != "https://"+r.Host {
			write(w, 403, map[string]string{"error": "cross-origin requests are disabled"})
			return
		}
		grants := map[string]string{}
		if len(r.Header.Values("Authorization")) > 1 {
			write(w, 401, map[string]string{"error": "select exactly one authentication credential"})
			return
		}
		authorization := r.Header.Get("Authorization")
		rank := map[string]int{"viewer": 1, "agent": 2, "editor": 3, "owner": 4}
		provided := sha256.Sum256([]byte(authorization))
		for id, t := range mounted {
			if authorization == "" && t.workspace.Anonymous {
				grants[id] = "owner"
			}
			for _, g := range t.workspace.Grants {
				want := sha256.Sum256([]byte("Bearer " + g.Token))
				if subtle.ConstantTimeCompare(provided[:], want[:]) == 1 && rank[g.Role] > rank[grants[id]] {
					grants[id] = g.Role
				}
			}
		}
		actor := identity.Actor{ID: "token:" + hex.EncodeToString(provided[:8]), Kind: "api_token", Name: "Server API token", Grants: grants}
		if authorization == "" {
			actor.Kind = "anonymous"
			actor.ID = "anonymous"
			actor.Name = "Local demo"
		}
		identified := false
		if auth != nil {
			resolved, found, err := auth.Resolve(r)
			if err != nil {
				code := 503
				msg := "Identity service unavailable"
				if errors.Is(err, identity.ErrNotFound) || errors.Is(err, identity.ErrForbidden) {
					code = 401
					msg = "Sign in again to continue"
				}
				write(w, code, map[string]string{"error": msg})
				return
			}
			if found {
				actor = resolved
				grants = resolved.Grants
				identified = true
			}
		}
		if len(grants) == 0 && !identified {
			write(w, 401, map[string]string{"error": "valid tenant API token required"})
			return
		}
		if r.URL.Path == "/api/tenants" {
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", "GET")
				write(w, 405, map[string]string{"error": "method not allowed"})
				return
			}
			out := make([]tenantDescriptor, 0, len(grants))
			for id, role := range grants {
				// A grant can name a tenant this process has not mounted yet.
				if t, ok := mounted[id]; ok {
					out = append(out, tenantDescriptor{id, t.workspace.Name, role})
				}
			}
			sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
			write(w, 200, out)
			return
		}
		// Multiple header values are ambiguous selectors, even if they happen to
		// name the same tenant. Do not trust a proxy's concatenation semantics.
		if len(r.Header.Values(tenancy.Header)) > 1 {
			write(w, 400, map[string]string{"error": "select exactly one tenant"})
			return
		}
		id := r.Header.Get(tenancy.Header)
		if id == "" {
			if grants["default"] != "" {
				id = "default"
			} else if len(grants) == 1 {
				for candidate := range grants {
					id = candidate
				}
			} else {
				write(w, 400, map[string]string{"error": "X-Nines-Tenant is required for a token with multiple tenants"})
				return
			}
		}
		role := grants[id]
		if role == "" {
			write(w, 403, map[string]string{"error": "tenant access denied"})
			return
		}
		if _, ok := mounted[id]; !ok {
			// Granted but not mounted here yet, such as a tenant created moments
			// ago that this process has not loaded.
			w.Header().Set("Retry-After", "5")
			write(w, 503, map[string]string{"error": "This tenant is not available on this server yet. Try again shortly."})
			return
		}
		if mounted[id].retired && r.Method != http.MethodGet && r.Method != http.MethodHead {
			write(w, 409, map[string]string{"error": "This tenant is retired and read-only. A platform administrator can delete it.", "code": "tenant_retired"})
			return
		}
		if !allowed(role, r) {
			if governance := mounted[id].workspace.IntentConversations; governance != nil && r.Method == http.MethodPost {
				principal := tenancy.Principal{TenantID: id, Role: role, Subject: actor.ID, Kind: actor.Kind, Name: actor.Name}
				governance.RecordDenied(tenancy.WithPrincipal(r.Context(), principal), r.URL.Path)
			}
			write(w, 403, map[string]string{"error": "this operation is not permitted for your role; an owner can review workspace access"})
			return
		}
		if auth != nil && actor.Session && r.Method != http.MethodGet && r.Method != http.MethodHead && !auth.RequireCSRF(r, actor) {
			write(w, 403, map[string]string{"error": "CSRF validation failed"})
			return
		}
		if feature, gated := features.ForPath(r.URL.Path); gated && !mounted[id].workspace.Features.Enabled(feature) {
			write(w, 404, map[string]string{"error": features.Disabled(feature), "code": "feature_disabled", "feature": feature})
			return
		}
		observability.Tenant(r.Context(), id)
		principal := tenancy.Principal{TenantID: id, Role: role, Subject: actor.ID, Kind: actor.Kind, Name: actor.Name}
		w.Header().Set(tenancy.Header, id)
		request := r.WithContext(tenancy.WithPrincipal(r.Context(), principal))
		handler := mounted[id].handler
		if auth != nil && strings.HasPrefix(r.URL.Path, "/api/team-access") {
			handler = auth.TenantHandler()
		}
		if auth != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
			requestID := fmt.Sprintf("%d-%s", time.Now().UnixNano(), actor.ID)
			if err := auth.RecordAudit(r.Context(), id, actor, r.Method, r.URL.Path, "attempted", requestID, ""); err != nil {
				write(w, 503, map[string]string{"error": "Audit service unavailable; operation was not started"})
				return
			}
			recorder := &auditResponse{ResponseWriter: w, status: 200}
			handler.ServeHTTP(recorder, request)
			outcome := fmt.Sprintf("HTTP %d", recorder.status)
			auditCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			defer cancel()
			if err := auth.RecordAudit(auditCtx, id, actor, r.Method, r.URL.Path, outcome, requestID, w.Header().Get("ETag")); err != nil {
				log.ErrorContext(r.Context(), "audit outcome could not be persisted", "tenant", id, "requestId", requestID)
			}
			return
		}
		handler.ServeHTTP(w, request)
	}), log)}, nil
}
func allowed(role string, r *http.Request) bool {
	if role != "owner" && role != "editor" && role != "agent" && role != "viewer" {
		return false
	}
	if strings.HasPrefix(r.URL.Path, "/api/session-replays") && strings.HasSuffix(r.URL.Path, "/archive") {
		return role == "owner"
	}
	if strings.HasPrefix(r.URL.Path, "/api/team-access") {
		return role == "owner"
	}
	if role == "owner" || r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	if r.Method != http.MethodPost {
		return false
	}
	if strings.HasPrefix(r.URL.Path, "/api/slos/") && strings.HasSuffix(r.URL.Path, "/rules/plan") {
		return true
	}
	if strings.HasPrefix(r.URL.Path, "/api/bindings/") && strings.HasSuffix(r.URL.Path, "/compile") {
		return true
	}
	if strings.HasPrefix(r.URL.Path, "/api/slo-templates/") && strings.HasSuffix(r.URL.Path, "/render") {
		return true
	}
	if r.URL.Path == "/api/dependency-discovery/kubernetes" || r.URL.Path == "/api/slos/compile" || r.URL.Path == "/api/intents/candidates" || r.URL.Path == "/api/intents/evidence" || r.URL.Path == "/api/intents/backtest" || r.URL.Path == "/api/intents/plan" || r.URL.Path == "/api/journeys/plan" || r.URL.Path == "/api/resources/plan" || r.URL.Path == "/api/resources/validate" || r.URL.Path == "/api/migrations/convert" {
		return true
	}
	if r.URL.Path == "/api/session-replays" {
		return role == "editor"
	}
	if strings.HasPrefix(r.URL.Path, "/api/session-replays/") && strings.HasSuffix(r.URL.Path, "/links") {
		return role == "editor" || role == "agent"
	}
	if r.URL.Path == "/api/intent-conversations" || strings.HasPrefix(r.URL.Path, "/api/intent-conversations/") {
		if strings.HasSuffix(r.URL.Path, "/selection") || strings.HasSuffix(r.URL.Path, "/change-set/approve") || strings.HasSuffix(r.URL.Path, "/change-set/apply") || strings.HasSuffix(r.URL.Path, "/change-set/reject") || strings.Contains(r.URL.Path, "/change-set/rollback") {
			return role == "editor"
		}
		return role == "editor" || role == "agent"
	}

	if strings.Contains(r.URL.Path, "/visual-evaluations") {
		return role == "editor"
	}
	if r.URL.Path == "/api/intents/draft" {
		return role == "editor" || role == "agent"
	}
	if (role == "editor" || role == "agent") && (r.URL.Path == "/api/investigations" || strings.HasPrefix(r.URL.Path, "/api/investigations/") && (strings.HasSuffix(r.URL.Path, "/messages") || strings.HasSuffix(r.URL.Path, "/evidence") || strings.HasSuffix(r.URL.Path, "/runs") || strings.HasSuffix(r.URL.Path, "/cancel"))) {
		return true
	}
	if role == "editor" {
		// Declaring an incident and moving it through the lifecycle are
		// responders' decisions, not an agent's.
		if r.URL.Path == "/api/incidents" || r.URL.Path == "/api/incidents/alerts" || strings.HasPrefix(r.URL.Path, "/api/incidents/") && strings.HasSuffix(r.URL.Path, "/transition") {
			return true
		}
		// People write postmortems; agents can help in review messages.
		if strings.HasPrefix(r.URL.Path, "/api/investigations/") && strings.HasSuffix(r.URL.Path, "/postmortem") {
			return true
		}
		if r.URL.Path == "/api/resources/apply" || r.URL.Path == "/api/slos" || r.URL.Path == "/api/journeys/apply" || r.URL.Path == "/api/gitops/proposals" {
			return true
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) == 4 && parts[0] == "api" && parts[1] == "slos" && (parts[3] == "samples" || parts[3] == "collect") {
			return true
		}
	}
	if role != "agent" && role != "editor" {
		return false
	}
	if r.URL.Path == "/api/workflows" {
		return true
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	return len(parts) == 4 && parts[0] == "api" && parts[1] == "journeys" && parts[2] != "" && parts[3] == "runs"
}

type auditResponse struct {
	http.ResponseWriter
	status  int
	written bool
}

func (w *auditResponse) WriteHeader(code int) {
	if w.written {
		return
	}
	w.status = code
	w.written = true
	w.ResponseWriter.WriteHeader(code)
}
func (w *auditResponse) Write(b []byte) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
