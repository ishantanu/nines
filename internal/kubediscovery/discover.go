package kubediscovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// ValidateRequest checks the map identity before any optional provider read.
func ValidateRequest(in v1.DependencyDiscoveryInput) error {
	return model.ServiceDependencyMap(v1.ServiceDependencyMap{
		APIVersion: v1.APIVersion, Kind: v1.DependencyMapKind,
		Metadata: v1.Metadata{Name: in.Name, Project: in.Project},
		Spec:     v1.DependencyMapSpec{DisplayName: in.DisplayName, Services: []v1.MapService{{Name: "preview", DisplayName: "Preview"}}},
	}).Validate()
}

func Discover(in v1.DependencyDiscoveryInput) (v1.DependencyDiscovery, error) {
	out := v1.DependencyDiscovery{APIVersion: v1.APIVersion, Evidence: []v1.DependencyEvidence{}, Warnings: []string{"Configuration-derived links are candidates, not observed traffic. Review each dependency and its criticality before saving. Service selectors identify backing workloads; NetworkPolicy is not evidence of a call."}}
	if err := ValidateRequest(in); err != nil {
		return out, err
	}
	if in.Inventory == nil {
		return out, model.Invalid("a sanitized Kubernetes inventory is required")
	}
	if err := Validate(*in.Inventory); err != nil {
		return out, err
	}
	inventory := *in.Inventory
	inventory.Resources = append([]v1.KubernetesResource(nil), inventory.Resources...)
	sort.Slice(inventory.Resources, func(i, j int) bool { return resourceID(inventory.Resources[i]) < resourceID(inventory.Resources[j]) })
	for i, r := range inventory.Resources {
		r.Hosts = sortedUnique(r.Hosts)
		r.Backends = sortedUnique(r.Backends)
		r.DependsOn = sortedUnique(r.DependsOn)
		inventory.Resources[i] = r
	}
	encoded, _ := json.Marshal(inventory)
	sum := sha256.Sum256(encoded)
	out.InventoryDigest = hex.EncodeToString(sum[:])
	g := v1.ServiceDependencyMap{APIVersion: v1.APIVersion, Kind: v1.DependencyMapKind, Metadata: v1.Metadata{Name: in.Name, Project: in.Project}, Spec: v1.DependencyMapSpec{DisplayName: in.DisplayName, Services: []v1.MapService{}, Dependencies: []v1.DeclaredDependency{}}}
	names := map[string]bool{}
	serviceNames := map[string]string{}
	services := []v1.KubernetesResource{}
	addNode := func(name, display string) {
		if !names[name] {
			names[name] = true
			if len(display) > 200 {
				display = display[:200]
			}
			g.Spec.Services = append(g.Spec.Services, v1.MapService{Name: name, DisplayName: display, SLORefs: []string{}})
		}
	}
	for _, r := range inventory.Resources {
		if r.Kind == "Service" {
			name := r.Service
			if name == "" {
				name = r.Namespace + "/" + r.Name
			}
			serviceNames[r.Namespace+"/"+r.Name] = name
			services = append(services, r)
			addNode(name, r.Name)
		}
	}
	unresolved, ambiguous := inventory.UnresolvedReferences, 0
	owners := map[string]string{}
	for _, r := range inventory.Resources {
		caller := r.Service
		if r.Kind == "Service" {
			caller = serviceNames[r.Namespace+"/"+r.Name]
		} else if caller == "" && r.Kind != "Ingress" {
			matches := []string{}
			for _, svc := range services {
				if svc.Namespace != r.Namespace || len(svc.Selector) == 0 {
					continue
				}
				matched := true
				for k, v := range svc.Selector {
					if actual, ok := r.Labels[k]; !ok || actual != v {
						matched = false
						break
					}
				}
				if matched {
					matches = append(matches, serviceNames[svc.Namespace+"/"+svc.Name])
				}
			}
			matches = sortedUnique(matches)
			if len(matches) > 1 {
				ambiguous++
				continue
			}
			if len(matches) == 1 {
				caller = matches[0]
			}
		}
		if caller == "" {
			caller = resourceIdentity(r)
		}
		addNode(caller, r.Name)
		owners[resourceID(r)] = caller
	}

	resolve := func(reference, ns string) string {
		if names[reference] {
			return reference
		}
		if n := serviceNames[ns+"/"+reference]; n != "" {
			return n
		}
		parts := strings.Split(reference, ".")
		if len(parts) == 2 || len(parts) == 3 && parts[2] == "svc" || len(parts) == 5 && strings.Join(parts[2:], ".") == "svc.cluster.local" {
			return serviceNames[parts[1]+"/"+parts[0]]
		}
		return serviceNames[reference]
	}
	edges := map[string]bool{}
	evidenceOmitted := 0
	addEdge := func(caller, callee, reason string, r v1.KubernetesResource) {
		if callee == "" {
			unresolved++
			return
		}
		if caller == callee {
			return
		}
		key := caller + "\x00" + callee
		if !edges[key] {
			edges[key] = true
			g.Spec.Dependencies = append(g.Spec.Dependencies, v1.DeclaredDependency{Origin: "kubernetes", Caller: caller, Callee: callee, Description: resourceID(r) + ": " + reason + " (configuration candidate)"})
		}
		if len(out.Evidence) < 200 {
			out.Evidence = append(out.Evidence, v1.DependencyEvidence{Caller: caller, Callee: callee, Resource: resourceID(r), Reason: reason})
		} else {
			evidenceOmitted++
		}
	}

	for _, r := range inventory.Resources {
		caller, ok := owners[resourceID(r)]
		if !ok {
			continue
		}

		for _, ref := range r.DependsOn {
			addEdge(caller, resolve(ref, r.Namespace), "declared dependency annotation", r)
		}
		reason := "configured service host"
		if r.Kind == "Service" {
			reason = "ExternalName route"
		}
		for _, host := range r.Hosts {
			addEdge(caller, resolve(host, r.Namespace), reason, r)
		}
		for _, backend := range r.Backends {
			addEdge(caller, resolve(backend, r.Namespace), "Ingress backend route", r)
		}
	}
	if evidenceOmitted > 0 {
		out.Warnings = append(out.Warnings, fmt.Sprintf("%d additional evidence records were omitted; narrow the inventory to inspect them.", evidenceOmitted))
	}
	if unresolved > 0 {
		out.Warnings = append(out.Warnings, fmt.Sprintf("%d references were unresolved or indirect. Secrets, ConfigMaps, external hosts and variable substitution are not resolved.", unresolved))
	}
	if ambiguous > 0 {
		out.Warnings = append(out.Warnings, fmt.Sprintf("%d workloads matched multiple service identities; their links were skipped. Use nines.dev/service to select an identity explicitly.", ambiguous))
	}
	if inventory.IgnoredResources > 0 {
		out.Warnings = append(out.Warnings, fmt.Sprintf("%d unsupported resources were ignored; no Secret or ConfigMap contents are used.", inventory.IgnoredResources))
	}
	sort.Slice(g.Spec.Services, func(i, j int) bool { return g.Spec.Services[i].Name < g.Spec.Services[j].Name })
	sort.Slice(g.Spec.Dependencies, func(i, j int) bool {
		return g.Spec.Dependencies[i].Caller+"\x00"+g.Spec.Dependencies[i].Callee < g.Spec.Dependencies[j].Caller+"\x00"+g.Spec.Dependencies[j].Callee
	})
	if err := model.ServiceDependencyMap(g).Validate(); err != nil {
		return out, err
	}
	out.Manifest = g
	body, err := yaml.Marshal(g)
	out.YAML = string(body)
	return out, err
}
func sortedUnique(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	n := 0
	for _, v := range out {
		if n == 0 || out[n-1] != v {
			out[n] = v
			n++
		}
	}
	return out[:n]
}

func resourceIdentity(r v1.KubernetesResource) string {
	value := r.Namespace + "/" + strings.ToLower(r.Kind) + "/" + r.Name
	if len(value) <= 200 {
		return value
	}
	sum := sha256.Sum256([]byte(value))
	return value[:180] + "-" + hex.EncodeToString(sum[:8])
}
