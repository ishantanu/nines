// Package kubediscovery produces reviewable dependency candidates from bounded
// Kubernetes inventory. Configuration is evidence of intent, never observed traffic.
package kubediscovery

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

const MaxBytes = 2 << 20

var dnsName = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var hostName = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]{0,251}[a-z0-9])?$`)
var serviceReference = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_./-]{0,199}$`)
var hostVariable = regexp.MustCompile(`(?i)(^|_)(HOST|HOSTNAME|URL|URI|ENDPOINT|ADDRESS|ADDR|DSN)$`)

func ValidNamespace(s string) bool { return dnsName.MatchString(s) }
func Host(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 2048 || strings.ContainsAny(value, " \t\r\n$(){}") {
		return ""
	}
	if strings.Contains(value, "://") {
		u, err := url.Parse(value)
		if err != nil {
			return ""
		}
		value = u.Hostname()
	} else if strings.Contains(value, ":") {
		u, err := url.Parse("//" + value)
		if err != nil || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return ""
		}
		value = u.Hostname()
	}
	value = strings.ToLower(strings.TrimSuffix(value, "."))
	if !hostName.MatchString(value) || strings.Contains(value, "..") {
		return ""
	}
	return value
}
func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func field(v any, path ...string) any {
	for _, p := range path {
		v = object(v)[p]
	}
	return v
}
func text(v any) string { s, _ := v.(string); return s }
func list(v any) []any  { s, _ := v.([]any); return s }
func labels(v any) map[string]string {
	out := map[string]string{}
	for k, v := range object(v) {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}
func annotation(v any, key string) string { return text(field(v, "metadata", "annotations", key)) }
func podTemplate(v any, kind string) any {
	switch kind {
	case "Pod":
		return v
	case "CronJob":
		return field(v, "spec", "jobTemplate", "spec", "template")
	default:
		return field(v, "spec", "template")
	}
}

// Project reads a Kubernetes List or YAML stream locally and discards everything
// except the explicit inventory contract before anything can be sent to Nines.
func Project(body []byte, namespace string) (v1.KubernetesInventory, error) {
	out := v1.KubernetesInventory{Resources: []v1.KubernetesResource{}}
	if len(body) == 0 || len(body) > MaxBytes || !ValidNamespace(namespace) {
		return out, model.Invalid("provide Kubernetes YAML/JSON up to 2 MiB and a valid default namespace")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	nodes := 0
	var check func(*yaml.Node, int) bool
	check = func(n *yaml.Node, depth int) bool {
		nodes++
		if nodes > 150000 || depth > 40 || n.Kind == yaml.AliasNode {
			return false
		}
		for _, c := range n.Content {
			if !check(c, depth+1) {
				return false
			}
		}
		return true
	}
	var add func(any) error
	count := 0
	add = func(raw any) error {
		kind := text(field(raw, "kind"))
		if kind == "List" || strings.HasSuffix(kind, "List") {
			for _, item := range list(field(raw, "items")) {
				// Typed API lists can omit the kind on each item.
				if child := object(item); child != nil && text(child["kind"]) == "" && kind != "List" {
					child["kind"] = strings.TrimSuffix(kind, "List")
				}
				if err := add(item); err != nil {
					return err
				}
			}
			return nil
		}
		count++
		if count > 1000 {
			return model.Invalid("Kubernetes inventory exceeds 1000 resources")
		}
		switch kind {
		case "Service", "Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob", "Pod", "Ingress":
		default:
			out.IgnoredResources++
			return nil
		}
		r := v1.KubernetesResource{Kind: kind, Name: text(field(raw, "metadata", "name")), Namespace: text(field(raw, "metadata", "namespace")), Service: annotation(raw, "nines.dev/service")}
		if r.Namespace == "" {
			r.Namespace = namespace
		}
		depends := annotation(raw, "nines.dev/depends-on")
		switch kind {
		case "Service":
			r.Selector = labels(field(raw, "spec", "selector"))
			if h := Host(text(field(raw, "spec", "externalName"))); h != "" {
				r.Hosts = []string{h}
			}
		case "Ingress":
			if n := text(field(raw, "spec", "defaultBackend", "service", "name")); n != "" {
				r.Backends = append(r.Backends, n)
			}
			for _, rule := range list(field(raw, "spec", "rules")) {
				for _, path := range list(field(rule, "http", "paths")) {
					if n := text(field(path, "backend", "service", "name")); n != "" {
						r.Backends = append(r.Backends, n)
					}
				}
			}
		default:
			pod := podTemplate(raw, kind)
			r.Labels = labels(field(pod, "metadata", "labels"))
			if value := annotation(pod, "nines.dev/service"); value != "" {
				r.Service = value
			}
			if value := annotation(pod, "nines.dev/depends-on"); value != "" {
				depends = value
			}
			for _, containerKind := range []string{"containers", "initContainers"} {
				for _, container := range list(field(pod, "spec", containerKind)) {
					for _, env := range list(field(container, "env")) {
						if !hostVariable.MatchString(text(field(env, "name"))) {
							continue
						}
						if h := Host(text(field(env, "value"))); h != "" {
							r.Hosts = append(r.Hosts, h)
						} else {
							out.UnresolvedReferences++
						}
					}
					if len(list(field(container, "envFrom"))) > 0 {
						out.UnresolvedReferences++
					}
				}
			}
		}
		if r.Service != "" && !serviceReference.MatchString(r.Service) {
			r.Service = ""
			out.UnresolvedReferences++
		}
		if depends != "" {
			for _, name := range strings.Split(depends, ",") {
				name = strings.TrimSpace(name)
				if serviceReference.MatchString(name) {
					r.DependsOn = append(r.DependsOn, name)
				} else {
					out.UnresolvedReferences++
				}
			}
		}
		out.Resources = append(out.Resources, r)
		return nil
	}
	for {
		var node yaml.Node
		err := decoder.Decode(&node)
		if err == io.EOF {
			break
		}
		if err != nil || !check(&node, 0) {
			return out, model.Invalid("invalid, aliased or excessively nested Kubernetes resource YAML")
		}
		if len(node.Content) == 0 {
			continue
		}
		var raw any
		if node.Decode(&raw) != nil {
			return out, model.Invalid("invalid Kubernetes resource document")
		}
		if err := add(raw); err != nil {
			return out, err
		}
	}
	if len(out.Resources) == 0 {
		return out, model.Invalid("inventory contains no supported Kubernetes resources")
	}
	if err := Validate(out); err != nil {
		return out, err
	}
	return out, nil
}
func Validate(in v1.KubernetesInventory) error {
	if len(in.Resources) < 1 || len(in.Resources) > 1000 || in.IgnoredResources < 0 || in.IgnoredResources > 1000 || in.UnresolvedReferences < 0 || in.UnresolvedReferences > 10000 {
		return model.Invalid("invalid or oversized Kubernetes inventory")
	}
	seen := map[string]bool{}
	for _, r := range in.Resources {
		key := r.Kind + "/" + r.Namespace + "/" + r.Name
		if !ValidNamespace(r.Namespace) || !validResourceName(r.Kind, r.Name) || seen[key] {
			return model.Invalid("resources need unique kind/namespace/name identities")
		}
		seen[key] = true
		switch r.Kind {
		case "Service", "Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob", "Pod", "Ingress":
		default:
			return model.Invalid("unsupported inventory resource kind")
		}
		if r.Service != "" && !serviceReference.MatchString(r.Service) || len(r.Service) > 200 || strings.ContainsAny(r.Service, "\r\n\x00") || len(r.DependsOn) > 40 || len(r.Hosts) > 100 || len(r.Backends) > 100 || len(r.Selector) > 32 || len(r.Labels) > 32 {
			return model.Invalid("inventory resource exceeds discovery bounds")
		}
		for k, v := range r.Labels {
			if len(k) > 253 || len(v) > 63 {
				return model.Invalid("invalid workload labels")
			}
		}
		for k, v := range r.Selector {
			if len(k) > 253 || len(v) > 63 {
				return model.Invalid("invalid service selector")
			}
		}
		for _, h := range r.Hosts {
			if h != Host(h) || strings.Contains(h, ":") || h == "" {
				return model.Invalid("inventory hosts must be sanitized host names, without credentials, paths or ports")
			}
		}
		for _, name := range r.Backends {
			if !dnsName.MatchString(name) {
				return model.Invalid("invalid Ingress backend service name")
			}
		}
		for _, name := range r.DependsOn {
			if !serviceReference.MatchString(name) {
				return model.Invalid("invalid declared dependency reference")
			}
		}
	}
	return nil
}
func resourceID(r v1.KubernetesResource) string {
	return fmt.Sprintf("%s %s/%s", r.Kind, r.Namespace, r.Name)
}

func validResourceName(kind, name string) bool {
	if kind == "Service" {
		return dnsName.MatchString(name)
	}
	return hostName.MatchString(name) && !strings.Contains(name, "..")
}
