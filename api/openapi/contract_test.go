package openapi

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestContractStructureAndRegisteredRoutes(t *testing.T) {
	data, err := os.ReadFile("v1alpha1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err = yaml.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	if spec["openapi"] != "3.1.1" {
		t.Fatal("unexpected OpenAPI version")
	}
	info, ok := spec["info"].(map[string]any)
	if !ok || info["version"] != "0.1.0-alpha.2" {
		t.Fatal("unexpected contract version")
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok || len(paths) != 14 {
		t.Fatal("unexpected contract scope")
	}
	components, ok := spec["components"].(map[string]any)
	if !ok {
		t.Fatal("missing components")
	}
	registered := ""
	for _, f := range []string{"../../internal/httpapi/server.go", "../../internal/intentflow/http.go"} {
		b, e := os.ReadFile(f)
		if e != nil {
			t.Fatal(e)
		}
		registered += string(b) + "\n"
	}
	ids := map[string]bool{}
	for path, item := range paths {
		methods, ok := item.(map[string]any)
		if !ok {
			t.Fatal(path)
		}
		for method, value := range methods {
			operation, ok := value.(map[string]any)
			if !ok {
				t.Fatal(path, method)
			}
			id, ok := operation["operationId"].(string)
			if !ok || id == "" || ids[id] {
				t.Fatal("missing or duplicate operation ID", id)
			}
			ids[id] = true
			needle := `"` + strings.ToUpper(method) + " " + path + `"`
			if !strings.Contains(registered, needle) {
				t.Error("unregistered operation", needle)
			}
			responses, ok := operation["responses"].(map[string]any)
			if !ok || responses["200"] == nil {
				t.Error("missing success response", id)
			}
			params, ok := operation["parameters"].([]any)
			if !ok || len(params) == 0 {
				t.Error("missing tenant parameter", id)
			}
		}
	}
	var checkRefs func(any)
	checkRefs = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, y := range x {
				if k == "$ref" {
					p, ok := y.(string)
					if !ok || !strings.HasPrefix(p, "#/components/") {
						t.Errorf("unsupported ref %v", y)
						continue
					}
					parts := strings.Split(strings.TrimPrefix(p, "#/components/"), "/")
					if len(parts) != 2 {
						t.Errorf("invalid ref %s", p)
						continue
					}
					group, ok := components[parts[0]].(map[string]any)
					if !ok || group[parts[1]] == nil {
						t.Errorf("unresolved ref %s", p)
					}
				} else {
					checkRefs(y)
				}
			}
		case []any:
			for _, y := range x {
				checkRefs(y)
			}
		}
	}
	checkRefs(spec)
}

func TestCriticalSchemaFieldsFollowGoContracts(t *testing.T) {
	data, err := os.ReadFile("v1alpha1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err = yaml.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)
	for name, value := range map[string]any{"NativeSLOResults": v1.NativeSLOResults{}, "NativeSLOCatalog": v1.NativeSLOCatalog{}, "NativeSLODraft": v1.NativeSLODraft{}, "ReviewRequest": v1.ReviewIntentChangeSet{}, "WorkflowStatus": v1.IntentWorkflowStatus{}, "ReviewArtifact": v1.IntentReviewArtifact{}, "IntentConversation": v1.IntentConversation{}} {
		schema := schemas[name].(map[string]any)
		props := schema["properties"].(map[string]any)
		typ := reflect.TypeOf(value)
		fields := map[string]bool{}
		var visit func(reflect.Type)
		visit = func(typ reflect.Type) {
			for i := 0; i < typ.NumField(); i++ {
				f := typ.Field(i)
				if f.Anonymous && f.Tag.Get("json") == "" && f.Type.Kind() == reflect.Struct {
					visit(f.Type)
					continue
				}
				tag := strings.Split(f.Tag.Get("json"), ",")[0]
				if tag != "" && tag != "-" {
					fields[tag] = true
				}
			}
		}
		visit(typ)
		for field := range props {
			if !fields[field] {
				t.Errorf("%s property %s not in Go type", name, field)
			}
		}
		b, e := json.Marshal(value)
		if e != nil {
			t.Fatal(e)
		}
		var encoded map[string]any
		if e = json.Unmarshal(b, &encoded); e != nil {
			t.Fatal(e)
		}
		required, ok := schema["required"].([]any)
		if !ok {
			t.Fatal(name, "has no required fields")
		}
		for _, entry := range required {
			key := entry.(string)
			if _, ok = encoded[key]; !ok {
				t.Errorf("%s requires omitted field %s", name, key)
			}
		}
	}
}

func TestIfMatchRevisionPattern(t *testing.T) {
	data, err := os.ReadFile("v1alpha1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err = yaml.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	params := spec["components"].(map[string]any)["parameters"].(map[string]any)
	schema := params["IfMatch"].(map[string]any)["schema"].(map[string]any)
	re := regexp.MustCompile(schema["pattern"].(string))
	for _, v := range []string{`"0"`, `"42"`} {
		if !re.MatchString(v) {
			t.Fatal("valid revision rejected", v)
		}
	}
	for _, v := range []string{"0", `"-1"`, `"01"`} {
		if re.MatchString(v) {
			t.Fatal("invalid revision accepted", v)
		}
	}
}
