package operator

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/time/rate"

	k "github.com/ishantanu/nines/api/kubernetes/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
)

var ErrOwnership = errors.New("resource belongs to another controller")

type apiError struct{ Code int }

func (e apiError) Error() string { return fmt.Sprintf("Kubernetes returned HTTP %d", e.Code) }
func code(err error, n int) bool { var e apiError; return errors.As(err, &e) && e.Code == n }

type API struct {
	Base, Namespace, TokenFile string
	HTTP                       *http.Client
	BeforeWrite                func() error
	Limiter                    *rate.Limiter
}

func InCluster(namespace string) (*API, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT_HTTPS")
	if port == "" {
		port = "443"
	}
	if host == "" {
		return nil, fmt.Errorf("in-cluster Kubernetes API configuration is required")
	}
	ca, e := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	if e != nil {
		return nil, e
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("invalid service-account CA")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	return &API{Base: "https://" + net.JoinHostPort(host, port), Namespace: namespace, TokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token", Limiter: rate.NewLimiter(5, 10), HTTP: &http.Client{Transport: observability.Transport{Base: transport, PeerService: "kubernetes"}, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (a *API) request(ctx context.Context, method, path string, input, output any) error {
	if a.Limiter != nil {
		if e := a.Limiter.Wait(ctx); e != nil {
			return e
		}
	}
	if method != "GET" && a.BeforeWrite != nil {
		if e := a.BeforeWrite(); e != nil {
			return e
		}
	}
	var body []byte
	var e error
	if input != nil {
		body, e = json.Marshal(input)
		if e != nil {
			return e
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, strings.TrimRight(a.Base, "/")+path, bytes.NewReader(body))
	if e != nil {
		return fmt.Errorf("invalid configured Kubernetes API")
	}
	if a.TokenFile != "" {
		token, e := secret(a.TokenFile)
		if e != nil {
			return e
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "nines-operator/0.1")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := *a.HTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, e := client.Do(req)
	if e != nil {
		return fmt.Errorf("kubernetes API request failed")
	}
	defer func() {
		_ = res.Body.Close()
	}()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return apiError{res.StatusCode}
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, (8<<20)+1))
	if e != nil || len(b) > 8<<20 {
		return fmt.Errorf("kubernetes response exceeds bounds")
	}
	if output != nil && json.Unmarshal(b, output) != nil {
		return fmt.Errorf("invalid Kubernetes response")
	}
	return nil
}
func (a *API) path(kind string) string {
	if kind == "PrometheusRule" {
		return "/apis/monitoring.coreos.com/v1/namespaces/" + url.PathEscape(a.Namespace) + "/prometheusrules"
	}
	return "/apis/nines.dev/v1alpha1/namespaces/" + url.PathEscape(a.Namespace) + "/" + Plurals[kind]
}
func (a *API) Get(ctx context.Context, kind, name string) (k.Resource, error) {
	var r k.Resource
	if Plurals[kind] == "" && kind != "PrometheusRule" {
		return r, model.Invalid("unsupported kind")
	}
	e := a.request(ctx, "GET", a.path(kind)+"/"+url.PathEscape(name), nil, &r)
	return r, e
}
func (a *API) List(ctx context.Context, kind string) ([]k.Resource, error) {
	if Plurals[kind] == "" {
		return nil, model.Invalid("unsupported resource kind")
	}
	out := []k.Resource{}
	next := ""
	for page := 0; page < 10; page++ {
		var response struct {
			Items    []k.Resource `json:"items"`
			Metadata struct {
				Continue string `json:"continue"`
			} `json:"metadata"`
		}
		path := a.path(kind) + "?limit=100"
		if next != "" {
			path += "&continue=" + url.QueryEscape(next)
		}
		if e := a.request(ctx, "GET", path, nil, &response); e != nil {
			return nil, e
		}
		out = append(out, response.Items...)
		if len(out) > 1000 {
			return nil, fmt.Errorf("namespace resource limit exceeded")
		}
		next = response.Metadata.Continue
		if next == "" {
			return out, nil
		}
	}
	return nil, fmt.Errorf("namespace resource listing is incomplete")
}
func owned(r k.Resource, uid string) bool {
	for _, o := range r.Metadata.OwnerReferences {
		if o.Controller && o.UID == uid && uid != "" {
			return true
		}
	}
	return false
}
func (a *API) ApplyOwned(ctx context.Context, desired k.Resource, ownerUID string) error {
	if desired.Metadata.Namespace != a.Namespace || !owned(desired, ownerUID) {
		return ErrOwnership
	}
	current, e := a.Get(ctx, desired.Kind, desired.Metadata.Name)
	if code(e, 404) {
		return a.request(ctx, "POST", a.path(desired.Kind), desired, nil)
	}
	if e != nil {
		return e
	}
	if !owned(current, ownerUID) {
		return ErrOwnership
	}
	references := append([]k.OwnerReference{}, current.Metadata.OwnerReferences...)
	for _, desiredRef := range desired.Metadata.OwnerReferences {
		found := false
		for i, old := range references {
			if old.UID == desiredRef.UID {
				if desiredRef.BlockOwnerDeletion == nil {
					desiredRef.BlockOwnerDeletion = old.BlockOwnerDeletion
				}
				references[i] = desiredRef
				found = true
				break
			}
		}
		if !found {
			references = append(references, desiredRef)
		}
	}
	desired.Metadata.OwnerReferences = references

	if CanonicalDigest(current.Spec) == CanonicalDigest(desired.Spec) && model.DefinitionDigest(current.Metadata.OwnerReferences) == model.DefinitionDigest(desired.Metadata.OwnerReferences) {
		matches := true
		for key, v := range desired.Metadata.Labels {
			matches = matches && current.Metadata.Labels[key] == v
		}
		for key, v := range desired.Metadata.Annotations {
			matches = matches && current.Metadata.Annotations[key] == v
		}
		if matches {
			return nil
		}
	}
	metadata := current.Metadata
	metadata.OwnerReferences = desired.Metadata.OwnerReferences
	if metadata.Labels == nil {
		metadata.Labels = map[string]string{}
	}
	for key, value := range desired.Metadata.Labels {
		metadata.Labels[key] = value
	}
	if metadata.Annotations == nil {
		metadata.Annotations = map[string]string{}
	}
	for key, value := range desired.Metadata.Annotations {
		metadata.Annotations[key] = value
	}
	desired.Metadata = metadata
	return a.request(ctx, "PUT", a.path(desired.Kind)+"/"+url.PathEscape(desired.Metadata.Name), desired, nil)
}
func (a *API) Finalizer(ctx context.Context, r k.Resource, add bool) error {
	next := []string{}
	found := false
	for _, f := range r.Metadata.Finalizers {
		if f == k.Finalizer {
			found = true
			if add {
				next = append(next, f)
			}
		} else {
			next = append(next, f)
		}
	}
	if found == add {
		return nil
	}
	if add {
		next = append(next, k.Finalizer)
	}
	r.Metadata.Finalizers = next
	return a.request(ctx, "PUT", a.path(r.Kind)+"/"+url.PathEscape(r.Metadata.Name), r, nil)
}
func (a *API) Status(ctx context.Context, r k.Resource, status k.Status) error {
	for i := range status.Conditions {
		if r.Status != nil {
			for _, old := range r.Status.Conditions {
				if old.Type == status.Conditions[i].Type && old.Status == status.Conditions[i].Status && old.Reason == status.Conditions[i].Reason {
					status.Conditions[i].LastTransitionTime = old.LastTransitionTime
				}
			}
		}
	}
	if r.Status != nil && model.DefinitionDigest(*r.Status) == model.DefinitionDigest(status) {
		return nil
	}
	body := struct {
		APIVersion string     `json:"apiVersion"`
		Kind       string     `json:"kind"`
		Metadata   k.Metadata `json:"metadata"`
		Status     k.Status   `json:"status"`
	}{r.APIVersion, r.Kind, r.Metadata, status}
	return a.request(ctx, "PUT", a.path(r.Kind)+"/"+url.PathEscape(r.Metadata.Name)+"/status", body, nil)
}

func (a *API) Probe(ctx context.Context) error {
	return a.request(ctx, "GET", a.path("ServiceLevelObjective")+"?limit=1", nil, nil)
}
