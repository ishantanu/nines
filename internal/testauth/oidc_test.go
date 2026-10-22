package testauth

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestAuthorizationRedirectUsesRegisteredLoopbackCallback(t *testing.T) {
	idp, err := NewOIDC()
	if err != nil {
		t.Fatal(err)
	}
	defer idp.Close()
	const callback = "http://127.0.0.1:9999/auth/callback"
	if err := idp.AllowRedirect(callback); err != nil {
		t.Fatal(err)
	}
	for _, uri := range []string{"https://attacker.invalid/callback", "//attacker.invalid/callback", "http://user:password@127.0.0.1/callback", "http://127.0.0.1/callback?next=https://attacker.invalid", "http://127.0.0.1/callback#fragment"} {
		if err := idp.AllowRedirect(uri); err == nil {
			t.Fatalf("registered invalid callback %q", uri)
		}
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, uri := range []string{callback, "https://attacker.invalid/callback", callback + "/other", callback + "?next=https://attacker.invalid"} {
		q := url.Values{"client_id": {"fixture-client"}, "redirect_uri": {uri}, "code_challenge_method": {"S256"}, "code_challenge": {"challenge"}, "nonce": {"nonce"}, "state": {"state&next=https://attacker.invalid"}, "login_as": {"alice"}}
		resp, err := client.Get(idp.Server.URL + "/authorize?" + q.Encode())
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if uri != callback {
			if resp.StatusCode != 400 || resp.Header.Get("Location") != "" {
				t.Fatalf("unregistered redirect accepted: %s %d", uri, resp.StatusCode)
			}
			continue
		}
		location, err := url.Parse(resp.Header.Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 302 || location.Scheme != "http" || location.Host != "127.0.0.1:9999" || location.Path != "/auth/callback" || location.Query().Get("state") != q.Get("state") || !strings.HasPrefix(location.Query().Get("code"), "fixture-") {
			t.Fatalf("incorrect callback %v", location)
		}
	}
}
