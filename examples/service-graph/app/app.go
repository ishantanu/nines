// Package app implements the instrumented HTTP chain used only by the optional
// service-dependency example. It is not a general proxy or a trace collector.
package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"
)

type App struct {
	name, peer, next string
	http             *http.Client
	good, bad, calls atomic.Uint64
}

func New(name, peer, next string) (*App, error) {
	valid := func(s string) bool {
		if len(s) < 1 || len(s) > 63 {
			return false
		}
		for _, r := range s {
			if r < 'a' || r > 'z' {
				if r < '0' || r > '9' {
					if r != '-' {
						return false
					}
				}
			}
		}
		return true
	}
	if !valid(name) || next != "" && !valid(peer) {
		return nil, fmt.Errorf("use simple service names")
	}
	if next != "" {
		u, e := url.Parse(next)
		if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "/request" {
			return nil, fmt.Errorf("configure a fixed HTTP request endpoint without credentials")
		}
	}
	return &App{name: name, peer: peer, next: next, http: &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	switch r.URL.Path {
	case "/healthz":
		w.WriteHeader(http.StatusOK)
	case "/metrics":
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		labels := `service=` + strconv.Quote(a.name) + `,environment="playground"`
		_, _ = fmt.Fprintf(w, "# HELP example_http_requests_total Actual requests handled by the local example.\n# TYPE example_http_requests_total counter\nexample_http_requests_total{%s,outcome=\"success\"} %d\nexample_http_requests_total{%s,outcome=\"failure\"} %d\n", labels, a.good.Load(), labels, a.bad.Load())
		if a.next != "" {
			_, _ = fmt.Fprintf(w, "# HELP traces_service_graph_request_total Outbound HTTP attempts observed by the example client wrapper; not a trace sampling guarantee.\n# TYPE traces_service_graph_request_total counter\ntraces_service_graph_request_total{client=%q,server=%q,connection_type=\"http\",environment=\"playground\"} %d\n", a.name, a.peer, a.calls.Load())
		}
	case "/request":
		if a.next != "" {
			a.calls.Add(1)
			request, e := http.NewRequestWithContext(r.Context(), http.MethodGet, a.next, nil)
			if e != nil {
				a.bad.Add(1)
				http.Error(w, "dependency unavailable", http.StatusBadGateway)
				return
			}
			response, e := a.http.Do(request)
			if e != nil {
				a.bad.Add(1)
				http.Error(w, "dependency unavailable", http.StatusBadGateway)
				return
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			_ = response.Body.Close()
			if response.StatusCode != http.StatusOK {
				a.bad.Add(1)
				http.Error(w, "dependency failed", http.StatusBadGateway)
				return
			}
		}
		a.good.Add(1)
		_, _ = io.WriteString(w, "ok\n")
	default:
		http.NotFound(w, r)
	}
}
func Generate(ctx context.Context, endpoint string, interval time.Duration) error {
	u, e := url.Parse(endpoint)
	if e != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" || u.Path != "/request" || interval < 100*time.Millisecond {
		return fmt.Errorf("configure a fixed request URL and interval of at least 100ms")
	}
	client := http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			request, e := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
			if e != nil {
				return e
			}
			response, e := client.Do(request)
			if e == nil {
				_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
				_ = response.Body.Close()
			}
		}
	}
}
