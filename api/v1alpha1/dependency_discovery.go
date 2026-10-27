package v1alpha1

// KubernetesResource contains only discovery metadata. Credentials, literal URLs,
// Secret/ConfigMap contents, container commands and pod status are never accepted.
type KubernetesResource struct {
	Kind      string            `json:"kind"`
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Labels    map[string]string `json:"labels,omitempty"`
	Selector  map[string]string `json:"selector,omitempty"`
	Service   string            `json:"service,omitempty"`
	DependsOn []string          `json:"dependsOn,omitempty"`
	Hosts     []string          `json:"hosts,omitempty"`
	Backends  []string          `json:"backends,omitempty"`
}
type KubernetesInventory struct {
	Resources            []KubernetesResource `json:"resources"`
	IgnoredResources     int                  `json:"ignoredResources"`
	UnresolvedReferences int                  `json:"unresolvedReferences"`
}
type DependencyDiscoveryInput struct {
	Mode        string               `json:"mode"`
	Namespace   string               `json:"namespace"`
	Name        string               `json:"name"`
	Project     string               `json:"project"`
	DisplayName string               `json:"displayName"`
	Inventory   *KubernetesInventory `json:"inventory,omitempty"`
}
type DependencyEvidence struct {
	Caller   string `json:"caller"`
	Callee   string `json:"callee"`
	Resource string `json:"resource"`
	Reason   string `json:"reason"`
}
type DependencyDiscovery struct {
	APIVersion      string               `json:"apiVersion"`
	Tenant          string               `json:"tenant,omitempty"`
	InventoryDigest string               `json:"inventoryDigest"`
	Manifest        ServiceDependencyMap `json:"manifest"`
	YAML            string               `json:"yaml"`
	Evidence        []DependencyEvidence `json:"evidence"`
	Warnings        []string             `json:"warnings"`
}
