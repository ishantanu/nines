package openapi

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	beta "github.com/ishantanu/nines/api/v1beta1"
)

// Generated schemas follow the Go types the server encodes, so a field cannot
// change without the contract changing. Run with UPDATE_OPENAPI=1 to rewrite
// them in v1alpha1.yaml; the test fails while they differ.
//
// Rules, matching encoding/json:
//   - a field without omitempty is required; with it, optional;
//   - a pointer field without omitempty can be null;
//   - named structs become components, referenced by $ref;
//   - embedded structs without a JSON name are flattened.

var (
	timeType = reflect.TypeOf(time.Time{})
	rawType  = reflect.TypeOf(json.RawMessage{})
	// stringEncoded lists types whose MarshalJSON writes a string. Other
	// custom encoders (such as BindingHealth, which only omits a field) still
	// encode their struct fields.
	stringEncoded = map[reflect.Type]bool{reflect.TypeOf(beta.Duration{}): true}
	// optionalFields are fields a custom MarshalJSON can drop although the
	// struct tag has no omitempty.
	optionalFields = map[reflect.Type][]string{
		reflect.TypeOf(v1.BindingHealth{}):   {"binding"},
		reflect.TypeOf(v1.CompiledBinding{}): {"definition", "binding"},
	}
)

type generator struct {
	names   map[reflect.Type]string
	taken   map[string]reflect.Type
	queue   []reflect.Type
	schemas map[string]any
	t       *testing.T
}

func newGenerator(t *testing.T, named map[string]any) *generator {
	g := &generator{names: map[reflect.Type]string{}, taken: map[string]reflect.Type{}, schemas: map[string]any{}, t: t}
	order := make([]string, 0, len(named))
	for name := range named {
		order = append(order, name)
	}
	sort.Strings(order)
	for _, name := range order {
		typ := reflect.TypeOf(named[name])
		g.names[typ] = name
		g.taken[name] = typ
		g.queue = append(g.queue, typ)
	}
	// Breadth-first in a fixed order, so a name shared by two packages is
	// always given to the same one.
	for len(g.queue) > 0 {
		next := g.queue[0]
		g.queue = g.queue[1:]
		g.schemas[g.names[next]] = g.object(next)
	}
	return g
}

// component names a struct type reached from a registered one: its Go name,
// prefixed with its package when two packages use the same name.
func (g *generator) component(t reflect.Type) string {
	if name, ok := g.names[t]; ok {
		return name
	}
	name := t.Name()
	if other, ok := g.taken[name]; ok && other != t {
		pkg := t.PkgPath()[strings.LastIndex(t.PkgPath(), "/")+1:]
		name = strings.ToUpper(pkg[:1]) + pkg[1:] + name
	}
	g.names[t] = name
	g.taken[name] = t
	g.queue = append(g.queue, t)
	return name
}

func (g *generator) schema(t reflect.Type) map[string]any {
	if name, ok := g.names[t]; ok {
		return map[string]any{"$ref": "#/components/schemas/" + name}
	}
	switch {
	case t == timeType:
		return map[string]any{"type": "string", "format": "date-time"}
	case t == rawType:
		return map[string]any{}
	case stringEncoded[t]:
		// Types whose custom JSON encoding is a string, such as durations.
		return map[string]any{"type": "string"}
	}
	switch t.Kind() {
	case reflect.Pointer:
		return g.schema(t.Elem())
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return map[string]any{"type": "string", "contentEncoding": "base64"}
		}
		return map[string]any{"type": "array", "items": g.schema(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": g.schema(t.Elem())}
	case reflect.Interface:
		return map[string]any{}
	case reflect.Struct:
		if t.Name() == "" {
			return g.object(t)
		}
		return map[string]any{"$ref": "#/components/schemas/" + g.component(t)}
	}
	g.t.Fatalf("no schema rule for %s", t)
	return nil
}

func (g *generator) object(t reflect.Type) map[string]any {
	properties := map[string]any{}
	required := []string{}
	var visit func(reflect.Type)
	visit = func(t reflect.Type) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := f.Tag.Get("json")
			name, opts, _ := strings.Cut(tag, ",")
			if name == "-" || !f.IsExported() && !f.Anonymous {
				continue
			}
			if f.Anonymous && name == "" {
				embedded := f.Type
				if embedded.Kind() == reflect.Pointer {
					embedded = embedded.Elem()
				}
				if embedded.Kind() == reflect.Struct {
					visit(embedded)
					continue
				}
			}
			if name == "" {
				name = f.Name
			}
			s := g.schema(f.Type)
			omit := strings.Contains(opts, "omitempty")
			if f.Type.Kind() == reflect.Pointer && !omit {
				s = map[string]any{"oneOf": []any{s, map[string]any{"type": "null"}}}
			}
			properties[name] = s
			if !omit && !slices.Contains(optionalFields[t], name) {
				required = append(required, name)
			}
		}
	}
	visit(t)
	sort.Strings(required)
	out := map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

// generated is every schema the contract derives from Go types.
func generated(t *testing.T) map[string]any {
	return newGenerator(t, generatedTypes).schemas
}

func TestGeneratedSchemasMatchGoTypes(t *testing.T) {
	path := "v1alpha1.yaml"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc yaml.Node
	if err = yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	// Replacing a schema could drop an anchor that other parts alias, so
	// aliases are expanded into plain copies first.
	expandAliases(&doc)
	schemas := mapping(t, mapping(t, doc.Content[0], "components"), "schemas")
	want := generated(t)
	names := make([]string, 0, len(want))
	for name := range want {
		names = append(names, name)
	}
	sort.Strings(names)
	stale := []string{}
	for _, name := range names {
		current := valueOf(schemas, name)
		var existing map[string]any
		if current != nil {
			_ = current.Decode(&existing)
		}
		// Structure comes from Go; human annotations (descriptions, bounds,
		// patterns, enums) are kept from the spec.
		merged := withAnnotations(want[name].(map[string]any), existing)
		var node yaml.Node
		if err = node.Encode(merged); err != nil {
			t.Fatal(err)
		}
		if current != nil && equalNodes(current, &node) {
			continue
		}
		stale = append(stale, name)
		if current != nil {
			*current = node
		} else {
			schemas.Content = append(schemas.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: name}, &node)
		}
	}
	update := os.Getenv("UPDATE_OPENAPI") == "1"
	if len(stale) == 0 && !update {
		return
	}
	if !update {
		t.Fatalf("schemas differ from Go types: %s; run UPDATE_OPENAPI=1 go test ./api/openapi", strings.Join(stale, ", "))
	}
	sortMapping(schemas)
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err = enc.Encode(&doc); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mapping(t *testing.T, n *yaml.Node, key string) *yaml.Node {
	t.Helper()
	if v := valueOf(n, key); v != nil {
		return v
	}
	t.Fatalf("missing %s", key)
	return nil
}

func valueOf(n *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func equalNodes(a, b *yaml.Node) bool {
	var x, y any
	if a.Decode(&x) != nil || b.Decode(&y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

func sortMapping(n *yaml.Node) {
	type pair struct{ k, v *yaml.Node }
	pairs := []pair{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		pairs = append(pairs, pair{n.Content[i], n.Content[i+1]})
	}
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].k.Value < pairs[j].k.Value })
	n.Content = n.Content[:0]
	for _, p := range pairs {
		n.Content = append(n.Content, p.k, p.v)
	}
}

func expandAliases(n *yaml.Node) {
	for i, c := range n.Content {
		if c.Kind == yaml.AliasNode {
			copied := deepCopy(c.Alias)
			n.Content[i] = copied
			c = copied
		}
		c.Anchor = ""
		expandAliases(c)
	}
}

func deepCopy(n *yaml.Node) *yaml.Node {
	if n.Kind == yaml.AliasNode {
		return deepCopy(n.Alias)
	}
	c := *n
	c.Anchor = ""
	c.Content = make([]*yaml.Node, len(n.Content))
	for i, child := range n.Content {
		c.Content[i] = deepCopy(child)
	}
	return &c
}

var annotationKeys = []string{"description", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "pattern", "enum", "minLength", "maxLength", "minItems", "maxItems", "examples", "deprecated"}

// withAnnotations copies human-written annotations from the spec's current
// schema onto the generated one, at the schema and property level.
func withAnnotations(generated, existing map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range generated {
		out[k] = v
	}
	if existing == nil {
		return out
	}
	for _, k := range annotationKeys {
		if v, ok := existing[k]; ok {
			out[k] = v
		}
	}
	props, _ := generated["properties"].(map[string]any)
	old, _ := existing["properties"].(map[string]any)
	if props == nil || old == nil {
		return out
	}
	merged := map[string]any{}
	for name, p := range props {
		gp, _ := p.(map[string]any)
		op, _ := old[name].(map[string]any)
		if gp == nil || op == nil {
			merged[name] = p
			continue
		}
		next := map[string]any{}
		for k, v := range gp {
			next[k] = v
		}
		for _, k := range annotationKeys {
			if v, ok := op[k]; ok {
				next[k] = v
			}
		}
		merged[name] = next
	}
	out["properties"] = merged
	return out
}
