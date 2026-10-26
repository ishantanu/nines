package slotemplates

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

const ExtensionEngine = "nines.dev/measurement-v1"
const MaxBundleBytes = 1 << 20

var extensionName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,29}$`)
var shortName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,23}$`)
var extensionVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[a-z0-9]+([.-][a-z0-9]+)*)?$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func SHA256(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func strictDocument(data []byte, out any) error {
	if len(data) == 0 || len(data) > MaxBundleBytes {
		return model.Invalid("extension document requires 1–1048576 bytes")
	}
	var node yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if decoder.Decode(&node) != nil {
		return model.Invalid("invalid extension YAML")
	}
	count := 0
	var inspect func(*yaml.Node, int) bool
	inspect = func(n *yaml.Node, depth int) bool {
		count++
		if count > 20000 || depth > 24 || n.Kind == yaml.AliasNode || n.Anchor != "" || n.Tag == "!!merge" {
			return false
		}
		for _, child := range n.Content {
			if !inspect(child, depth+1) {
				return false
			}
		}
		return true
	}
	if !inspect(&node, 0) {
		return model.Invalid("extension YAML aliases, anchors, merge keys or excessive nesting are unsupported")
	}
	decoder = yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(out); err != nil {
		return model.Invalid("invalid extension schema; check fields, types and duplicate keys")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return model.Invalid("provide exactly one extension document")
	}
	return nil
}

// ParseValues applies the same duplicate-key and structural limits to CLI and API inputs.
func ParseValues(data []byte) (v1.TemplateValues, error) {
	var values v1.TemplateValues
	if len(data) > 64<<10 {
		return values, model.Invalid("template values exceed 64 KiB")
	}
	err := strictDocument(data, &values)
	return values, err
}

// ParseBundle validates content and executes only deterministic declarative tests.
// A checksum identifies exact bytes; it does not authenticate a publisher.
func ParseBundle(data []byte) (v1.SLOExtensionBundle, error) {
	var b v1.SLOExtensionBundle
	if err := strictDocument(data, &b); err != nil {
		return b, err
	}
	if b.APIVersion != v1.APIVersion || b.Kind != "SLOExtensionBundle" || !extensionName.MatchString(b.Metadata.Name) || strings.HasPrefix(b.Metadata.Name, "nines") || !validExtensionVersion(b.Metadata.Version) || !text(b.Metadata.Publisher, 200) || !text(b.Metadata.Description, 1000) || b.Spec.Engine != ExtensionEngine {
		return b, model.Invalid("invalid extension identity, semantic version or engine capability")
	}
	if len(b.Spec.Recipes) < 1 || len(b.Spec.Recipes) > 30 || len(b.Spec.Tests) > 120 || len(b.Spec.Categories) > 12 || len(b.Spec.Policies) > 20 || len(b.Spec.Evaluations) > 20 {
		return b, model.Invalid("extension content exceeds contract limits")
	}
	names := map[string]bool{}
	categoryNames := map[string]bool{}
	for _, c := range categories() {
		categoryNames[c.ID] = true
	}
	for _, c := range b.Spec.Categories {
		if !shortName.MatchString(c.ID) || !text(c.Title, 100) || categoryNames[c.ID] {
			return b, model.Invalid("invalid or conflicting extension category")
		}
		categoryNames[c.ID] = true
	}
	policies := map[string]bool{}
	for _, p := range b.Spec.Policies {
		if !shortName.MatchString(p.Name) || policies[p.Name] || !text(p.Description, 1000) {
			return b, model.Invalid("invalid or duplicate policy preset")
		}
		policies[p.Name] = true
	}
	evaluations := map[string]bool{}
	for _, e := range b.Spec.Evaluations {
		if !shortName.MatchString(e.Name) || evaluations[e.Name] || !validExtensionVersion(e.Version) || !text(e.Rubric, 2000) || !text(e.Sampling, 1000) || e.UnknownVerdict != "not_good" || e.MissingVerdict != "unknown" {
			return b, model.Invalid("evaluation contracts require versioned rubric, sampling and explicit unknown/missing semantics")
		}
		if len(e.PopulationLabels) == 0 {
			return b, model.Invalid("evaluation contract requires pinned population labels for rubric/evaluator identity")
		}
		if err := model.ValidateLabels(e.PopulationLabels); err != nil {
			return b, err
		}
		evaluations[e.Name] = true
	}
	usedPolicies := map[string]bool{}
	usedEvaluations := map[string]bool{}
	for _, r := range b.Spec.Recipes {
		if !shortName.MatchString(r.Name) || names[r.Name] || !categoryNames[r.Category] || !shortName.MatchString(r.SLI) || !text(r.Title, 200) || !text(r.Description, 1000) || !text(r.EligibleEvents, 1000) || !text(r.GoodEvents, 1000) || len(r.Requirements) < 1 || len(r.Requirements) > 16 || len(r.Inputs) > 24 {
			return b, model.Invalid("invalid or duplicate measurement recipe")
		}
		names[r.Name] = true
		if r.Policy != "" {
			usedPolicies[r.Policy] = true
		}
		if r.Evaluation != "" {
			usedEvaluations[r.Evaluation] = true
		}
		if r.Evaluation != "" && (r.Manifest.Spec.Measurement == nil || r.Manifest.Spec.Measurement.Type != "availability" || r.Manifest.Spec.Measurement.SuccessLabels["verdict"] != "pass") {
			return b, model.Invalid("evaluation recipes require verdict=pass event measurements")
		}

		for _, requirement := range r.Requirements {
			if !text(requirement, 1000) {
				return b, model.Invalid("invalid recipe requirement")
			}
		}
		if r.Policy != "" && !policies[r.Policy] || r.Evaluation != "" && !evaluations[r.Evaluation] {
			return b, model.Invalid("recipe references an unknown policy or evaluation")
		}
		if r.Manifest.Spec.Measurement == nil || r.Manifest.Spec.Intent != nil || r.Manifest.Spec.Lifecycle != v1.LifecycleExperimental {
			return b, model.Invalid("extension recipe must own an inline experimental measurement without intent provenance")
		}
		if err := model.SLODefinition(r.Manifest).Validate(); err != nil {
			return b, err
		}
		merged := bundleTemplate(b, r, SHA256(data))
		if err := model.SLODefinition(merged.Manifest).Validate(); err != nil {
			return b, err
		}
		fields := map[string]bool{}
		for name, input := range r.Inputs {
			typ, ok := inputTypes[input.Field]
			if !shortName.MatchString(name) || !ok || input.Type != typ || fields[input.Field] || !text(input.Description, 500) || len(input.Enum) > 30 {
				return b, model.Invalid("invalid, duplicate or unsupported typed input field")
			}
			if input.Field == "thresholdSeconds" && r.Manifest.Spec.Measurement.Type != "latency" || input.Field == "successLabels" && r.Manifest.Spec.Measurement.Type != "availability" {
				return b, model.Invalid("input field is incompatible with recipe measurement")
			}
			if r.Evaluation != "" && input.Field == "successLabels" {
				return b, model.Invalid("evaluation recipes cannot redefine the pass verdict")
			}
			fields[input.Field] = true
			if typ != "number" && typ != "integer" && (input.Minimum != nil || input.Maximum != nil) || typ != "string" && len(input.Enum) > 0 {
				return b, model.Invalid("input constraints do not match its type")
			}
			if input.Minimum != nil && !model.Finite(*input.Minimum) || input.Maximum != nil && !model.Finite(*input.Maximum) || input.Minimum != nil && input.Maximum != nil && *input.Minimum > *input.Maximum {
				return b, model.Invalid("invalid input bounds")
			}
			enumSeen := map[string]bool{}
			for _, option := range input.Enum {
				if !text(option, 200) || enumSeen[option] {
					return b, model.Invalid("invalid input enum")
				}
				enumSeen[option] = true
			}
			if input.Default != nil {
				if _, err := checkInput(input, input.Default); err != nil {
					return b, err
				}
			}
		}
	}
	for name := range policies {
		if !usedPolicies[name] {
			return b, model.Invalid("policy presets must be used by a validated recipe")
		}
	}
	for name := range evaluations {
		if !usedEvaluations[name] {
			return b, model.Invalid("evaluation contracts must be used by a validated recipe")
		}
	}
	positive := map[string]bool{}
	negative := map[string]bool{}
	tests := map[string]bool{}
	for _, test := range b.Spec.Tests {
		if !shortName.MatchString(test.Name) || tests[test.Name] || !names[test.Recipe] {
			return b, model.Invalid("invalid conformance test")
		}
		tests[test.Name] = true
		recipe := findRecipe(b, test.Recipe)
		template := bundleTemplate(b, recipe, SHA256(data))
		values := v1.TemplateValues{Inputs: test.Inputs, ExpectedTemplateDigest: template.Digest}
		prepared, err := extensionValues(template, values)
		var draft v1.SLOTemplateDraft
		if err == nil {
			draft, err = renderTemplate(template, prepared)
		}
		if test.ExpectError {
			if test.GoodSelector != "" || test.TotalSelector != "" || err == nil {
				return b, model.Invalid("extension negative conformance test %s failed", test.Name)
			}
			negative[test.Recipe] = true
		} else {
			if err != nil || test.GoodSelector == "" || test.TotalSelector == "" {
				return b, model.Invalid("extension positive conformance test %s failed", test.Name)
			}
			window := model.MeasurementWindow(*draft.Manifest.Spec.Measurement)
			if window.GoodSelector != test.GoodSelector || window.TotalSelector != test.TotalSelector {
				return b, model.Invalid("extension selector conformance test %s failed", test.Name)
			}
			positive[test.Recipe] = true
		}
	}
	for name := range names {
		if !positive[name] || !negative[name] {
			return b, model.Invalid("every recipe needs passing positive and rejection conformance tests")
		}
	}
	return b, nil
}
func validExtensionVersion(version string) bool {
	if len(version) > 80 || !extensionVersion.MatchString(version) {
		return false
	}
	_, prerelease, ok := strings.Cut(version, "-")
	if ok {
		for _, part := range strings.Split(prerelease, ".") {
			if len(part) > 1 && part[0] == '0' && strings.Trim(part, "0123456789") == "" {
				return false
			}
		}
	}
	return true
}
func text(s string, limit int) bool {
	return strings.TrimSpace(s) != "" && len(s) <= limit && !strings.ContainsAny(s, "\x00\r")
}
func findRecipe(b v1.SLOExtensionBundle, name string) v1.ExtensionRecipe {
	for _, r := range b.Spec.Recipes {
		if r.Name == name {
			return r
		}
	}
	return v1.ExtensionRecipe{}
}
func bundleTemplate(b v1.SLOExtensionBundle, r v1.ExtensionRecipe, digest string) v1.SLOTemplate {
	manifest := r.Manifest
	for _, p := range b.Spec.Policies {
		if p.Name == r.Policy {
			if p.Target != nil {
				manifest.Spec.Target = *p.Target
			}
			if p.WindowSeconds != nil {
				manifest.Spec.WindowSeconds = *p.WindowSeconds
			}
			if p.Policies != nil {
				manifest.Spec.Policies = p.Policies
				manifest.Spec.Policy = nil
			}
			if p.Quality != nil {
				manifest.Spec.Quality = p.Quality
			}
		}
	}
	category := r.Category
	for _, c := range b.Spec.Categories {
		if c.ID == category {
			category = b.Metadata.Name + "--" + category
			break
		}
	}
	t := v1.SLOTemplate{ID: b.Metadata.Name + "--" + r.Name, Revision: 1, Category: category, SLI: r.SLI, Title: r.Title, Description: r.Description, EligibleEvents: r.EligibleEvents, GoodEvents: r.GoodEvents, Requirements: r.Requirements, Manifest: manifest, Inputs: r.Inputs, Extension: &v1.ExtensionReference{Name: b.Metadata.Name, Version: b.Metadata.Version, SHA256: digest}}
	for _, e := range b.Spec.Evaluations {
		if e.Name == r.Evaluation {
			copy := e
			t.Evaluation = &copy
		}
	}
	t.Digest = model.DefinitionDigest(t)
	return t
}

var inputTypes = map[string]string{"service": "string", "metric": "string", "labels": "labels", "successLabels": "labels", "thresholdSeconds": "number", "scrapeIntervalSeconds": "integer", "target": "number", "windowSeconds": "integer", "stepSeconds": "integer", "observationPoint": "string"}

func checkInput(schema v1.ExtensionInput, value any) (any, error) {
	// Normalize YAML and JSON values through the same JSON data model.
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, model.Invalid("invalid extension input value")
	}
	var normalized any
	if json.Unmarshal(raw, &normalized) != nil {
		return nil, model.Invalid("invalid extension input")
	}
	switch schema.Type {
	case "string":
		s, ok := normalized.(string)
		if !ok || !text(s, 500) || len(schema.Enum) > 0 && !slices.Contains(schema.Enum, s) {
			return nil, model.Invalid("extension input requires an allowed string")
		}
	case "number", "integer":
		n, ok := normalized.(float64)
		if !ok || !model.Finite(n) || schema.Type == "integer" && (n != float64(int64(n)) || n < 0 || n > 31*86400) || schema.Minimum != nil && n < *schema.Minimum || schema.Maximum != nil && n > *schema.Maximum {
			return nil, model.Invalid("extension input requires a bounded %s", schema.Type)
		}
	case "labels":
		m, ok := normalized.(map[string]any)
		if !ok {
			return nil, model.Invalid("extension input requires equality labels")
		}
		labels := map[string]string{}
		for k, v := range m {
			s, ok := v.(string)
			if !ok {
				return nil, model.Invalid("extension labels must be strings")
			}
			labels[k] = s
		}
		if err := model.ValidateLabels(labels); err != nil {
			return nil, err
		}
		return labels, nil
	default:
		return nil, model.Invalid("unsupported extension input type")
	}
	return normalized, nil
}
func extensionValues(t v1.SLOTemplate, values v1.TemplateValues) (v1.TemplateValues, error) {
	if values.Measurement != nil || values.Target != nil || values.WindowSeconds != nil || values.StepSeconds != nil {
		return values, model.Invalid("extension measurement and policy overrides must use declared inputs")
	}
	// Round-trip prevents callers from mutating registry-owned defaults.
	raw, _ := json.Marshal(t.Manifest)
	var manifest v1.ServiceLevelObjective
	_ = json.Unmarshal(raw, &manifest)
	inputs := map[string]any{}
	for name, value := range values.Inputs {
		if _, ok := t.Inputs[name]; !ok {
			return values, model.Invalid("unknown or invalid extension input")
		}
		inputs[name] = value
	}
	names := make([]string, 0, len(t.Inputs))
	for name := range t.Inputs {
		names = append(names, name)
	}
	sort.Strings(names)
	// Apply service before a complete explicitly supplied population label set.
	sort.SliceStable(names, func(i, j int) bool {
		return t.Inputs[names[i]].Field == "service" && t.Inputs[names[j]].Field != "service"
	})
	for _, name := range names {
		schema := t.Inputs[name]
		value, ok := inputs[name]
		if schema.Field == "service" && values.Service != "" {
			if ok && value != values.Service {
				return values, model.Invalid("conflicting service inputs")
			}
			value = values.Service
			ok = true
		}
		if !ok && schema.Default != nil {
			value = schema.Default
			ok = true
		}
		if !ok {
			if schema.Required {
				return values, model.Invalid("missing required extension input %s", name)
			}
			continue
		}
		normalized, err := checkInput(schema, value)
		if err != nil {
			return values, model.Invalid("extension input %s: %v", name, err)
		}
		m := manifest.Spec.Measurement
		switch schema.Field {
		case "service":
			manifest.Spec.Service = normalized.(string)
			if _, ok := m.Labels["service"]; ok {
				m.Labels["service"] = normalized.(string)
			}
		case "metric":
			m.Metric = normalized.(string)
		case "labels":
			m.Labels = normalized.(map[string]string)
		case "successLabels":
			m.SuccessLabels = normalized.(map[string]string)
		case "observationPoint":
			m.ObservationPoint = normalized.(string)
		case "thresholdSeconds":
			n := normalized.(float64)
			m.ThresholdSeconds = &n
		case "scrapeIntervalSeconds":
			m.ScrapeIntervalSeconds = int64(normalized.(float64))
		case "target":
			manifest.Spec.Target = normalized.(float64)
		case "windowSeconds":
			manifest.Spec.WindowSeconds = int64(normalized.(float64))
		case "stepSeconds":
			manifest.Spec.StepSeconds = int64(normalized.(float64))
		}
	}
	if t.Evaluation != nil {
		for label, value := range t.Evaluation.PopulationLabels {
			if manifest.Spec.Measurement.Labels[label] != value {
				return values, model.Invalid("evaluation population must preserve pinned label %s", label)
			}
		}
	}
	values.Inputs = nil
	values.Service = ""
	values.Measurement = manifest.Spec.Measurement
	target, window, step := manifest.Spec.Target, manifest.Spec.WindowSeconds, manifest.Spec.StepSeconds
	values.Target = &target
	values.WindowSeconds = &window
	values.StepSeconds = &step
	// Service is applied in the same rendered document as the measurement inputs.
	values.Service = manifest.Spec.Service
	return values, nil
}

// Registry is an immutable startup snapshot of built-ins and pinned extensions.
type Registry struct {
	templates  []v1.SLOTemplate
	categories []v1.SLOTemplateCategory
	extensions []v1.SLOExtensionInfo
}

func Default() *Registry { return &Registry{templates: catalog(), categories: categories()} }
func (r *Registry) List(category string) (v1.SLOTemplateCatalog, error) {
	if r == nil {
		r = Default()
	}
	// Return a detached value, including all nested maps and example manifests.
	raw, _ := json.Marshal(v1.SLOTemplateCatalog{APIVersion: v1.APIVersion, Kind: "SLOTemplateCatalog", Templates: r.templates, Categories: r.categories, Extensions: r.extensions, Digest: model.DefinitionDigest(struct {
		Templates  []v1.SLOTemplate
		Categories []v1.SLOTemplateCategory
	}{r.templates, r.categories})})
	var out v1.SLOTemplateCatalog
	_ = json.Unmarshal(raw, &out)
	if category != "" && !slices.ContainsFunc(out.Categories, func(c v1.SLOTemplateCategory) bool { return c.ID == category }) {
		return out, model.Invalid("unknown SLO template category")
	}
	if category != "" {
		out.Templates = slices.DeleteFunc(out.Templates, func(t v1.SLOTemplate) bool { return t.Category != category })
	}
	return out, nil
}
func (r *Registry) Get(id string) (v1.SLOTemplate, error) {
	catalog, err := r.List("")
	if err != nil {
		return v1.SLOTemplate{}, err
	}
	for _, t := range catalog.Templates {
		if t.ID == id {
			return t, nil
		}
	}
	return v1.SLOTemplate{}, model.ErrNotFound
}
func (r *Registry) Render(id string, values v1.TemplateValues) (v1.SLOTemplateDraft, error) {
	t, err := r.Get(id)
	if err != nil {
		return v1.SLOTemplateDraft{}, err
	}
	if values.ExpectedTemplateDigest != "" && values.ExpectedTemplateDigest != t.Digest {
		return v1.SLOTemplateDraft{}, model.ErrRevisionConflict
	}
	if t.Extension != nil {
		if values.ExpectedTemplateDigest == "" {
			return v1.SLOTemplateDraft{}, model.Invalid("extension rendering requires expectedTemplateDigest from the selected catalog entry")
		}
		values, err = extensionValues(t, values)
		if err != nil {
			return v1.SLOTemplateDraft{}, err
		}
	} else if len(values.Inputs) != 0 {
		return v1.SLOTemplateDraft{}, model.Invalid("built-in templates do not declare extension inputs")
	}
	return renderTemplate(t, values)
}
func extensionInfo(b v1.SLOExtensionBundle, digest string) v1.SLOExtensionInfo {
	return v1.SLOExtensionInfo{ExtensionReference: v1.ExtensionReference{Name: b.Metadata.Name, Version: b.Metadata.Version, SHA256: digest}, Publisher: b.Metadata.Publisher, Description: b.Metadata.Description, Engine: b.Spec.Engine, Recipes: len(b.Spec.Recipes), Tests: len(b.Spec.Tests)}
}
func (r *Registry) add(b v1.SLOExtensionBundle, digest string) error {
	if len(r.extensions) >= 32 {
		return model.Invalid("at most 32 installed extensions are supported")
	}
	for _, p := range r.extensions {
		if p.Name == b.Metadata.Name {
			return model.Invalid("duplicate extension identity")
		}
	}
	next := Registry{templates: slices.Clone(r.templates), categories: slices.Clone(r.categories), extensions: slices.Clone(r.extensions)}
	for _, c := range b.Spec.Categories {
		id := b.Metadata.Name + "--" + c.ID
		if slices.ContainsFunc(next.categories, func(existing v1.SLOTemplateCategory) bool { return existing.ID == id }) {
			return model.Invalid("duplicate extension category identifier")
		}
		next.categories = append(next.categories, v1.SLOTemplateCategory{ID: id, Title: c.Title})
	}
	for _, recipe := range b.Spec.Recipes {
		t := bundleTemplate(b, recipe, digest)
		if slices.ContainsFunc(next.templates, func(existing v1.SLOTemplate) bool { return existing.ID == t.ID }) {
			return model.Invalid("duplicate template identifier")
		}
		next.templates = append(next.templates, t)
	}
	next.extensions = append(next.extensions, extensionInfo(b, digest))
	snapshot, err := next.List("")
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil || len(encoded) > 768<<10 {
		return model.Invalid("installed catalog exceeds the bounded 768 KiB response contract")
	}
	*r = next
	return nil
}
