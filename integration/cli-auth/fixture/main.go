// Synthetic loopback SSO fixture for browser and native CLI verification.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"

	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/identity"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/testauth"
)

func main() {
	idp, e := testauth.NewOIDC()
	if e != nil {
		log.Fatal(e)
	}
	defer idp.Close()
	ln, e := net.Listen("tcp4", "127.0.0.1:8208")
	if e != nil {
		log.Fatal(e)
	}
	origin := "http://" + ln.Addr().String()
	idp.AllowRedirect(origin + "/auth/oidc/company/callback")
	cfg, e := identity.ParseConfig([]byte(fmt.Sprintf(`publicURL: %s
providers:
- id: company
  name: Synthetic company SSO
  type: oidc
  issuerURL: %s
  clientID: fixture-client
  clientSecretEnv: FIXTURE_SECRET
bootstrap:
- {tenant: alpha, provider: company, subject: alice, role: owner}
- {tenant: beta, provider: company, subject: bob, role: owner}
`, origin, idp.Server.URL)), func(string) string { return "fixture-secret" })
	if e != nil {
		log.Fatal(e)
	}
	auth, e := identity.New(context.Background(), cfg, identity.NewMemory(), map[string]bool{"alpha": true, "beta": true})
	if e != nil {
		log.Fatal(e)
	}
	handler, e := httpapi.NewMulti([]httpapi.Workspace{{ID: "alpha", Name: "Alpha fixture", Service: service.New(store.NewMemory(), nil)}, {ID: "beta", Name: "Beta fixture", Service: service.New(store.NewMemory(), nil)}}, slog.New(slog.NewTextHandler(io.Discard, nil)), auth)
	if e != nil {
		log.Fatal(e)
	}
	fmt.Println("Synthetic Nines CLI SSO fixture: " + origin)
	log.Fatal(http.Serve(ln, handler))
}
