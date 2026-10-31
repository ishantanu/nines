package rules

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	k "github.com/ishantanu/nines/api/kubernetes/v1alpha1"
	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/dash0"
	"github.com/ishantanu/nines/internal/datadog"
	"github.com/ishantanu/nines/internal/googlecloud"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/newrelic"
	"github.com/ishantanu/nines/internal/observability"
)

type Target struct {
	NewRelicAccountID     string `json:"newRelicAccountID,omitempty"`
	NewRelicEntityGUID    string `json:"newRelicEntityGUID,omitempty"`
	NewRelicUserKey       string `json:"-"`
	DatadogAPIKey         string `json:"-"`
	DatadogApplicationKey string `json:"-"`
	GoogleProjectID       string `json:"googleProjectID,omitempty"`
	GoogleServiceID       string `json:"googleServiceID,omitempty"`
	GoogleAuth            string `json:"googleAuth,omitempty"`
	Dataset               string `json:"dataset,omitempty"`
	AutoDeploy            bool   `json:"autoDeploy,omitempty"`
	Publisher             string `json:"publisher,omitempty"`
	ProviderKind          string `json:"providerKind,omitempty"`
	Reload                bool   `json:"reload,omitempty"`
	Connection            string `json:"connection"`
	Kind                  string `json:"kind"`
	QueryURL              string `json:"queryURL"`
	RulerURL              string `json:"rulerURL,omitempty"`
	MimirOrgID            string `json:"mimirOrgID,omitempty"`
	Namespace             string `json:"namespace"`
	Directory             string `json:"directory,omitempty"`
	BearerToken           string `json:"-"`
	BasicUsername         string `json:"-"`
	BasicPassword         string `json:"-"`
}
type Plan struct {
	NativeNewRelic    *newrelic.Publication    `json:"nativeNewRelic,omitempty"`
	NativeDatadog     *datadog.Publication     `json:"nativeDatadog,omitempty"`
	NativeGoogleCloud *googlecloud.Publication `json:"nativeGoogleCloud,omitempty"`
	NativeSLO         *dash0.Publication       `json:"nativeSLO,omitempty"`
	Resource          *k.Resource              `json:"resource,omitempty"`
	ResourceYAML      string                   `json:"resourceYAML,omitempty"`
	PriorUID          string                   `json:"priorUID,omitempty"`
	PriorGeneration   int64                    `json:"priorGeneration,omitempty"`
	PriorSpecDigest   string                   `json:"priorSpecDigest,omitempty"`
	PriorRevision     string                   `json:"priorRevision,omitempty"`
	Target            Target                   `json:"target"`
	Group             Group                    `json:"group"`
	Digest            string                   `json:"digest"`
	YAML              string                   `json:"yaml"`
	SLORevision       int64                    `json:"sloRevision"`
}
type Deployment struct {
	NativeQueries *newrelic.NativeQueries `json:"nativeQueries,omitempty"`
	NativeID      string                  `json:"nativeID,omitempty"`
	NativeURL     string                  `json:"nativeURL,omitempty"`
	Activation    *v1.IntegrationStage    `json:"activation,omitempty"`
	Evaluation    *v1.IntegrationStage    `json:"evaluation,omitempty"`
	State         string                  `json:"state"`
	Digest        string                  `json:"digest"`
	Target        Target                  `json:"target"`
	Group         string                  `json:"group"`
	Message       string                  `json:"message"`
}

// ExternalDestination publishes desired resources rather than writing provider
// rules itself. Implementations must fence ownership and reviewed preconditions.
type ExternalDestination interface {
	Plan(context.Context, model.SLO, Target) (Plan, error)
	Apply(context.Context, model.SLO, Plan) (Deployment, error)
	Inspect(context.Context, model.SLO, Target, time.Time) (v1.IntegrationStage, v1.IntegrationStage)
	Identity(context.Context, model.SLO, Target) (model.RuleOutputIdentity, error)
}
type Manager struct {
	PublicationGuard func(context.Context, model.SLO, Target) error
	External         ExternalDestination

	DynamicTarget func(string) (Target, bool)
	BeforeWrite   func() error // Optional lease/authorization guard immediately before external writes.
	OwnerUID      string       // Optional Kubernetes resource ownership fence. Empty preserves API/CLI behavior.
	Tenant        string
	Targets       map[string]Target
	mu            sync.Mutex
}

func (m *Manager) Plan(s model.SLO) (Plan, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return m.PlanContext(ctx, s)
}
func (m *Manager) PlanContext(ctx context.Context, s model.SLO) (Plan, error) {
	if s.Retired() {
		return Plan{}, model.Invalid("SLO is archived; rule publication is disabled")
	}
	target, ok := m.target(ConnectionID(s))
	if !ok {
		return Plan{}, model.Invalid("connection %s has no recording-rule destination configured", ConnectionID(s))
	}
	if guard, ok := m.External.(interface {
		CheckRoute(context.Context, model.SLO, Target) error
	}); ok {
		if err := guard.CheckRoute(ctx, s, target); err != nil {
			return Plan{}, err
		}
	}
	if target.Kind == "operator" {
		if m.External == nil {
			return Plan{}, model.ErrUnavailable
		}
		return m.External.Plan(ctx, s, target)
	}
	if target.Kind == "newrelic" {
		return m.planNewRelic(ctx, s, target)
	}
	if target.Kind == "datadog" {
		return m.planDatadog(ctx, s, target)
	}
	if target.Kind == "googlecloud" {
		return m.planGoogle(ctx, s, target)
	}
	if target.Kind == "dash0" {
		return m.planNative(ctx, s, target)
	}
	group, err := Generate(s, m.Tenant)
	if m.OwnerUID != "" {
		group, err = GenerateOwned(s, m.Tenant, m.OwnerUID)
	}
	if err != nil {
		return Plan{}, err
	}
	b, err := yaml.Marshal(File{Groups: []Group{group}})
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{Target: target, Group: group, YAML: string(b), SLORevision: model.ConfigRevision(s)}
	plan.Digest = model.DefinitionDigest(struct {
		Tenant   string
		Manifest model.SLODefinition
		Revision int64
		Target   Target
		Group    Group
	}{m.Tenant, model.DefinitionFromSLO(s), model.ConfigRevision(s), target, group})
	return plan, nil
}
func (m *Manager) Apply(ctx context.Context, s model.SLO, expected string) (Deployment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Deployment{}, err
	}
	p, err := m.PlanContext(ctx, s)
	if err != nil {
		return Deployment{}, err
	}
	if expected == "" || expected != p.Digest {
		return Deployment{}, model.ErrRevisionConflict
	}
	if m.PublicationGuard != nil {
		if err := m.PublicationGuard(ctx, s, p.Target); err != nil {
			return Deployment{}, err
		}
	}
	if p.Target.Kind == "operator" {
		if err := m.authorizeWrite(ctx, s, expected); err != nil {
			return Deployment{}, err
		}
		return m.External.Apply(ctx, s, p)
	}
	if p.Target.Kind == "newrelic" {
		return m.applyNewRelic(ctx, s, p)
	}
	if p.Target.Kind == "datadog" {
		return m.applyDatadog(ctx, s, p)
	}
	if p.Target.Kind == "googlecloud" {
		return m.applyGoogle(ctx, s, p)
	}
	if p.Target.Kind == "dash0" {
		return m.applyNative(ctx, s, p)
	}
	result := Deployment{Digest: p.Digest, Target: p.Target, Group: p.Group.Name}
	if p.Target.Kind == "prometheus" {
		file := filepath.Join(p.Target.Directory, p.Target.Namespace+"-"+p.Group.Name+".yaml")
		existing, err := readRuleFile(file)
		if err == nil {
			if !Owned(existing.Groups[0], m.Tenant, p.Target.Connection, s.ID) || !ownedUID(existing.Groups[0], m.OwnerUID) {
				return result, model.Invalid("refusing to replace a rule file not owned by this SLO, tenant and resource owner")
			}
			if sameGroup(existing.Groups[0], p.Group) {
				result.State = "unchanged"
				result.Message = "Rule file already matches the reviewed definition. Runtime loading is checked separately."
				return result, nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return result, model.Invalid("cannot safely read the configured rule file; inspect its type, size, ownership and YAML before retrying")
		}
		if err := m.authorizeWrite(ctx, s, p.Digest); err != nil {
			return result, err
		}
		temp, err := os.CreateTemp(p.Target.Directory, ".nines-rules-*")
		if err != nil {
			return result, errors.New("configured rule directory must exist and be writable")
		}
		defer func() {
			_ = os.Remove(temp.Name())
		}()
		if err = temp.Chmod(0644); err == nil {
			_, err = temp.Write([]byte(p.YAML))
		}
		if err == nil {
			err = temp.Sync()
		}
		closeErr := temp.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			if guard := m.authorizeWrite(ctx, s, p.Digest); guard != nil {
				return result, guard
			}
			err = os.Rename(temp.Name(), file)
		}
		if err != nil {
			return result, errors.New("could not atomically write configured rule file")
		}
		result.State = "written"
		result.Message = "Rule file written to the configured Prometheus directory. This result proves persistence only."
		return result, nil
	}
	if p.Target.Kind != "mimir" {
		return result, model.Invalid("unsupported rule destination")
	}
	path := "/config/v1/rules/" + url.PathEscape(p.Target.Namespace)
	code, body, err := rulerRequest(ctx, p.Target, http.MethodGet, path+"/"+url.PathEscape(p.Group.Name), nil)
	if err != nil {
		return result, err
	}
	if code == http.StatusOK {
		existing, decodeErr := decodeRuleGroup(body)
		if decodeErr != nil || !Owned(existing, m.Tenant, p.Target.Connection, s.ID) || !ownedUID(existing, m.OwnerUID) {
			return result, model.Invalid("refusing to replace a rule group not owned by this SLO, tenant and resource owner")
		}
		if sameGroup(existing, p.Group) {
			result.State = "unchanged"
			result.Message = "Mimir rule group already matches the reviewed definition."
			return result, nil
		}
	} else if code != http.StatusNotFound {
		return result, fmt.Errorf("ruler inspection returned HTTP %d", code)
	}
	body, err = yaml.Marshal(p.Group)
	if err != nil {
		return result, err
	}
	if err := m.authorizeWrite(ctx, s, p.Digest); err != nil {
		return result, err
	}
	code, _, err = rulerRequest(ctx, p.Target, http.MethodPost, path, body)
	if err != nil || code >= 500 {
		// A timeout or gateway error can arrive after the ruler commits. Read
		// the exact reviewed destination once; never blindly repeat the POST.
		if ctx.Err() == nil {
			readCode, readBody, readErr := rulerRequest(ctx, p.Target, http.MethodGet, path+"/"+url.PathEscape(p.Group.Name), nil)
			stored, decodeErr := decodeRuleGroup(readBody)
			if readErr == nil && readCode == http.StatusOK && decodeErr == nil && sameGroup(stored, p.Group) {
				result.State = "verified"
				result.Message = "The write response was ambiguous, but an exact readback matches the reviewed rule group. Evaluation and notification delivery remain unverified."
				return result, nil
			}
		}
		return result, deploymentUnverifiedError{}
	}
	if code == http.StatusBadRequest || code == http.StatusRequestEntityTooLarge {
		return result, model.Invalid("ruler rejected a group containing %d rules (HTTP %d); review generated expressions and backend rule-group limits", len(p.Group.Rules), code)
	}
	if code != http.StatusAccepted {
		return result, fmt.Errorf("ruler deployment returned HTTP %d", code)
	}
	result.State = "accepted"
	result.Message = "Mimir accepted the rule group in the configured tenant and namespace. Ruler evaluation and sample ingestion must still be monitored."
	return result, nil
}

// authorizeWrite checks cancellation, the caller's current authority and the
// configured destination immediately before an externally visible mutation.
func (m *Manager) authorizeWrite(ctx context.Context, s model.SLO, digest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.BeforeWrite != nil {
		if err := m.BeforeWrite(); err != nil {
			return err
		}
	}
	latest, err := m.PlanContext(ctx, s)
	if err != nil {
		return err
	}
	if latest.Digest != digest {
		return model.ErrRevisionConflict
	}
	return ctx.Err()
}

func sameGroup(a, b Group) bool {
	normal := func(g Group) Group {
		if seconds, err := DurationSeconds(g.Interval); err == nil {
			g.Interval = fmt.Sprintf("%ds", seconds)
		}
		g.Rules = append([]Rule(nil), g.Rules...)
		for i := range g.Rules {
			if g.Rules[i].Alert != "" {
				if g.Rules[i].For == "" {
					g.Rules[i].For = "0s"
				} else if seconds, err := DurationSeconds(g.Rules[i].For); err == nil {
					g.Rules[i].For = fmt.Sprintf("%ds", seconds)
				}
			}
		}
		return g
	}
	return model.DefinitionDigest(normal(a)) == model.DefinitionDigest(normal(b))
}
func rulerRequest(ctx context.Context, target Target, method, path string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(target.RulerURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, errors.New("invalid configured ruler URL")
	}
	req.Header.Set("X-Scope-OrgID", target.MimirOrgID)
	if body != nil {
		req.Header.Set("Content-Type", "application/yaml")
	}
	if target.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+target.BearerToken)
	} else if target.BasicUsername != "" {
		req.SetBasicAuth(target.BasicUsername, target.BasicPassword)
	}
	client := http.Client{Transport: observability.Transport{PeerService: "mimir-ruler"}, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return 0, nil, errors.New("configured ruler request failed")
	}
	defer func() {
		_ = response.Body.Close()
	}()
	b, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(b) > 4<<20 {
		return 0, nil, errors.New("invalid or oversized ruler response")
	}
	return response.StatusCode, b, nil
}

func ownedUID(g Group, uid string) bool {
	if uid == "" {
		for _, r := range g.Rules {
			if r.Labels["nines_resource_uid"] != "" {
				return false
			}
		}
		return true
	}
	for _, r := range g.Rules {
		if r.Labels["nines_resource_uid"] != uid {
			return false
		}
	}
	return len(g.Rules) > 0
}

// RemoveOwned deletes only one exact operator-owned group in a configured target.
// An empty owner UID is never sufficient authority for operator cleanup.
func (m *Manager) RemoveOwned(ctx context.Context, source, group, slo string) error {
	if m.OwnerUID == "" {
		return model.Invalid("operator cleanup requires a resource UID")
	}
	target, ok := m.target(source)
	if !ok || target.Kind != "mimir" {
		return model.Invalid("configured Mimir target is required")
	}
	path := "/config/v1/rules/" + url.PathEscape(target.Namespace) + "/" + url.PathEscape(group)
	code, body, e := rulerRequest(ctx, target, http.MethodGet, path, nil)
	if e != nil {
		return e
	}
	if code == http.StatusNotFound {
		return nil
	}
	if code != http.StatusOK {
		return fmt.Errorf("ruler cleanup inspection returned HTTP %d", code)
	}
	existing, decodeErr := decodeRuleGroup(body)
	if decodeErr != nil || !Owned(existing, m.Tenant, source, slo) || !ownedUID(existing, m.OwnerUID) {
		return model.Invalid("refusing cleanup of a group owned by another resource")
	}
	if m.BeforeWrite != nil {
		if e := m.BeforeWrite(); e != nil {
			return e
		}
	}
	code, _, e = rulerRequest(ctx, target, http.MethodDelete, path, nil)
	if e != nil {
		return e
	}
	if code != http.StatusAccepted && code != http.StatusOK && code != http.StatusNoContent && code != http.StatusNotFound {
		return fmt.Errorf("ruler cleanup returned HTTP %d", code)
	}
	return nil
}

func (m *Manager) target(name string) (Target, bool) {
	if target, ok := m.Targets[name]; ok {
		return target, true
	}
	if m.DynamicTarget != nil {
		return m.DynamicTarget(name)
	}
	return Target{}, false
}

// A provider write can commit before its response is lost. Expose a fixed,
// actionable unavailable error without echoing upstream content or credentials.
type deploymentUnverifiedError struct{}

func (deploymentUnverifiedError) Error() string {
	return "ruler deployment outcome is unverified; inspect the destination before retrying the same reviewed digest"
}
func (deploymentUnverifiedError) Unwrap() error { return model.ErrUnavailable }

// OutputIdentity follows the resource UID selected by an external deployment.
func (m *Manager) OutputIdentity(ctx context.Context, s model.SLO) (model.RuleOutputIdentity, error) {
	target, ok := m.target(ConnectionID(s))
	if ok && target.Kind == "operator" {
		if m.External == nil {
			return model.RuleOutputIdentity{}, model.ErrUnavailable
		}
		return m.External.Identity(ctx, s, target)
	}
	if ok && (target.Kind == "dash0" || target.Kind == "googlecloud" || target.Kind == "datadog" || target.Kind == "newrelic") {
		return model.RuleOutputIdentity{}, model.Invalid("native SLO publication does not produce Nines recording-rule output")
	}
	if _, err := m.PlanContext(ctx, s); err != nil {
		return model.RuleOutputIdentity{}, err
	}
	return OutputIdentity(s, m.Tenant, m.OwnerUID), nil
}

// Withdraw is supported only by an external operator destination. It retains
// the saved SLO and requires the same exact reviewed plan as publication.
func (m *Manager) Withdraw(ctx context.Context, s model.SLO, expected string) (Deployment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	plan, err := m.PlanContext(ctx, s)
	if err != nil {
		return Deployment{}, err
	}
	if plan.Digest != expected || expected == "" {
		return Deployment{}, model.ErrRevisionConflict
	}
	if plan.Target.Kind == "newrelic" {
		return m.withdrawNewRelic(ctx, s, plan)
	}
	if plan.Target.Kind == "datadog" {
		return m.withdrawDatadog(ctx, s, plan)
	}
	if plan.Target.Kind == "googlecloud" {
		return m.withdrawGoogle(ctx, s, plan)
	}
	if plan.Target.Kind == "dash0" {
		return m.withdrawNative(ctx, s, plan)
	}
	external, ok := m.External.(interface {
		Withdraw(context.Context, model.SLO, Plan) (Deployment, error)
	})
	if !ok || plan.Target.Kind != "operator" {
		return Deployment{}, model.Invalid("withdraw is supported only for operator publications")
	}
	if err := m.authorizeWrite(ctx, s, expected); err != nil {
		return Deployment{}, err
	}
	return external.Withdraw(ctx, s, plan)
}

// TargetFor returns the trusted connection configuration, never a user URL.
func (m *Manager) TargetFor(s model.SLO) (Target, bool) { return m.target(ConnectionID(s)) }

// RouteDigest pins provider identity/ownership independently of automation policy.
func RouteDigest(target Target) string {
	target.AutoDeploy = false
	return model.DefinitionDigest(target)
}
