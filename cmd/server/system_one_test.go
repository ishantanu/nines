package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSystemOneTenantOptInWithoutProviderRequests(t *testing.T) {
	clearIntentProviderEnvironment(t)
	t.Setenv("JEV_API_KEY", "test-system-one-credential")
	t.Setenv("TYPESAFE_API_KEY", "")
	t.Setenv("SYSTEM_ONE_ALPHA_KEY", "test-system-one-alpha-credential")
	file := filepath.Join(t.TempDir(), "tenants.yaml")
	body := `tenants:
- id: alpha
  name: Alpha
  systemOne:
    provider: typesafe
    apiKeyEnv: SYSTEM_ONE_ALPHA_KEY
    model: jev-1.14.0
  access:
  - tokenEnv: SYSTEM_ONE_TEST_OWNER
    role: owner
  source:
    kind: manual
- id: beta
  name: Beta
  access:
  - tokenEnv: SYSTEM_ONE_TEST_OWNER
    role: owner
  source:
    kind: manual
`
	t.Setenv("SYSTEM_ONE_TEST_OWNER", "owner-api-key-longer-than-24-characters")
	if err := os.WriteFile(file, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	runtimes, err := initTenants(context.Background(), file, false, true, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, r := range runtimes {
			r.Close()
		}
	}()
	if runtimes[0].Workspace.IntentConversations.SystemOne == nil || runtimes[1].Workspace.IntentConversations.SystemOne != nil {
		t.Fatal("tenant System One routing leaked")
	}
	for _, r := range runtimes {
		if r.Workspace.IntentConversations.Model != nil {
			t.Fatal("Jev key enabled generative model")
		}
	}
	demo, err := initTenants(context.Background(), file, true, true, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, r := range demo {
			r.Close()
		}
	}()
	for _, r := range demo {
		if r.Workspace.IntentConversations.SystemOne != nil {
			t.Fatal("demo enabled Jev")
		}
	}
}
