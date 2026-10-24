package httpapi

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

//go:embed web
var webFiles embed.FS

func frontend() http.Handler {
	root, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	files := http.FileServerFS(root)
	var config struct {
		Endpoint   string `json:"endpoint"`
		ReplayHost string `json:"replayHost"`
	}
	if b, err := fs.ReadFile(root, "telemetry.json"); err == nil {
		_ = json.Unmarshal(b, &config)
	}
	connect := "connect-src 'self'"
	if origin := browserTelemetryOrigin(config.Endpoint); origin != "" {
		connect += " " + origin
	}
	if origin := browserReplayOrigin(config.ReplayHost); origin != "" && origin != browserTelemetryOrigin(config.Endpoint) {
		connect += " " + origin
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+connect+"; img-src 'self' data:; font-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		if r.URL.Path == "/replay-player.html" {
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; frame-src 'self'; form-action 'none'; object-src 'none'; base-uri 'none'; frame-ancestors 'self'")
		}
		// Only known portal routes get the application shell. Missing APIs and
		// assets remain 404 instead of receiving a successful HTML response.
		if portalPath(r.URL.Path) {
			w.Header().Set("Cache-Control", "no-store")
			page := r.Clone(r.Context())
			u := *r.URL
			u.Path, u.RawPath = "/", ""
			page.URL = &u
			files.ServeHTTP(w, page)
			return
		}
		files.ServeHTTP(w, r)
	})
}

// Only the trusted endpoint embedded by the frontend build can expand connect-src.
// Request parameters and tenant definitions cannot alter this policy.
func browserTelemetryOrigin(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(u.Host, " ;\t\r\n'\"") {
		return ""
	}
	ip := net.ParseIP(u.Hostname())
	local := u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())
	if u.Scheme != "https" && (u.Scheme != "http" || !local) {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func browserReplayOrigin(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Path != "" && u.Path != "/" {
		return ""
	}
	return browserTelemetryOrigin(endpoint)
}

var portalIdentifier = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// Keep the canonical route vocabulary aligned with web/src/lib/navigation.ts.
func portalPath(path string) bool {
	if path == "/" || path == "/login" || path == "/login/" {
		return true
	}
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(parts) < 4 || len(parts) > 5 || parts[0] != "" || parts[1] != "tenants" || !portalIdentifier.MatchString(parts[2]) {
		return false
	}
	detail := len(parts) == 5
	if detail && !portalIdentifier.MatchString(parts[4]) {
		return false
	}
	switch parts[3] {
	case "slos", "critical-user-journeys", "agent-evidence", "reviews", "investigations", "intents", "replays":
		return true
	case "home", "service-dependencies", "measurements", "alerts", "workflows", "data-sources", "access-control", "clients":
		return !detail
	default:
		return false
	}
}
