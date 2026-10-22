package reviewskills

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestCatalogCopiesAndSchemaValidation(t *testing.T) {
	c := Catalog()
	if len(c) != 3 {
		t.Fatal(len(c))
	}
	ref := Default("model")
	skill, err := Get(ref)
	if err != nil || len(ref.Digest) != 64 {
		t.Fatal(err)
	}
	skill.Spec.InputSchema["type"] = "string"
	skill.Spec.ToolAllowlist[0] = "shell"
	fresh, _ := Get(ref)
	if fresh.Spec.InputSchema["type"] != "object" || fresh.Spec.ToolAllowlist[0] == "shell" {
		t.Fatal("mutable catalog")
	}
	for _, value := range []any{map[string]any{}, map[string]any{"question": "ok", "tenant": "other"}, map[string]any{"question": strings.Repeat("x", 4001)}} {
		if Validate(ref, false, value) == nil {
			t.Fatal("invalid input accepted")
		}
	}
	if err := Validate(ref, false, map[string]any{"question": "Review the evidence"}); err != nil {
		t.Fatal(err)
	}
	if _, err = Get(v1.ReviewSkillRef{ID: ref.ID, Version: ref.Version, Digest: "wrong"}); err == nil {
		t.Fatal("digest mismatch accepted")
	}
}

func TestSchemasRejectUnsupportedOutputs(t *testing.T) {
	for _, skill := range Catalog() {
		ref := Ref(skill)
		if Validate(ref, true, map[string]any{"summary": "looks healthy", "evidenceDigests": []string{strings.Repeat("a", 64)}, "limitations": []string{}, "tenant": "beta"}) == nil {
			t.Fatal("extra output field accepted", ref.ID)
		}
		if skill.Metadata.Name == "measurement-review" && Validate(ref, true, map[string]any{"summary": "missing limits", "evidenceDigests": []string{strings.Repeat("a", 64)}, "limitations": []string{}}) == nil {
			t.Fatal("measurement limitation is required")
		}
	}
}

// Pin the development contracts against accidental changes. Intentional schema
// changes can update this baseline before release; released versions must remain immutable.
func TestPinnedSkillDigestsRemainAvailable(t *testing.T) {
	b, err := os.ReadFile("testdata/released-digests.json")
	if err != nil {
		t.Fatal(err)
	}
	var frozen map[string]string
	if err = json.Unmarshal(b, &frozen); err != nil {
		t.Fatal(err)
	}
	for key, digest := range frozen {
		parts := strings.Split(key, "@")
		if len(parts) != 2 {
			t.Fatal(key)
		}
		if _, err := Get(v1.ReviewSkillRef{ID: parts[0], Version: parts[1], Digest: digest}); err != nil {
			t.Fatalf("pinned contract changed or removed: %s", key)
		}
	}
}
