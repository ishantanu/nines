package identity

import "encoding/json"

// sessionProviderBinding ties authentication to the current provider identity.
// It deliberately excludes secrets and display settings so routine credential
// rotation does not change who authenticated the user. Role mappings are checked
// separately on each request and are never frozen into this binding.
func (s *Service) sessionProviderBinding(u User) string {
	p, ok := s.providers[u.Provider]
	if !ok || u.Subject == "" || s.user(u.Provider, externalUser{Subject: u.Subject}).ID != u.ID {
		return ""
	}
	c := p.config
	values := []string{c.ID, c.Type}
	switch c.Type {
	case "oidc":
		values = append(values, c.IssuerURL, c.ClientID)
	case "ldap":
		values = append(values, c.URL, c.UserBaseDN, c.UserFilter, c.SubjectAttribute)
	case "local":
	default:
		return ""
	}
	raw, _ := json.Marshal(values)
	return hash(string(raw))
}
