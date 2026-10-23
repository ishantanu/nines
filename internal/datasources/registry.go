// Package datasources manages tenant-scoped provider connections. Host and
// upstream organization boundaries come from server configuration, never input.
package datasources

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ishantanu/nines/internal/identity"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/prometheus"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

type Input struct {
	Name             string `json:"name"`
	Kind             string `json:"kind"`
	URL              string `json:"url"`
	MimirOrgID       string `json:"mimirOrgID"`
	BearerToken      string `json:"bearerToken"`
	ClearCredentials bool   `json:"clearCredentials"`
	Reload           bool   `json:"reload"`
	RulesEnabled     bool   `json:"rulesEnabled"`
	RulerURL         string `json:"rulerURL"`
	ExpectedVersion  int64  `json:"expectedVersion"`
}
type record struct {
	Reload       bool   `json:"reload,omitempty"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	URL          string `json:"url"`
	MimirOrgID   string `json:"mimirOrgID,omitempty"`
	RulesEnabled bool   `json:"rulesEnabled"`
	RulerURL     string `json:"rulerURL,omitempty"`
	Version      int64  `json:"version"`
	Secret       []byte `json:"encryptedSecret,omitempty"`
}
type entry struct {
	record
	client *prometheus.Client
	target *rules.Target
}
type Registry struct {
	mu                    sync.RWMutex
	repo                  identity.Repository
	tenant                string
	reserved, hosts, orgs map[string]bool
	key                   cipher.AEAD
	directory             string
	store                 store.Store
	entries               map[string]entry
}

func New(ctx context.Context, repo identity.Repository, tenant string, reserved, hosts, orgs map[string]bool, key, directory string, st store.Store) (*Registry, error) {
	r := &Registry{repo: repo, tenant: tenant, reserved: reserved, hosts: hosts, orgs: orgs, directory: directory, store: st, entries: map[string]entry{}}
	if key != "" {
		b, e := base64.StdEncoding.DecodeString(key)
		if e != nil || len(b) != 32 {
			return nil, model.Invalid("NINES_SOURCE_ENCRYPTION_KEY must be a base64 32-byte key")
		}
		block, e := aes.NewCipher(b)
		if e != nil {
			return nil, e
		}
		r.key, e = cipher.NewGCM(block)
		if e != nil {
			return nil, e
		}
	}
	err := repo.View(ctx, func(tx identity.Tx) error {
		rows, e := tx.List("data_source", tenant)
		if e != nil {
			return e
		}
		for _, row := range rows {
			var value record
			if e = decodeRecord(row, &value); e != nil {
				return e
			}
			if reserved[value.Name] {
				return model.Invalid("managed source conflicts with a configured source")
			}
			next, e := r.prepare(value)
			if e != nil {
				return e
			}
			r.entries[value.Name] = next
		}
		return nil
	})
	return r, err
}
func (r *Registry) Close()        { r.repo.Close() }
func (r *Registry) Enabled() bool { return len(r.hosts) > 0 }
func (r *Registry) Source(name string) (service.MetricSource, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[name]
	return service.MetricSource{Kind: e.Kind, Collector: e.client}, ok
}
func (r *Registry) Target(name string) (rules.Target, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[name]
	if !ok || e.target == nil {
		return rules.Target{}, false
	}
	return *e.target, true
}
func (r *Registry) public(e entry) map[string]any {
	auth := "none"
	if len(e.Secret) > 0 {
		auth = "bearer"
	}
	var destination *tenancy.RuleDestination
	if e.target != nil {
		destination = &tenancy.RuleDestination{Namespace: e.target.Namespace, Directory: e.target.Directory, RulerURL: e.target.RulerURL, Reload: e.target.Reload}
	}
	return map[string]any{"id": e.Name, "kind": e.Kind, "endpoint": e.URL, "mimirOrgID": e.MimirOrgID, "authentication": auth, "configuration": "managed", "version": e.Version, "rules": destination}
}
func (r *Registry) List() []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := []string{}
	for name := range r.entries {
		names = append(names, name)
	}
	sort.Strings(names)
	out := []map[string]any{}
	for _, name := range names {
		out = append(out, r.public(r.entries[name]))
	}
	return out
}
func (r *Registry) validURL(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Scheme == "http" || u.Scheme == "https") && r.hosts[strings.ToLower(u.Hostname())]
}
func (r *Registry) prepare(v record) (entry, error) {
	if !tenancy.ValidID(v.Name) || v.Name == "primary" || (v.Kind != "prometheus" && v.Kind != "mimir") || !r.validURL(v.URL) {
		return entry{}, model.Invalid("select prometheus or mimir and an endpoint hostname allowed by the server")
	}
	if v.Kind == "mimir" && (!tenancy.ValidMimirID(v.MimirOrgID) || !r.orgs[v.MimirOrgID]) {
		return entry{}, model.Invalid("Mimir organization is not authorized for this tenant")
	}
	if v.Kind == "prometheus" && (v.MimirOrgID != "" || v.RulerURL != "") {
		return entry{}, model.Invalid("Prometheus connections cannot set a Mimir organization or ruler URL")
	}
	token := ""
	if len(v.Secret) > 0 {
		if r.key == nil || len(v.Secret) < r.key.NonceSize() {
			return entry{}, model.Invalid("managed source credentials require the configured encryption key")
		}
		b, e := r.key.Open(nil, v.Secret[:r.key.NonceSize()], v.Secret[r.key.NonceSize():], []byte(r.tenant+"/"+v.Name))
		if e != nil {
			return entry{}, errors.New("cannot decrypt managed source credentials")
		}
		token = string(b)
	}
	var c *prometheus.Client
	var err error
	if v.Kind == "mimir" {
		c, err = prometheus.NewMimir(v.URL, v.MimirOrgID, token)
	} else {
		c, err = prometheus.New(v.URL, token)
	}
	if err != nil {
		return entry{}, err
	}
	result := entry{record: v, client: c}
	if v.Reload && (!v.RulesEnabled || v.Kind != "prometheus") {
		return entry{}, model.Invalid("automatic reload requires enabled Prometheus rule deployment")
	}
	if v.RulesEnabled {
		t := rules.Target{Connection: v.Name, Kind: v.Kind, QueryURL: v.URL, Namespace: r.tenant + "-" + v.Name, MimirOrgID: v.MimirOrgID, BearerToken: token}
		if v.Kind == "mimir" {
			t.RulerURL = v.RulerURL
			if t.RulerURL == "" {
				t.RulerURL = v.URL
			}
			if !r.validURL(t.RulerURL) {
				return entry{}, model.Invalid("ruler hostname is not allowed by the server")
			}
		} else {
			if r.directory == "" {
				return entry{}, model.Invalid("server has no managed Prometheus rule directory")
			}
			t.Directory = r.directory
			t.Reload = v.Reload
		}
		result.target = &t
	}
	return result, nil
}
func (r *Registry) referenced(ctx context.Context, name string) bool {
	slos, err := r.store.List(ctx)
	if err != nil {
		return true
	}
	for _, slo := range slos {
		if slo.Source == name {
			return true
		}
	}
	if st, ok := r.store.(store.TopologyStore); ok {
		after := ""
		for {
			rows, e := st.ListTopology(ctx, "MeasurementBinding", after, 100)
			if e != nil {
				return true
			}
			for _, row := range rows {
				var binding model.MeasurementBinding
				if decodeRecord(row.Manifest, &binding) != nil {
					return true
				}
				if binding.Spec.Source == name {
					return true
				}
			}
			if len(rows) < 100 {
				break
			}
			last := rows[len(rows)-1]
			after = last.Kind + "/" + last.ID
		}
	}
	return false
}

// Save is called under the definition lock, preventing a reference from being
// created concurrently with an endpoint identity change or deletion.
func (r *Registry) Save(ctx context.Context, in Input) (map[string]any, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.Enabled() {
		return nil, model.ErrUnavailable
	}
	if r.reserved[in.Name] {
		return nil, model.Invalid("this source is managed by server configuration")
	}
	old, exists := r.entries[in.Name]
	if in.ExpectedVersion < 0 || (!exists && in.ExpectedVersion != 0) || (exists && old.Version != in.ExpectedVersion) {
		return nil, model.ErrRevisionConflict
	}
	if !exists && len(r.entries)+len(r.reserved) >= 21 {
		return nil, model.Invalid("tenant connection limit reached")
	}
	v := record{Name: in.Name, Kind: in.Kind, URL: strings.TrimRight(in.URL, "/"), MimirOrgID: in.MimirOrgID, RulesEnabled: in.RulesEnabled, Reload: in.Reload, RulerURL: strings.TrimRight(in.RulerURL, "/"), Version: in.ExpectedVersion + 1, Secret: old.Secret}
	if exists && (old.URL != v.URL || old.Kind != v.Kind || old.MimirOrgID != v.MimirOrgID || old.RulesEnabled != v.RulesEnabled || old.RulerURL != v.RulerURL) && r.referenced(ctx, in.Name) {
		return nil, model.Invalid("source is referenced by an SLO or measurement binding; create a new connection and review definition changes")
	}
	if in.ClearCredentials {
		v.Secret = nil
	}
	if in.BearerToken != "" {
		if r.key == nil || len(in.BearerToken) > 8192 || strings.ContainsAny(in.BearerToken, "\r\n") {
			return nil, model.Invalid("credential encryption key is required, and tokens must be single-line")
		}
		nonce := make([]byte, r.key.NonceSize())
		if _, e := rand.Read(nonce); e != nil {
			return nil, e
		}
		v.Secret = r.key.Seal(nonce, nonce, []byte(in.BearerToken), []byte(r.tenant+"/"+in.Name))
	}
	next, err := r.prepare(v)
	if err != nil {
		return nil, err
	}
	err = r.repo.Update(ctx, "data-sources:"+r.tenant, func(tx identity.Tx) error {
		current, e := tx.Get("data_source", r.tenant, in.Name)
		if errors.Is(e, identity.ErrNotFound) {
			if in.ExpectedVersion != 0 {
				return model.ErrRevisionConflict
			}
		} else if e != nil {
			return e
		} else {
			var stored record
			if decodeRecord(current, &stored) != nil || stored.Version != in.ExpectedVersion {
				return model.ErrRevisionConflict
			}
		}
		return tx.Put("data_source", r.tenant, in.Name, encodeRecord(v), time.Time{})
	})
	if err != nil {
		return nil, err
	}
	r.entries[in.Name] = next
	return r.public(next), nil
}
func (r *Registry) Delete(ctx context.Context, name string, expected int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	old, ok := r.entries[name]
	if !ok {
		return model.ErrNotFound
	}
	if expected != old.Version {
		return model.ErrRevisionConflict
	}
	if r.referenced(ctx, name) {
		return model.Invalid("source is referenced by an SLO or measurement binding")
	}
	err := r.repo.Update(ctx, "data-sources:"+r.tenant, func(tx identity.Tx) error {
		raw, e := tx.Get("data_source", r.tenant, name)
		if e != nil {
			return e
		}
		var v record
		if decodeRecord(raw, &v) != nil || v.Version != expected {
			return model.ErrRevisionConflict
		}
		return tx.Delete("data_source", r.tenant, name)
	})
	if err == nil {
		delete(r.entries, name)
	}
	return err
}
