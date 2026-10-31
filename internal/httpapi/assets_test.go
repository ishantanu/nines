package httpapi

import (
	"encoding/xml"
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrowserTelemetryCSPOrigin(t *testing.T) {
	for _, tt := range []struct{ endpoint, want string }{
		{"", ""}, {"http://127.0.0.1:54318", "http://127.0.0.1:54318"}, {"http://[::1]:4318/v1", "http://[::1]:4318"}, {"https://collector.example/otlp", "https://collector.example"},
		{"http://collector.example", ""}, {"https://user:secret@collector.example", ""}, {"https://collector.example?key=secret", ""}, {"https://collector.example#x", ""}, {"https://collector.example;evil", ""}, {"https://collector.example\nconnect-src *", ""}, {"javascript:alert(1)", ""},
	} {
		if got := browserTelemetryOrigin(tt.endpoint); got != tt.want {
			t.Errorf("%q => %q, want %q", tt.endpoint, got, tt.want)
		}
	}
	r := httptest.NewRecorder()
	frontend().ServeHTTP(r, httptest.NewRequest("GET", "/?otelEndpoint=https://evil.example", nil))
	csp := r.Header().Get("Content-Security-Policy")
	if strings.Contains(csp, "evil.example") || !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatal(csp)
	}
}
func TestPortalDeepLinksAndAssetMisses(t *testing.T) {
	handler := frontend()
	for _, path := range []string{"/", "/login", "/login/?returnTo=%2Ftenants%2Falpha%2Fslos", "/tenants/alpha/home", "/tenants/alpha/slos/api-latency", "/tenants/alpha/critical-user-journeys/checkout", "/tenants/alpha/reviews/incident-1", "/tenants/alpha/intents/chat-1", "/tenants/alpha/replays/session-1", "/tenants/alpha/data-sources/", "/tenants/alpha/telemetry-connections", "/tenants/alpha/telemetry-connections/", "/tenants/alpha/access-control", "/tenants/alpha/clients"} {
		for _, method := range []string{"GET", "HEAD"} {
			t.Run(method+path, func(t *testing.T) {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(method, path, nil))
				if response.Code != 200 || !strings.Contains(response.Header().Get("Content-Type"), "text/html") || response.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("%s %s: %d %v", method, path, response.Code, response.Header())
				}
				if method == "GET" && !strings.Contains(response.Body.String(), `id="root"`) {
					t.Fatal("missing app shell")
				}
				if method == "HEAD" && response.Body.Len() != 0 {
					t.Fatal("HEAD returned a body")
				}
			})
		}
	}
	for _, path := range []string{"/unknown", "/api/missing", "/auth/missing", "/assets/missing.js", "/tenants/alpha/missing", "/tenants/alpha/home/extra", "/tenants/alpha/slos/a/b", "/tenants/alpha/slos/%2e%2e", "/tenants/alpha/slos/a%2fb", "/tenants/alpha/slos//"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
			if response.Code != 404 {
				t.Fatalf("%s: %d", path, response.Code)
			}
		})
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("POST", "/login", nil))
	if response.Code != 405 {
		t.Fatalf("POST login: %d", response.Code)
	}
}

func TestProviderSVGAssetsServeAsImages(t *testing.T) {
	for _, provider := range []string{"prometheus", "mimir", "datadog", "newrelic", "victoriametrics", "dash0", "googlecloud"} {
		t.Run(provider, func(t *testing.T) {
			paths, err := fs.Glob(webFiles, "web/assets/"+provider+"-*.svg")
			if err != nil || len(paths) != 1 {
				t.Fatalf("expected one bundled %s SVG: %v", provider, err)
			}
			response := httptest.NewRecorder()
			frontend().ServeHTTP(response, httptest.NewRequest("GET", strings.TrimPrefix(paths[0], "web"), nil))
			if response.Code != 200 || !strings.HasPrefix(response.Header().Get("Content-Type"), "image/svg+xml") {
				t.Fatal("provider asset was not served as SVG", response.Code)
			}
			var svg struct {
				XMLName xml.Name `xml:"svg"`
				Paths   []struct {
					Data string `xml:"d,attr"`
				} `xml:"path"`
			}
			if err = xml.Unmarshal(response.Body.Bytes(), &svg); err != nil || svg.XMLName.Space != "http://www.w3.org/2000/svg" || len(svg.Paths) == 0 || svg.Paths[0].Data == "" {
				t.Fatal("provider SVG has no usable artwork", err)
			}
		})
	}
}
