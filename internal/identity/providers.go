package identity

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-ldap/ldap/v3"
	"golang.org/x/oauth2"
)

type provider struct {
	config   ProviderConfig
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
	tls      *tls.Config
}
type externalUser struct {
	Subject, Name, Email string
	localCredentialHash  string
	Authorization        *authorizationClaims
}

func initProvider(ctx context.Context, p ProviderConfig, publicURL string) (provider, error) {
	out := provider{config: p}
	if p.Type == "local" {
		return out, nil
	}
	if p.Type == "oidc" {
		issuer, err := oidc.NewProvider(oidc.ClientContext(ctx, providerHTTP()), p.IssuerURL)
		if err != nil {
			return out, errors.New("OIDC provider discovery failed")
		}
		ep := issuer.Endpoint()
		var metadata struct {
			JWKS string `json:"jwks_uri"`
		}
		if err = issuer.Claims(&metadata); err != nil {
			return out, errors.New("invalid OIDC metadata")
		}
		for _, s := range []string{ep.AuthURL, ep.TokenURL, metadata.JWKS} {
			u, e := url.Parse(s)
			if e != nil || !webEndpoint(u) {
				return out, errors.New("invalid OIDC endpoint")
			}
		}
		out.oauth = oauth2.Config{ClientID: p.ClientID, ClientSecret: p.secret, RedirectURL: publicURL + "/auth/oidc/" + p.ID + "/callback", Endpoint: ep, Scopes: []string{oidc.ScopeOpenID, "profile", "email"}}
		out.verifier = issuer.Verifier(&oidc.Config{ClientID: p.ClientID})
		return out, nil
	}
	u, _ := url.Parse(p.URL)
	out.tls = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname()}
	if p.CAFile != "" {
		b, err := os.ReadFile(p.CAFile)
		if err != nil {
			return out, errors.New("cannot read LDAP CA file")
		}
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(b) {
			return out, errors.New("LDAP CA file contains no certificates")
		}
		out.tls.RootCAs = pool
	}
	if _, err := ldap.CompileFilter(strings.Replace(p.UserFilter, "{username}", ldap.EscapeFilter("validation"), 1)); err != nil {
		return out, errors.New("invalid LDAP userFilter")
	}
	return out, nil
}
func providerHTTP() *http.Client {
	return &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func (p provider) authenticateLDAP(ctx context.Context, username, password string) (externalUser, error) {
	var user externalUser
	if username == "" || len(username) > 256 || password == "" || len(password) > 4096 || strings.ContainsRune(username, 0) {
		return user, ErrForbidden
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := ldap.DialURL(p.config.URL, ldap.DialWithDialer(dialer), ldap.DialWithTLSConfig(p.tls))
	if err != nil {
		return user, ErrForbidden
	}
	defer func() {
		_ = conn.Close()
	}()
	conn.SetTimeout(7 * time.Second)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	if p.config.StartTLS {
		if err = conn.StartTLS(p.tls); err != nil {
			return user, ErrForbidden
		}
	}
	if err = conn.Bind(p.config.BindDN, p.config.secret); err != nil {
		return user, ErrForbidden
	}
	filter := strings.Replace(p.config.UserFilter, "{username}", ldap.EscapeFilter(username), 1)
	result, err := conn.Search(ldap.NewSearchRequest(p.config.UserBaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2, 5, false, filter, []string{p.config.SubjectAttribute, p.config.NameAttribute, p.config.EmailAttribute}, nil))
	if err != nil || len(result.Entries) != 1 {
		return user, ErrForbidden
	}
	entry := result.Entries[0]
	if len(entry.GetRawAttributeValues(p.config.SubjectAttribute)) != 1 {
		return user, ErrForbidden
	}
	raw := entry.GetRawAttributeValue(p.config.SubjectAttribute)
	if len(raw) == 0 || len(raw) > 256 {
		return user, ErrForbidden
	}
	// AD's objectGUID is binary. LDAP entryUUID and textual immutable IDs retain
	// their administrator-visible representation for explicit membership grants.
	if strings.EqualFold(p.config.SubjectAttribute, "objectGUID") || strings.EqualFold(p.config.SubjectAttribute, "objectSid") {
		user.Subject = base64.RawURLEncoding.EncodeToString(raw)
	} else {
		user.Subject = string(raw)
	}
	if err = conn.Bind(entry.DN, password); err != nil {
		return externalUser{}, ErrForbidden
	}
	user.Name = entry.GetAttributeValue(p.config.NameAttribute)
	user.Email = entry.GetAttributeValue(p.config.EmailAttribute)
	if len(user.Name) > 300 {
		user.Name = user.Name[:300]
	}
	if len(user.Email) > 320 {
		user.Email = ""
	}
	return user, nil
}
