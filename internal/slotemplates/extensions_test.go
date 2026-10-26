package slotemplates

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func exampleExtension(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../examples/extensions/example-agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func installExample(t *testing.T) (*Registry, string, []byte) {
	t.Helper()
	data := exampleExtension(t)
	lock := filepath.Join(t.TempDir(), "extensions.lock.yaml")
	if _, err := Install(lock, data, SHA256(data), false); err != nil {
		t.Fatal(err)
	}
	r, err := Load(lock)
	if err != nil {
		t.Fatal(err)
	}
	return r, lock, data
}
func extensionTestValues(template v1.SLOTemplate) v1.TemplateValues {
	return v1.TemplateValues{ExpectedTemplateDigest: template.Digest, Inputs: map[string]any{"metric": "support_evaluations_total", "service": "support-agent", "population": map[string]string{"service": "support-agent", "environment": "production", "rubric": "support-v1", "evaluator": "rules-v1"}}}
}
func TestExtensionContractRejectsUnsafeOrAmbiguousBundles(t *testing.T) {
	data := exampleExtension(t)
	bundle, err := ParseBundle(data)
	if err != nil || len(bundle.Spec.Tests) != 3 {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{
		"unknown executable":    []byte(strings.Replace(string(data), "  engine:", "  execute: curl example.invalid\n  engine:", 1)),
		"unknown engine":        []byte(strings.Replace(string(data), ExtensionEngine, "unverified/engine", 1)),
		"duplicate version":     []byte(strings.Replace(string(data), "  version: 1.0.0", "  version: 1.0.0\n  version: 2.0.0", 1)),
		"alias":                 []byte(strings.Replace(string(data), "metadata:", "metadata: &identity", 1)),
		"reserved name":         []byte(strings.Replace(string(data), "name: example-agent", "name: nines-core", 1)),
		"non-semver":            []byte(strings.Replace(string(data), "version: 1.0.0", "version: 1.0.0-01", 1)),
		"wrong golden selector": []byte(strings.Replace(string(data), "verdict=\"pass\"}", "verdict=\"fail\"}", 1)),
		"extra document":        append(append([]byte{}, data...), []byte("\n---\n{}\n")...),
		"oversized":             []byte(strings.Repeat("x", MaxBundleBytes+1)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseBundle(body); err == nil {
				t.Fatal("invalid bundle accepted")
			}
		})
	}
	bundle.Spec.Tests = bundle.Spec.Tests[:1]
	body, _ := yaml.Marshal(bundle)
	if _, err = ParseBundle(body); err == nil {
		t.Fatal("missing negative conformance accepted")
	}
}
func TestPinnedExtensionRenderingAndImmutableSnapshot(t *testing.T) {
	r, lock, data := installExample(t)
	catalog, err := r.List("agent")
	if err != nil || len(catalog.Extensions) != 1 || len(catalog.Templates) != 5 {
		t.Fatal(catalog, err)
	}
	template, err := r.Get("example-agent--evaluation-pass")
	if err != nil {
		t.Fatal(err)
	}
	values := extensionTestValues(template)
	draft, err := r.Render(template.ID, values)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Extension == nil || draft.Extension.SHA256 != SHA256(data) || draft.Manifest.Spec.Service != "support-agent" || draft.Manifest.Spec.Lifecycle != v1.LifecycleExperimental || draft.Manifest.Spec.Quality.MinEvents != 100 || len(draft.Manifest.Spec.Policies) != 2 {
		t.Fatal(draft)
	}
	selectors := model.MeasurementWindow(*draft.Manifest.Spec.Measurement)
	if strings.Contains(selectors.TotalSelector, "verdict") || !strings.Contains(selectors.GoodSelector, `verdict="pass"`) {
		t.Fatal(selectors)
	}
	fresh, err := r.Get(template.ID)
	if err != nil {
		t.Fatal(err)
	}
	fresh.Manifest.Spec.Measurement.Labels["service"] = "changed"
	again, _ := r.Render(template.ID, values)
	if again.YAML != draft.YAML {
		t.Fatal("registry mutated by caller")
	}
	for name, mutate := range map[string]func(*v1.TemplateValues){
		"missing pin":      func(v *v1.TemplateValues) { v.ExpectedTemplateDigest = "" },
		"stale pin":        func(v *v1.TemplateValues) { v.ExpectedTemplateDigest = strings.Repeat("a", 64) },
		"undeclared input": func(v *v1.TemplateValues) { v.Inputs["endpoint"] = "https://other.invalid" },
		"wrong type":       func(v *v1.TemplateValues) { v.Inputs["metric"] = 17 },
		"missing required": func(v *v1.TemplateValues) { delete(v.Inputs, "metric") },
		"out of bounds":    func(v *v1.TemplateValues) { v.Inputs["target"] = .5 },
		"label injection":  func(v *v1.TemplateValues) { v.Inputs["population"] = map[string]string{"le": "1"} },
		"bypass schema":    func(v *v1.TemplateValues) { v.Measurement = draft.Manifest.Spec.Measurement },
	} {
		t.Run(name, func(t *testing.T) {
			v := extensionTestValues(template)
			mutate(&v)
			if _, err := r.Render(template.ID, v); err == nil {
				t.Fatal("invalid values accepted")
			}
		})
	}
	upgraded := []byte(strings.Replace(string(data), "version: 1.0.0", "version: 1.0.1", 1))
	if _, err = Install(lock, upgraded, SHA256(upgraded), false); err == nil {
		t.Fatal("implicit upgrade")
	}
	if _, err = Install(lock, upgraded, SHA256(upgraded), true); err != nil {
		t.Fatal(err)
	}
	newer, err := Load(lock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = newer.Render(template.ID, values); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("stale catalog pin accepted", err)
	}
	if old, err := r.Render(template.ID, values); err != nil || old.YAML != draft.YAML {
		t.Fatal("live registry changed behind caller", err)
	}
	mutated := []byte(strings.Replace(string(upgraded), "Count passing evaluation verdicts", "Count valid evaluation verdicts", 1))
	if _, err = Install(lock, mutated, SHA256(mutated), true); err == nil {
		t.Fatal("same version changed content")
	}
}
func TestExtensionLockDigestPathsAndNoFallback(t *testing.T) {
	_, lock, data := installExample(t)
	root := filepath.Dir(lock)
	if _, err := Install(filepath.Join(t.TempDir(), "bad.lock.yaml"), data, strings.Repeat("0", 64), false); err == nil {
		t.Fatal("wrong checksum")
	}
	var manifest v1.SLOExtensionLock
	raw, _ := os.ReadFile(lock)
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	original := manifest.Extensions[0]
	for _, path := range []string{"../secret.yaml", "/absolute.yaml", "https://example.invalid/file", "nested\\file"} {
		manifest.Extensions[0] = original
		manifest.Extensions[0].Path = path
		body, _ := yaml.Marshal(manifest)
		if err := os.WriteFile(lock, body, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(lock); err == nil {
			t.Fatal("unsafe path accepted", path)
		}
	}
	manifest.Extensions[0] = original
	body, _ := yaml.Marshal(manifest)
	if err := os.WriteFile(lock, body, 0600); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(root, filepath.FromSlash(original.Path))
	if err := os.WriteFile(artifact, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(lock); err == nil {
		t.Fatal("corruption silently fell back")
	}
	if err := os.Remove(artifact); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "outside.yaml")
	if err := os.WriteFile(other, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, artifact); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(lock); err == nil {
		t.Fatal("symlink artifact accepted")
	}
	if _, err := ReadBundleFile(artifact); err == nil {
		t.Fatal("symlink install source accepted")
	}
	if _, err := Load(filepath.Join(root, "missing.lock.yaml")); err == nil {
		t.Fatal("missing configured lock silently fell back")
	}
}

func TestExtensionValidationDoesNotEchoInvalidScalarValues(t *testing.T) {
	private := "private-input-do-not-echo"
	_, err := ParseValues([]byte("target: " + private + "\n"))
	if err == nil || strings.Contains(err.Error(), private) {
		t.Fatal("unsafe parse error", err)
	}
	data := []byte(strings.Replace(string(exampleExtension(t)), "target: 0.99", "target: "+private, 1))
	_, err = ParseBundle(data)
	if err == nil || strings.Contains(err.Error(), private) {
		t.Fatal("unsafe bundle error", err)
	}
}
func TestExtensionCategoryCollisionsFailAtomically(t *testing.T) {
	original, err := ParseBundle(exampleExtension(t))
	if err != nil {
		t.Fatal(err)
	}
	first := original
	first.Metadata.Name = "example"
	first.Spec.Categories = []v1.SLOTemplateCategory{{ID: "agents--x", Title: "Agents"}}
	raw, _ := yaml.Marshal(first)
	var second v1.SLOExtensionBundle
	if err = yaml.Unmarshal(raw, &second); err != nil {
		t.Fatal(err)
	}
	first.Spec.Recipes[0].Category = "agents--x"
	second.Metadata.Name = "example--agents"
	second.Spec.Categories = []v1.SLOTemplateCategory{{ID: "x", Title: "Other agents"}}
	second.Spec.Recipes[0].Category = "x"
	registry := Default()
	if err = registry.add(first, "first"); err != nil {
		t.Fatal(err)
	}
	before, _ := registry.List("")
	if err = registry.add(second, "second"); err == nil {
		t.Fatal("duplicate category accepted")
	}
	after, _ := registry.List("")
	if after.Digest != before.Digest || len(after.Extensions) != len(before.Extensions) {
		t.Fatal("failed addition mutated registry")
	}
}
