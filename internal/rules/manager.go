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

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
)

type Target struct {
	Connection    string `json:"connection"`
	Kind          string `json:"kind"`
	QueryURL      string `json:"queryURL"`
	RulerURL      string `json:"rulerURL,omitempty"`
	MimirOrgID    string `json:"mimirOrgID,omitempty"`
	Namespace     string `json:"namespace"`
	Directory     string `json:"directory,omitempty"`
	BearerToken   string `json:"-"`
	BasicUsername string `json:"-"`
	BasicPassword string `json:"-"`
}
type Plan struct {
	Target      Target `json:"target"`
	Group       Group  `json:"group"`
	Digest      string `json:"digest"`
	YAML        string `json:"yaml"`
	SLORevision int64  `json:"sloRevision"`
}
type Deployment struct {
	State   string `json:"state"`
	Digest  string `json:"digest"`
	Target  Target `json:"target"`
	Group   string `json:"group"`
	Message string `json:"message"`
}
type Manager struct {
	DynamicTarget func(string) (Target, bool)
	BeforeWrite   func() error // Optional lease/authorization guard immediately before external writes.
	OwnerUID      string       // Optional Kubernetes resource ownership fence. Empty preserves API/CLI behavior.
	Tenant        string
	Targets       map[string]Target
	mu            sync.Mutex
}

func (m *Manager) Plan(s model.SLO) (Plan, error) {
	target, ok := m.target(ConnectionID(s))
	if !ok {
		return Plan{}, model.Invalid("connection %s has no recording-rule destination configured", ConnectionID(s))
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
	p, err := m.Plan(s)
	if err != nil {
		return Deployment{}, err
	}
	if expected == "" || expected != p.Digest {
		return Deployment{}, model.ErrRevisionConflict
	}
	result := Deployment{Digest: p.Digest, Target: p.Target, Group: p.Group.Name}
	if p.Target.Kind == "prometheus" {
		file := filepath.Join(p.Target.Directory, p.Target.Namespace+"-"+p.Group.Name+".yaml")
		if stat, err := os.Lstat(file); err == nil {
			if !stat.Mode().IsRegular() {
				return result, model.Invalid("rule destination is not a regular file")
			}
			old, err := os.ReadFile(file)
			if err != nil {
				return result, errors.New("cannot read configured rule file")
			}
			var existing File
			if yaml.Unmarshal(old, &existing) != nil || len(existing.Groups) != 1 || !Owned(existing.Groups[0], m.Tenant, p.Target.Connection, s.ID) {
				return result, model.Invalid("refusing to replace a rule file not owned by this SLO and tenant")
			}
			if sameGroup(existing.Groups[0], p.Group) {
				result.State = "unchanged"
				result.Message = "Rule file already matches. Prometheus must load this file through rule_files."
				return result, nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return result, errors.New("cannot inspect configured rule file")
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
			err = os.Rename(temp.Name(), file)
		}
		if err != nil {
			return result, errors.New("could not atomically write configured rule file")
		}
		result.State = "written"
		result.Message = "Rule file written to the configured Prometheus directory. Load it through rule_files and reload Prometheus; activation is not yet verified."
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
		var existing Group
		if yaml.Unmarshal(body, &existing) != nil || !Owned(existing, m.Tenant, p.Target.Connection, s.ID) || !ownedUID(existing, m.OwnerUID) {
			return result, model.Invalid("refusing to replace a rule group not owned by this SLO and tenant")
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
	if m.BeforeWrite != nil {
		if e := m.BeforeWrite(); e != nil {
			return result, e
		}
	}
	code, _, err = rulerRequest(ctx, p.Target, http.MethodPost, path, body)
	if err != nil {
		return result, err
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
	var existing Group
	if yaml.Unmarshal(body, &existing) != nil || !Owned(existing, m.Tenant, source, slo) || !ownedUID(existing, m.OwnerUID) {
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
