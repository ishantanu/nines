package datasources

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/tenancy"
)

// ProviderRoutes contains server-authorized account IDs and datasets for a tenant.
// Models and connection requests cannot extend this policy.
type ProviderRoutes map[string][]string

func managedKind(kind string) bool {
	switch kind {
	case "prometheus", "mimir", "datadog", "newrelic", "dash0", "victoriametrics":
		return true
	}
	return false
}
func collectionMode(mode string) string {
	if mode == "" {
		return "automatic"
	}
	return mode
}
func (r *Registry) AllowedProviderRoutes() ProviderRoutes {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := ProviderRoutes{}
	for kind, values := range r.routes {
		out[kind] = append([]string{}, values...)
		sort.Strings(out[kind])
		unique := out[kind][:0]
		for _, value := range out[kind] {
			if value != "" && (len(unique) == 0 || unique[len(unique)-1] != value) {
				unique = append(unique, value)
			}
		}
		out[kind] = unique
	}
	return out
}
func (r *Registry) validateProviderRoute(v record) error {
	if v.NativeEntityGUID != "" && (v.Kind != "newrelic" || !v.RulesEnabled) {
		return model.Invalid("nativeEntityGUID requires enabled New Relic native publication")
	}
	if v.Kind == "newrelic" && v.RulesEnabled {
		approved := false
		for _, guid := range r.routes["newrelic-entity"] {
			if guid != "" && guid == v.NativeEntityGUID {
				approved = true
			}
		}
		if !approved {
			return model.Invalid("New Relic native entity is not authorized for this tenant; ask the server administrator to allow its GUID")
		}
	}

	if v.Kind == "victoriametrics" {
		account, project := v.Options["accountID"], v.Options["projectID"]
		if account == "" && project == "" {
			return nil
		}
		route := account
		if project != "" {
			route += ":" + project
		}
		if tenancy.ValidVictoriaMetricsRoute(route) {
			for _, allowed := range r.routes["victoriametrics"] {
				if route == allowed {
					return nil
				}
			}
		}
		return model.Invalid("VictoriaMetrics cluster route is not authorized for this tenant; ask the server administrator to allow its account/project")
	}
	field := ""
	switch v.Kind {
	case "newrelic":
		field = "accountID"
	case "dash0":
		field = "dataset"
	default:
		return nil
	}
	for _, allowed := range r.routes[v.Kind] {
		if allowed != "" && v.Options[field] == allowed {
			return nil
		}
	}
	return model.Invalid("%s %s is not authorized for this tenant; ask the server administrator to allow it", v.Kind, field)
}
func (r *Registry) credentials(v record) (map[string]string, error) {
	out := map[string]string{}
	if len(v.Secret) == 0 {
		return out, nil
	}
	if r.key == nil || len(v.Secret) < r.key.NonceSize() {
		return nil, model.Invalid("managed source credentials require the configured encryption key")
	}
	b, e := r.key.Open(nil, v.Secret[:r.key.NonceSize()], v.Secret[r.key.NonceSize():], []byte(r.tenant+"/"+v.Name))
	if e != nil {
		return nil, errors.New("cannot decrypt managed source credentials")
	}
	switch v.CredentialFormat {
	case "":
		out["bearerToken"] = string(b) // Retain existing encrypted bearer records.
	case "map-v1":
		if json.Unmarshal(b, &out) != nil {
			return nil, errors.New("cannot decode managed source credentials")
		}
	default:
		return nil, errors.New("unknown managed source credential format")
	}
	return out, nil
}
func credentialAllowed(kind, key string) bool {
	switch kind {
	case "datadog":
		return key == "apiKey" || key == "applicationKey"
	case "newrelic":
		return key == "userKey"
	case "prometheus", "mimir", "dash0", "victoriametrics":
		return key == "bearerToken"
	}
	return false
}
func (r *Registry) updateCredentials(v *record, in Input) error {
	if len(in.Credentials) > 4 {
		return model.Invalid("too many provider credential fields")
	}
	if in.BearerToken != "" && len(in.Credentials) > 0 {
		return model.Invalid("use either bearerToken or credentials, not both")
	}
	credentials, err := r.credentials(*v)
	if err != nil {
		return err
	}
	if in.ClearCredentials {
		credentials = map[string]string{}
	}
	updates := in.Credentials
	if in.BearerToken != "" {
		updates = map[string]string{"bearerToken": in.BearerToken}
	}
	changed := in.ClearCredentials
	for name, value := range updates {
		if !credentialAllowed(v.Kind, name) {
			return model.Invalid("credential field is not supported by the selected provider")
		}
		if value == "" {
			continue
		} // Blank inputs preserve separately encrypted credentials.
		if len(value) > 8192 || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n\x00") {
			return model.Invalid("provider credentials must be bounded single-line values")
		}
		credentials[name] = value
		changed = true
	}
	if !changed {
		return nil
	}
	if len(credentials) == 0 {
		v.Secret = nil
		v.CredentialFormat = ""
		return nil
	}
	if r.key == nil {
		return model.Invalid("credential encryption key is required")
	}
	b, err := json.Marshal(credentials)
	if err != nil {
		return errors.New("cannot encode provider credentials")
	}
	nonce := make([]byte, r.key.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	v.Secret = r.key.Seal(nonce, nonce, b, []byte(r.tenant+"/"+v.Name))
	v.CredentialFormat = "map-v1"
	return nil
}

func copyOptions(options map[string]string) map[string]string {
	if len(options) == 0 {
		return nil
	}
	out := make(map[string]string, len(options))
	for key, value := range options {
		out[key] = value
	}
	return out
}
