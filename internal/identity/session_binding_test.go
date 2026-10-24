package identity

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBrowserAndCLISessionsRequireCurrentProviderBinding(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Service)
	}{
		{"removed", func(s *Service) { delete(s.providers, "microsoft") }},
		{"issuer", func(s *Service) {
			p := s.providers["microsoft"]
			p.config.IssuerURL = "https://other.invalid"
			s.providers["microsoft"] = p
		}},
		{"audience", func(s *Service) {
			p := s.providers["microsoft"]
			p.config.ClientID = "other-client"
			s.providers["microsoft"] = p
		}},
		{"type", func(s *Service) { p := s.providers["microsoft"]; p.config.Type = "local"; s.providers["microsoft"] = p }},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			repo := NewMemory()
			svc := mappingFixture(t, repo)
			svc.limits = map[string]limitEntry{}
			session, actor := mappedLogin(t, svc, "alice", evidence(nil, nil))
			if err := repo.Update(ctx, "tenant:alpha", func(tx Tx) error {
				return svc.saveMember(tx, "alpha", Member{UserID: actor.ID, Role: "owner", EffectiveRole: "owner", Version: 1})
			}); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewUnstartedServer(nil)
			svc.config.PublicURL = "http://" + server.Listener.Addr().String()
			server.Config.Handler = svc.Handler()
			server.Start()
			defer server.Close()
			authorization, verifier, _ := startCLI(t, server, "alpha")
			code := authorizeCLI(t, svc, server, session, authorization)
			result := request(t, server.Client(), "POST", server.URL+"/auth/cli/exchange", "", exchangeBody(code, verifier), 200)
			token := textField(t, result, "token")
			if a, err := svc.CLIActor(ctx, token); err != nil || a.Grants["alpha"] != "owner" {
				t.Fatal("initial CLI access", a, err)
			}
			// Rebuild the service over the same persisted repository, as on restart.
			after := mappingFixture(t, repo)
			test.change(after)
			if _, err := after.SessionActor(ctx, session); !errors.Is(err, ErrForbidden) {
				t.Fatal("old provider session remained valid", err)
			}
			if _, err := after.CLIActor(ctx, token); !errors.Is(err, ErrForbidden) {
				t.Fatal("CLI credential retained old provider trust", err)
			}
		})
	}
}

func TestSessionBindingSurvivesSecretRotationAndUsesCurrentRoles(t *testing.T) {
	ctx := context.Background()
	repo := NewMemory()
	svc := mappingFixture(t, repo)
	session, _ := mappedLogin(t, svc, "alice", evidence(nil, []string{"Reliability.Admin", "Reliability.Reader"}))
	after := mappingFixture(t, repo)
	p := after.providers["microsoft"]
	p.config.secret = "rotated-private-secret"
	p.config.Name = "Renamed display name"
	p.config.RoleMappings = []RoleMapping{{ID: "reader", Tenant: "alpha", Claim: "roles", Value: "Reliability.Reader", Role: "viewer"}}
	after.providers["microsoft"] = p
	actor, err := after.SessionActor(ctx, session)
	if err != nil || actor.Grants["alpha"] != "viewer" || len(actor.Grants) != 1 {
		t.Fatal("current policy or secret rotation failed", actor, err)
	}
	after.Now = func() time.Time { return time.Now().Add(16 * time.Minute) }
	if _, err = after.SessionActor(ctx, session); !errors.Is(err, ErrForbidden) {
		t.Fatal("expired credential survived", err)
	}
}

func TestLDAPSessionRejectsChangedDirectoryIdentity(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*ProviderConfig)
	}{
		{"directory", func(p *ProviderConfig) { p.URL = "ldaps://127.0.0.1:2" }},
		{"search-base", func(p *ProviderConfig) { p.UserBaseDN = "ou=other,dc=example" }},
		{"login-filter", func(p *ProviderConfig) { p.UserFilter = "(&(uid={username})(memberOf=cn=allowed,dc=example))" }},
		{"subject-attribute", func(p *ProviderConfig) { p.SubjectAttribute = "objectGUID" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			svc, _ := serviceFixture(t, NewMemory())
			token, _, err := svc.login(ctx, "directory", externalUser{Subject: "alice"})
			if err != nil {
				t.Fatal(err)
			}
			cfg := svc.config
			cfg.Providers = append([]ProviderConfig(nil), cfg.Providers...)
			test.change(&cfg.Providers[0])
			after, err := New(ctx, cfg, svc.repo, svc.tenants)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = after.SessionActor(ctx, token); !errors.Is(err, ErrForbidden) {
				t.Fatal("old directory authentication survived", err)
			}
		})
	}
}
