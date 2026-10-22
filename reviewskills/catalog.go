// Package reviewskills embeds reviewed execution contracts. Bundles are shipped with
// the binary, never loaded from a caller, URL, filesystem path or model response.
package reviewskills

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/google/jsonschema-go/jsonschema"
	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

//go:embed bundles/*/*/skill.yaml
var bundles embed.FS
var ErrUnavailable = errors.New("review skill version or digest unavailable")

type compiled struct {
	skill         v1.ReviewSkill
	input, output *jsonschema.Resolved
}

var catalog = load()

func load() []compiled {
	entries, err := bundles.ReadDir("bundles")
	if err != nil {
		panic(err)
	}
	var out []compiled
	for _, entry := range entries {
		versions, err := bundles.ReadDir("bundles/" + entry.Name())
		if err != nil {
			panic(err)
		}
		for _, version := range versions {
			data, err := bundles.ReadFile("bundles/" + entry.Name() + "/" + version.Name() + "/skill.yaml")
			if err != nil {
				panic(err)
			}
			var skill v1.ReviewSkill
			d := yaml.NewDecoder(bytes.NewReader(data))
			d.KnownFields(true)
			if err = d.Decode(&skill); err != nil {
				panic(err)
			}
			var extra any
			if d.Decode(&extra) != io.EOF {
				panic("multiple skill documents")
			}
			if skill.APIVersion != v1.APIVersion || skill.Kind != "ReviewSkill" || skill.Metadata.Name != entry.Name() || skill.Metadata.Version != version.Name() || skill.Spec.Mode != "model" && skill.Spec.Mode != "deterministic" {
				panic("invalid embedded skill identity")
			}
			if len(skill.Spec.Instructions) == 0 || len(skill.Spec.Instructions) > 4000 {
				panic("invalid skill instructions")
			}
			for _, obligation := range []string{"captured_context", "deterministic_assessment", "exact_digest"} {
				if !slices.Contains(skill.Spec.EvidenceObligations, obligation) {
					panic("missing evidence obligation")
				}
			}
			if len(skill.Spec.EvidenceObligations) != 3 {
				panic("unsupported evidence obligation")
			}
			if skill.Spec.Mode == "model" && !slices.Contains(skill.Spec.ToolAllowlist, "finish_review") {
				panic("missing final tool")
			}
			for _, tool := range skill.Spec.ToolAllowlist {
				if !slices.Contains([]string{"read_context", "assess_evidence", "read_objective", "read_assessment", "finish_review"}, tool) {
					panic("unknown skill capability")
				}
			}
			for _, tool := range skill.Spec.RequiredTools {
				if !slices.Contains(skill.Spec.ToolAllowlist, tool) {
					panic("undeclared required capability")
				}
			}
			b, _ := json.Marshal(skill)
			skill.Digest = Digest(b)
			resolve := func(schema map[string]any) *jsonschema.Resolved {
				b, _ := json.Marshal(schema)
				var s jsonschema.Schema
				if err := json.Unmarshal(b, &s); err != nil {
					panic(err)
				}
				r, err := s.Resolve(nil)
				if err != nil {
					panic(err)
				}
				return r
			}
			out = append(out, compiled{skill: skill, input: resolve(skill.Spec.InputSchema), output: resolve(skill.Spec.OutputSchema)})
		}
	}
	return out
}
func Digest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func Catalog() []v1.ReviewSkill {
	out := []v1.ReviewSkill{}
	for _, c := range catalog {
		b, _ := json.Marshal(c.skill)
		var s v1.ReviewSkill
		_ = json.Unmarshal(b, &s)
		out = append(out, s)
	}
	return out
}
func Ref(s v1.ReviewSkill) v1.ReviewSkillRef {
	return v1.ReviewSkillRef{ID: s.Metadata.Name, Version: s.Metadata.Version, Digest: s.Digest}
}
func Get(ref v1.ReviewSkillRef) (v1.ReviewSkill, error) {
	for _, s := range Catalog() {
		if Ref(s) == ref {
			return s, nil
		}
	}
	return v1.ReviewSkill{}, ErrUnavailable
}
func Default(mode string) v1.ReviewSkillRef {
	id := "evidence-check"
	if mode == "model" {
		id = "reliability-review"
	}
	for _, c := range catalog {
		if c.skill.Metadata.Name == id && c.skill.Metadata.Version == "1.0.0" {
			return Ref(c.skill)
		}
	}
	panic("missing default skill")
}
func Validate(ref v1.ReviewSkillRef, output bool, value any) error {
	for _, c := range catalog {
		if Ref(c.skill) == ref {
			b, err := json.Marshal(value)
			if err != nil {
				return err
			}
			var v any
			if json.Unmarshal(b, &v) != nil {
				return errors.New("invalid skill value")
			}
			schema := c.input
			if output {
				schema = c.output
			}
			if err = schema.Validate(v); err != nil {
				return fmt.Errorf("skill schema mismatch: %w", err)
			}
			return nil
		}
	}
	return ErrUnavailable
}
