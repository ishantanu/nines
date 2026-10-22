package httpapi

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"net/url"
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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+connect+"; img-src 'self' data:; font-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		if r.URL.Path == "/replay-player.html" {
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; frame-src 'self'; form-action 'none'; object-src 'none'; base-uri 'none'; frame-ancestors 'self'")
		}
		// Only real bundled assets and the index are served; no SPA path fallback
		// can turn a misspelled API URL into a successful HTML response.
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
