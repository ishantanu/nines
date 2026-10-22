package httpapi

import (
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
func TestReplayOriginPreservesStrictScriptPolicy(t *testing.T) {
	for _, tt := range []struct{ value, want string }{{"https://replay.example/", "https://replay.example"}, {"http://127.0.0.1:9900", "http://127.0.0.1:9900"}, {"http://replay.example", ""}, {"https://replay.example/path", ""}, {"https://user:secret@replay.example", ""}, {"https://replay.example?url=secret", ""}, {"https://replay.example;script-src *", ""}} {
		if got := browserReplayOrigin(tt.value); got != tt.want {
			t.Fatal(tt.value, got)
		}
	}
}
