package mcpserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func remoteYAML() string {
	return `apiVersion: nines.dev/v1alpha1
kind: RemoteMCP
publicURL: https://agents.example/mcp
server: https://nines.example
tenant: alpha
issuer: https://identity.example/realms/nines
introspectionURL: https://identity.example/introspect
introspectionClientID: nines-resource
introspectionSecretEnv: MCP_INTROSPECTION_SECRET
bindings:
  - subject: alice
    clientID: managed
    apiTokenEnv: MCP_ALICE_API_KEY
`
}
func TestRemoteConfigurationIsStrictAndNeverSerializesCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "remote.yaml")
	lookup := func(key string) string {
		switch key {
		case "MCP_INTROSPECTION_SECRET":
			return "synthetic-introspection"
		case "MCP_ALICE_API_KEY":
			return "synthetic-alice"
		}
		return ""
	}
	load := func(body string) (RemoteConfig, error) {
		t.Helper()
		if e := os.WriteFile(path, []byte(body), 0600); e != nil {
			t.Fatal(e)
		}
		return LoadRemoteConfig(path, lookup)
	}
	c, e := load(remoteYAML())
	if e != nil {
		t.Fatal(e)
	}
	b, e := json.Marshal(c)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(b), "synthetic-") {
		t.Fatal("configuration serialized credentials")
	}
	for _, body := range []string{
		remoteYAML() + "unrecognized: true\n",
		remoteYAML() + "---\nkind: Other\n",
		strings.Replace(remoteYAML(), "https://agents.example/mcp", "http://agents.example/mcp", 1),
		strings.Replace(remoteYAML(), "https://identity.example/introspect", "https://other:secret@identity.example/introspect", 1),
		strings.Replace(remoteYAML(), "tenant: alpha", "tenant: ../beta", 1),
		strings.Replace(remoteYAML(), "apiTokenEnv: MCP_ALICE_API_KEY", "apiTokenEnv: MISSING", 1),
		remoteYAML() + "allowRuleDeployment: true\n",
		remoteYAML() + "  - subject: bob\n    clientID: managed\n    apiTokenEnv: MCP_ALICE_API_KEY\n",
		strings.Replace(remoteYAML(), "https://agents.example/mcp", "https://agents.example/mcp?token=secret", 1),
	} {
		if _, e = load(body); e == nil {
			t.Fatal("unsafe remote configuration accepted")
		}
	}
}
