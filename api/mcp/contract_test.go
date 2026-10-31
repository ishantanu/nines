package mcpcontract

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/mcpserver"
)

func TestRemoteMCPConfigurationSchemaMatchesRuntime(t *testing.T) {
	b, e := os.ReadFile("v1alpha1.schema.json")
	if e != nil {
		t.Fatal(e)
	}
	var schema struct {
		AdditionalProperties bool                       `json:"additionalProperties"`
		Properties           map[string]json.RawMessage `json:"properties"`
		Required             []string                   `json:"required"`
	}
	if e = json.Unmarshal(b, &schema); e != nil {
		t.Fatal(e)
	}
	fields := map[string]bool{}
	typ := reflect.TypeOf(mcpserver.RemoteConfig{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		tag := strings.Split(f.Tag.Get("yaml"), ",")[0]
		if tag != "" {
			fields[tag] = true
		}
	}
	if len(fields) != len(schema.Properties) || schema.AdditionalProperties {
		t.Fatal("remote configuration contract drifted")
	}
	for field := range fields {
		if _, ok := schema.Properties[field]; !ok {
			t.Fatal("runtime field missing from schema", field)
		}
	}
	for _, key := range schema.Required {
		if !fields[key] {
			t.Fatal("schema requires an unknown field", key)
		}
	}
}
