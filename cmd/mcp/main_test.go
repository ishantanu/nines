package main

import (
	"context"
	"net/http"
	"testing"
)

func TestAPIKeyEnvironmentMatchesCLI(t *testing.T) {
	for _, tc := range []struct{ name, primary, legacy, want string }{
		{"primary", "primary-test", "", "primary-test"},
		{"legacy", "", "legacy-test", "legacy-test"},
		{"precedence", "primary-test", "legacy-test", "primary-test"},
		{"absent", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NINES_API_KEY", tc.primary)
			t.Setenv("API_TOKEN", tc.legacy)
			if got := apiToken(); got != tc.want {
				t.Fatal("CLI/MCP credential precedence differs")
			}
		})
	}
}

func TestRemoteListenRequiresTLSOutsideLoopback(t *testing.T) {
	for _, address := range []string{"0.0.0.0:0", ":0", "localhost:0", "invalid"} {
		if e := serveRemote(context.Background(), address, "", "", http.NotFoundHandler()); e == nil {
			t.Fatal("insecure remote listener accepted", address)
		}
	}
	if e := serveRemote(context.Background(), "127.0.0.1:0", "cert.pem", "", http.NotFoundHandler()); e == nil {
		t.Fatal("partial TLS configuration accepted")
	}
}
