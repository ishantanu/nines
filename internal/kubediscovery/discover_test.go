package kubediscovery

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func fixture(t *testing.T) (v1.DependencyDiscoveryInput, []byte) {
	t.Helper()
	body, err := os.ReadFile("testdata/resources.yaml")
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := Project(body, "default")
	if err != nil {
		t.Fatal(err)
	}
	return v1.DependencyDiscoveryInput{Mode: "resources", Name: "commerce", Project: "production", DisplayName: "Commerce", Inventory: &inventory}, body
}
func TestDiscoveryIsDeterministicAndKeepsConfigurationSeparate(t *testing.T) {
	in, _ := fixture(t)
	out, err := Discover(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Manifest.Spec.Services) != 3 || len(out.Manifest.Spec.Dependencies) != 2 || len(out.Evidence) != 2 || len(out.InventoryDigest) != 64 {
		t.Fatalf("unexpected discovery: %+v", out)
	}
	for _, edge := range out.Manifest.Spec.Dependencies {
		if edge.Critical {
			t.Fatal("inferred criticality")
		}
	}
	reversed := append([]v1.KubernetesResource(nil), in.Inventory.Resources...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	in.Inventory.Resources = reversed
	again, err := Discover(in)
	if err != nil || again.YAML != out.YAML || again.InventoryDigest != out.InventoryDigest {
		t.Fatal("inventory order changed output", err)
	}
	encoded, _ := json.Marshal(out)
	for _, private := range []string{"not-for-discovery", "private-cache-reference", "postgres://", "example.invalid", "NetworkPolicy" + " allowed-traffic"} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("retained unrelated or sensitive workload content", private)
		}
	}
	if !strings.Contains(strings.Join(out.Warnings, " "), "not observed traffic") || !strings.Contains(strings.Join(out.Warnings, " "), "unresolved") {
		t.Fatal("missing evidence limitations", out.Warnings)
	}
}
func TestProjectionStripsCredentialsBeforeTransmission(t *testing.T) {
	in, body := fixture(t)
	body = []byte(strings.Replace(string(body), "postgres://database", "postgres://fixture-user:credential-sentinel@database", 1))
	inventory, err := Project(body, "default")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(inventory)
	for _, private := range []string{"fixture-user", "credential-sentinel", "postgres://", "checkout", "private-cache-reference", "DEBUG_TOKEN"} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("unsafe inventory projection", private)
		}
	}
	in.Inventory = &inventory
	if _, err = Discover(in); err != nil {
		t.Fatal(err)
	}
	unsafe := inventory
	unsafe.Resources = append([]v1.KubernetesResource(nil), inventory.Resources...)
	unsafe.Resources[0].Hosts = []string{"https://user:secret@database/path"}
	if Validate(unsafe) == nil {
		t.Fatal("API accepted unprojected host")
	}
}
func TestOnlyActualSelectorsProduceOwnersAndAmbiguityStaysExplicit(t *testing.T) {
	in, _ := fixture(t)
	// Missing labels must not match a selector with an empty expected value.
	in.Inventory.Resources[0].Selector = map[string]string{"missing": ""}
	out, err := Discover(in)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range out.Manifest.Spec.Dependencies {
		if e.Caller == "commerce/deployment/api" && e.Callee == "database" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing label matched an empty selector", out.Manifest)
	}
	in, _ = fixture(t)
	duplicate := in.Inventory.Resources[0]
	duplicate.Name = "api-alias"
	duplicate.Service = "other-api"
	in.Inventory.Resources = append(in.Inventory.Resources, duplicate)
	out, err = Discover(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Manifest.Spec.Dependencies) != 1 || !strings.Contains(strings.Join(out.Warnings, " "), "multiple service identities") {
		t.Fatal("ambiguous workload fabricated a caller", out)
	}
}
func TestExplicitWorkloadIdentitiesResolveWithoutServicesAndIgnoreOrder(t *testing.T) {
	in, _ := fixture(t)
	in.Inventory = &v1.KubernetesInventory{Resources: []v1.KubernetesResource{{Kind: "Deployment", Name: "api", Namespace: "team", Service: "api", DependsOn: []string{"worker"}}, {Kind: "StatefulSet", Name: "worker", Namespace: "team", Service: "worker"}}}
	out, err := Discover(in)
	if err != nil || len(out.Manifest.Spec.Dependencies) != 1 {
		t.Fatal("forward identity reference unresolved", out, err)
	}
}
func TestNamespacesDuplicateResourcesAndYAMLBounds(t *testing.T) {
	for _, input := range []string{"kind: Service\nmetadata: {name: api}\nspec: &a {selector: {app: api}}\nother: *a", "kind: Service\nkind: Service\nmetadata: {name: api}", strings.Repeat(" ", MaxBytes+1)} {
		if _, err := Project([]byte(input), "default"); err == nil {
			t.Fatal("accepted aliased, duplicate or oversized input")
		}
	}
	in, _ := fixture(t)
	in.Inventory.Resources = append(in.Inventory.Resources, in.Inventory.Resources[0])
	if _, err := Discover(in); err == nil {
		t.Fatal("accepted duplicate resource identity")
	}
	if (Config{Namespaces: []string{"*"}}).Validate() == nil || (Config{Namespaces: []string{"team", "team"}}).Validate() == nil {
		t.Fatal("invalid namespace configuration")
	}
}

func TestLongKubernetesWorkloadNamesHaveStableBoundedIdentities(t *testing.T) {
	resource := v1.KubernetesResource{Kind: "Deployment", Namespace: "commerce", Name: strings.Repeat("a", 220) + ".worker"}
	in := v1.DependencyDiscoveryInput{Name: "long-names", Project: "production", DisplayName: "Long names", Inventory: &v1.KubernetesInventory{Resources: []v1.KubernetesResource{resource}}}
	first, err := Discover(in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Discover(in)
	if err != nil || first.YAML != second.YAML || len(first.Manifest.Spec.Services[0].Name) > 200 || len(first.Manifest.Spec.Services[0].DisplayName) > 200 {
		t.Fatal("unbounded or unstable identity", err)
	}
}

func TestTypedListSuppliesMissingItemKind(t *testing.T) {
	inventory, err := Project([]byte(`{"kind":"ServiceList","items":[{"metadata":{"name":"api","namespace":"commerce"},"spec":{"selector":{"app":"api"}}}]}`), "default")
	if err != nil || len(inventory.Resources) != 1 || inventory.Resources[0].Kind != "Service" {
		t.Fatal("typed API list not projected", inventory, err)
	}
}
