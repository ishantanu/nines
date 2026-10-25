package identity_test

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/ishantanu/nines/internal/identity"
)

func directoryConnection(t *testing.T, endpoint string) *ldap.Conn {
	t.Helper()
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() != "127.0.0.1" {
		t.Fatal("directory fixture must use loopback")
	}
	ca, err := os.ReadFile(filepath.Join(os.Getenv("NINES_IDENTITY_FIXTURE_DIR"), "ca.pem"))
	if err != nil {
		t.Fatal("directory CA unavailable")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		t.Fatal("invalid directory fixture CA")
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool, ServerName: u.Hostname()}
	conn, err := ldap.DialURL(endpoint, ldap.DialWithDialer(&net.Dialer{Timeout: 3 * time.Second}), ldap.DialWithTLSConfig(cfg))
	if err != nil {
		t.Fatal("directory connection failed")
	}
	conn.SetTimeout(3 * time.Second)
	if u.Scheme == "ldap" {
		if err = conn.StartTLS(cfg); err != nil {
			_ = conn.Close()
			t.Fatal("directory StartTLS failed")
		}
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
func directorySubject(t *testing.T, conn *ldap.Conn, username string) string {
	t.Helper()
	result, err := conn.Search(ldap.NewSearchRequest("ou=people,dc=example,dc=test", ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2, 3, false, "(uid="+ldap.EscapeFilter(username)+")", []string{"entryUUID"}, nil))
	if err != nil || len(result.Entries) != 1 {
		t.Fatal("directory fixture user unavailable")
	}
	subject := result.Entries[0].GetAttributeValue("entryUUID")
	if subject == "" {
		t.Fatal("directory omitted stable UUID")
	}
	return subject
}
func (h *harness) directoryLogin(username, password string, want int) (*http.Client, identity.Actor, string) {
	h.t.Helper()
	c := browser()
	_, csrf := h.session(c)
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)
	h.call(c, "", "", csrf, "POST", "/auth/ldap/directory/login", body, want)
	actor, csrf := h.session(c)
	if want != 200 && actor.ID != "" {
		h.t.Fatal("failed directory authentication created a user session")
	}
	return c, actor, csrf
}

func TestRealIdentityOpenLDAPTLSAndFailureBoundaries(t *testing.T) {
	requireFixture(t)
	for _, mode := range []string{"ldaps", "starttls"} {
		t.Run(mode, func(t *testing.T) {
			endpoint := os.Getenv("NINES_IDENTITY_LDAP_URL")
			if mode == "starttls" {
				endpoint = os.Getenv("NINES_IDENTITY_STARTTLS_URL")
			}
			admin := directoryConnection(t, endpoint)
			if err := admin.Bind("cn=admin,dc=example,dc=test", fixtureSecret(t, "admin-password")); err != nil {
				t.Fatal("fixture admin bind failed")
			}
			aliceSubject := directorySubject(t, admin, "alice")
			bobSubject := directorySubject(t, admin, "bob")
			h := newHarness(t, func(public string) identity.Config {
				return parseConfig(t, fmt.Sprintf(`publicURL: %s
providers:
- id: directory
  name: OpenLDAP fixture
  type: ldap
  url: %s
  startTLS: %v
  caFile: %s
  bindDN: cn=reader,dc=example,dc=test
  bindPasswordEnv: ACCEPTANCE_READER
  userBaseDN: ou=people,dc=example,dc=test
  userFilter: (uid={username})
bootstrap:
- {tenant: alpha, provider: directory, subject: %s, role: owner}
- {tenant: beta, provider: directory, subject: %s, role: owner}
`, public, endpoint, mode == "starttls", filepath.Join(os.Getenv("NINES_IDENTITY_FIXTURE_DIR"), "ca.pem"), aliceSubject, bobSubject), func(string) string { return fixtureSecret(t, "reader-password") })
			})
			alice, actor, csrf := h.directoryLogin("alice", fixtureSecret(t, "user-password"), 200)
			if actor.IdentitySubject != aliceSubject {
				t.Fatal("directory identity was not pinned to entryUUID")
			}
			h.call(alice, "alpha", "", "", "GET", "/api/slos", "", 200)
			h.call(alice, "beta", "", "", "GET", "/api/slos", "", 403)
			h.directoryLogin("alice", "invalid-password", 401)
			h.directoryLogin("*)(uid=*)", fixtureSecret(t, "user-password"), 401)
			config := h.nativeLogin(alice, csrf, "alpha", "approve")
			h.nativeStatus(config, "alpha", 0, "owner")
			h.nativeStatus(config, "beta", 3, "")
			h.restart()
			h.nativeStatus(config, "alpha", 0, "owner")
			// A directory rename must preserve the immutable identity and direct grant.
			originalDN := "uid=alice,ou=people,dc=example,dc=test"
			renamedDN := "uid=alice-renamed,ou=people,dc=example,dc=test"
			if err := admin.ModifyDN(ldap.NewModifyDNRequest(originalDN, "uid=alice-renamed", true, "")); err != nil {
				t.Fatal("directory rename failed")
			}
			defer func() {
				if err := admin.ModifyDN(ldap.NewModifyDNRequest(renamedDN, "uid=alice", true, "")); err != nil {
					t.Error("directory rename cleanup failed")
				}
			}()
			renamed, after, renamedCSRF := h.directoryLogin("alice-renamed", fixtureSecret(t, "user-password"), 200)
			if after.ID != actor.ID || after.IdentitySubject != aliceSubject {
				t.Fatal("rename created another identity")
			}
			h.call(renamed, "alpha", "", "", "GET", "/api/slos", "", 200)
			h.directoryLogin("alice", fixtureSecret(t, "user-password"), 401)
			// Rotating the upstream reader credential fails new logins at the same
			// trusted route. It must not create a session or fall back to local grants.
			readerDN := "cn=reader,dc=example,dc=test"
			if _, err := admin.PasswordModify(ldap.NewPasswordModifyRequest(readerDN, "", opaque(t))); err != nil {
				t.Fatal("reader failure injection failed")
			}
			defer func() {
				if _, err := admin.PasswordModify(ldap.NewPasswordModifyRequest(readerDN, "", fixtureSecret(t, "reader-password"))); err != nil {
					t.Error("reader credential cleanup failed")
				}
			}()
			h.directoryLogin("alice-renamed", fixtureSecret(t, "user-password"), 401)
			// Existing authentication is deliberately not continuous directory polling.
			h.nativeStatus(config, "alpha", 0, "owner")
			h.call(renamed, "", "", renamedCSRF, "POST", "/auth/logout-all", "", 200)
			h.nativeStatus(config, "alpha", 3, "")
			h.call(alice, "alpha", "", "", "GET", "/api/slos", "", 401)
			if _, err := admin.PasswordModify(ldap.NewPasswordModifyRequest(readerDN, "", fixtureSecret(t, "reader-password"))); err != nil {
				t.Fatal("reader credential restore failed")
			}
			h.directoryLogin("alice-renamed", fixtureSecret(t, "user-password"), 200)
			// An untrusted CA is rejected for both encrypted transports, without issuing
			// authentication. The reader has been restored before this separate check.
			h.config.Providers[0].CAFile = ""
			h.restart()
			h.directoryLogin("alice-renamed", fixtureSecret(t, "user-password"), 401)
			t.Log("Verified real OpenLDAP TLS, reader bind failure, escaped search, stable rename, persistent native CLI access, explicit revocation and untrusted CA rejection")
		})
	}
}
