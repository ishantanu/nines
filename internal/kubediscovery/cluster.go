package kubediscovery

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/time/rate"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
)

type Config struct {
	Namespaces []string `json:"namespaces" yaml:"namespaces"`
}

func (c Config) Validate() error {
	if len(c.Namespaces) < 1 || len(c.Namespaces) > 20 {
		return model.Invalid("Kubernetes discovery requires 1–20 explicit namespaces")
	}
	seen := map[string]bool{}
	for _, ns := range c.Namespaces {
		if !ValidNamespace(ns) || seen[ns] {
			return model.Invalid("Kubernetes discovery namespaces must be unique DNS labels; wildcards are unsupported")
		}
		seen[ns] = true
	}
	return nil
}

type Reader interface {
	Namespaces() []string
	Read(context.Context, string) (v1.KubernetesInventory, error)
}
type Cluster struct {
	Config          Config
	Base, TokenFile string
	HTTP            *http.Client
	Limiter         *rate.Limiter
}

func (c *Cluster) Namespaces() []string { return append([]string{}, c.Config.Namespaces...) }
func InCluster(cfg Config) (*Cluster, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT_HTTPS")
	if port == "" {
		port = "443"
	}
	if host == "" {
		return nil, model.Invalid("Kubernetes discovery needs in-cluster service-account configuration")
	}
	ca, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("kubernetes discovery service-account CA unavailable")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return nil, model.Invalid("invalid Kubernetes discovery CA")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	return &Cluster{Config: cfg, Limiter: rate.NewLimiter(5, 10), Base: "https://" + net.JoinHostPort(host, port), TokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token", HTTP: &http.Client{Transport: observability.Transport{Base: transport, PeerService: "kubernetes"}, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Cluster) Read(ctx context.Context, ns string) (v1.KubernetesInventory, error) {
	empty := v1.KubernetesInventory{}
	allowed := false
	for _, n := range c.Config.Namespaces {
		allowed = allowed || n == ns
	}
	if !allowed || !ValidNamespace(ns) {
		return empty, model.Invalid("namespace is not enabled for this tenant's Kubernetes discovery")
	}
	items := []json.RawMessage{}
	total := 0
	for _, prefix := range []string{"/api/v1/namespaces/" + ns + "/services", "/apis/apps/v1/namespaces/" + ns + "/deployments", "/apis/apps/v1/namespaces/" + ns + "/statefulsets", "/apis/apps/v1/namespaces/" + ns + "/daemonsets", "/apis/networking.k8s.io/v1/namespaces/" + ns + "/ingresses"} {
		next := ""
		for page := 0; page < 10; page++ {
			if c.Limiter != nil {
				if err := c.Limiter.Wait(ctx); err != nil {
					return empty, err
				}
			}
			query := url.Values{"limit": {"100"}}
			if next != "" {
				query.Set("continue", next)
			}
			req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(c.Base, "/")+prefix+"?"+query.Encode(), nil)
			if err != nil {
				return empty, model.ErrUnavailable
			}
			token, err := os.ReadFile(c.TokenFile)
			if err != nil {
				return empty, model.ErrUnavailable
			}
			credential := strings.TrimSpace(string(token))
			if credential == "" || len(credential) > 16384 || strings.ContainsAny(credential, "\r\n") {
				return empty, model.ErrUnavailable
			}
			req.Header.Set("Authorization", "Bearer "+credential)
			req.Header.Set("Accept", "application/json")
			req.Header.Set("User-Agent", "nines-dependency-discovery")
			client := *c.HTTP
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			response, err := client.Do(req)
			if err != nil {
				return empty, fmt.Errorf("kubernetes inventory request unavailable: %w", model.ErrUnavailable)
			}
			body, readErr := io.ReadAll(io.LimitReader(response.Body, MaxBytes+1))
			_ = response.Body.Close()
			if response.StatusCode != 200 || readErr != nil || len(body) > MaxBytes {
				return empty, fmt.Errorf("kubernetes inventory read failed; verify namespace RBAC and provider availability: %w", model.ErrUnavailable)
			}
			var result struct {
				Items    []json.RawMessage `json:"items"`
				Metadata struct {
					Continue string `json:"continue"`
				} `json:"metadata"`
			}
			if json.Unmarshal(body, &result) != nil {
				return empty, model.ErrUnavailable
			}
			// Some Kubernetes list responses omit each item's kind/apiVersion.
			kinds := map[string]string{"services": "Service", "deployments": "Deployment", "statefulsets": "StatefulSet", "daemonsets": "DaemonSet", "ingresses": "Ingress"}
			resource := prefix[strings.LastIndex(prefix, "/")+1:]
			for _, raw := range result.Items {
				var document map[string]json.RawMessage
				if json.Unmarshal(raw, &document) != nil || document == nil {
					return empty, model.ErrUnavailable
				}
				kind, _ := json.Marshal(kinds[resource])
				document["kind"] = kind
				encoded, _ := json.Marshal(document)
				total += len(encoded)
				items = append(items, encoded)
			}
			if len(items) > 1000 || total > MaxBytes {
				return empty, model.Invalid("Kubernetes discovery inventory exceeds bounds; use a narrower namespace or filtered local export")
			}
			next = result.Metadata.Continue
			if next == "" {
				break
			}
			if len(next) > 4096 || page == 9 {
				return empty, model.Invalid("Kubernetes discovery pagination exceeds bounds")
			}
		}
	}
	body, _ := json.Marshal(struct {
		Kind  string            `json:"kind"`
		Items []json.RawMessage `json:"items"`
	}{"List", items})
	inventory, err := Project(body, ns)
	if err != nil {
		return empty, err
	}
	for _, resource := range inventory.Resources {
		if resource.Namespace != ns {
			return empty, model.Invalid("Kubernetes inventory returned a resource outside the requested namespace")
		}
	}
	return inventory, nil
}
