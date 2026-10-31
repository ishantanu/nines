package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoopbackAliasesStayOnConfiguredPortAndRequestOrigin(t *testing.T) {
	for _, public := range []string{"http://127.0.0.1:18080", "http://localhost:18080"} {
		s := &Service{config: Config{PublicURL: public, AllowLoopbackAliases: true}}
		for _, tc := range []struct {
			origin, host string
			want         bool
		}{
			{"http://localhost:18080", "localhost:18080", true},
			{"http://127.0.0.1:18080", "127.0.0.1:18080", true},
			{"http://localhost:18081", "localhost:18081", false},
			{"http://127.0.0.1:18081", "127.0.0.1:18081", false},
			{"https://localhost:18080", "localhost:18080", false},
			{"http://localhost.evil.test:18080", "localhost.evil.test:18080", false},
			{"http://127.0.0.2:18080", "127.0.0.2:18080", false},
			{"http://[::1]:18080", "[::1]:18080", false},
			{"http://localhost:18080/", "localhost:18080", false},
			{"http://localhost:18080?x=1", "localhost:18080", false},
			{"http://localhost:18080#fragment", "localhost:18080", false},
			{"http://user@localhost:18080", "localhost:18080", false},
			{"null", "localhost:18080", false},
		} {
			t.Run(public+"/"+tc.origin, func(t *testing.T) {
				req := httptest.NewRequest("POST", "http://"+tc.host+"/auth/logout", nil)
				req.Header.Set("Origin", tc.origin)
				if got := s.ValidOrigin(req); got != tc.want {
					t.Fatalf("origin accepted=%v, want %v", got, tc.want)
				}
			})
		}
		other := "http://localhost:18080"
		if strings.Contains(public, "localhost") {
			other = "http://127.0.0.1:18080"
		}
		req := httptest.NewRequest("POST", "http://evil.test/auth/logout", nil)
		req.Header.Set("Origin", other)
		req.Header.Set("X-Forwarded-Host", strings.TrimPrefix(other, "http://"))
		if s.ValidOrigin(req) {
			t.Fatal("forwarded host granted a loopback alias")
		}
		req = httptest.NewRequest("POST", other+"/auth/logout", nil)
		req.Header.Set("Origin", other)
		req.Header.Add("Origin", public)
		if s.ValidOrigin(req) {
			t.Fatal("duplicate Origin accepted")
		}
		s.config.AllowLoopbackAliases = false
		req.Header.Set("Origin", other)
		if s.ValidOrigin(req) {
			t.Fatal("aliases must be explicitly enabled")
		}
	}
	prod := &Service{config: Config{PublicURL: "https://nines.example.test", AllowLoopbackAliases: true}}
	req := httptest.NewRequest("POST", "http://localhost:18080/auth/logout", nil)
	req.Header.Set("Origin", "http://localhost:18080")
	if prod.ValidOrigin(req) {
		t.Fatal("production public URL granted loopback alias")
	}
}

func TestLoopbackAliasConfigurationRequiresLocalHTTP(t *testing.T) {
	for _, public := range []string{"https://nines.example.test", "https://localhost:18080", "http://127.0.0.2:18080", "http://[::1]:18080"} {
		_, err := ParseConfig([]byte("publicURL: "+public+"\nallowLoopbackAliases: true\n"), func(string) string { return "" })
		if err == nil || !strings.Contains(err.Error(), "allowLoopbackAliases") {
			t.Fatalf("%s: %v", public, err)
		}
	}
}

func TestLocalAliasLoginSessionAndLogoutRetainCSRF(t *testing.T) {
	for _, host := range []string{"localhost:18080", "127.0.0.1:18080"} {
		t.Run(host, func(t *testing.T) {
			cfg, err := demoAuthConfig("http://127.0.0.1:18080", "admin", true, "admin")
			if err != nil {
				t.Fatal(err)
			}
			cfg.AllowLoopbackAliases = true
			svc, err := New(context.Background(), cfg, NewMemory(), map[string]bool{"alpha": true})
			if err != nil {
				t.Fatal(err)
			}
			defer svc.Close()
			handler := svc.Handler()
			call := func(method, path, body, csrf string, cookie *http.Cookie, want int) *httptest.ResponseRecorder {
				t.Helper()
				req := httptest.NewRequest(method, "http://"+host+path, strings.NewReader(body))
				req.Header.Set("Origin", "http://"+host)
				req.Header.Set("Content-Type", "application/json")
				if csrf != "" {
					req.Header.Set("X-CSRF-Token", csrf)
				}
				if cookie != nil {
					req.AddCookie(cookie)
				}
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, req)
				if res.Code != want {
					t.Fatalf("%s: status %d, want %d", path, res.Code, want)
				}
				return res
			}
			info := call("GET", "/auth/session", "", "", nil, 200)
			var state struct {
				CSRF          string `json:"csrfToken"`
				Authenticated bool   `json:"authenticated"`
			}
			if err := json.Unmarshal(info.Body.Bytes(), &state); err != nil {
				t.Fatal(err)
			}
			loginCookie := info.Result().Cookies()[0]
			call("POST", "/auth/local/local/login", `{"username":"admin","password":"admin"}`, "", loginCookie, 403)
			signed := call("POST", "/auth/local/local/login", `{"username":"admin","password":"admin"}`, state.CSRF, loginCookie, 200)
			var session *http.Cookie
			for _, cookie := range signed.Result().Cookies() {
				if cookie.Domain != "" {
					t.Fatal("cookie must remain host-only")
				}
				if cookie.Name == svc.cookieName("session") {
					session = cookie
				}
			}
			if session == nil {
				t.Fatal("missing session cookie")
			}
			info = call("GET", "/auth/session", "", "", session, 200)
			if err := json.Unmarshal(info.Body.Bytes(), &state); err != nil {
				t.Fatal(err)
			}
			if !state.Authenticated {
				t.Fatal("session was not authenticated")
			}
			call("POST", "/auth/logout", `{}`, "", session, 403)
			call("POST", "/auth/logout", `{}`, state.CSRF, session, 200)
		})
	}
}
