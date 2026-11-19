package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestCLIDeviceClientKeepsBootstrapRequestsUnauthenticated(t *testing.T) {
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("Authorization") != "" {
			t.Error("bootstrap leaked the resource bearer credential")
		}
		if r.URL.Path == "/auth/cli/device/start" {
			var in v1.CLIDeviceStart
			if json.NewDecoder(r.Body).Decode(&in) != nil || in.Tenant != "alpha" {
				t.Error("lost client tenant binding")
			}
		}
		_, _ = w.Write([]byte(`{"apiVersion":"nines.dev/v1alpha1","kind":"CLIAuthenticationCapabilities","status":"authorization_pending"}`))
	}))
	defer server.Close()
	c, err := New(Config{URL: server.URL, Tenant: "alpha", Token: "must-not-send-resource-token"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = c.CLIAuthenticationCapabilities(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = c.StartCLIDeviceAuthorization(ctx, "challenge"); err != nil {
		t.Fatal(err)
	}
	if _, err = c.PollCLIDeviceAuthorization(ctx, v1.CLIDevicePoll{}); err != nil {
		t.Fatal(err)
	}
	if err = c.CancelCLIDeviceAuthorization(ctx, v1.CLIDevicePoll{}); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 4 {
		t.Fatal("missing device operations")
	}
}
