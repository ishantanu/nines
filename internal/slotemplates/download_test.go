package slotemplates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHTTPSExtensionArtifactPinningAndNoRedirects(t *testing.T) {
	data := exampleExtension(t)
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("download inherited authority")
		}
		switch r.URL.Path {
		case "/bundle.yaml":
			_, _ = w.Write(data)
		case "/redirect":
			http.Redirect(w, r, "/bundle.yaml", http.StatusFound)
		case "/large":
			_, _ = w.Write([]byte(strings.Repeat("x", MaxBundleBytes+1)))
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	transport := server.Client().Transport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	got, err := fetchBundle(context.Background(), server.URL+"/bundle.yaml", SHA256(data), transport)
	if err != nil || string(got) != string(data) {
		t.Fatal(err)
	}
	before := calls.Load()
	if _, err = fetchBundle(context.Background(), server.URL+"/redirect", SHA256(data), transport); err == nil || calls.Load() != before+1 {
		t.Fatal("redirect followed")
	}
	for _, tc := range []struct{ url, digest string }{{server.URL + "/bundle.yaml", strings.Repeat("0", 64)}, {server.URL + "/large", SHA256(data)}, {server.URL + "/missing", SHA256(data)}, {strings.Replace(server.URL, "https:", "http:", 1), SHA256(data)}, {server.URL + "/bundle.yaml?token=private", SHA256(data)}} {
		if _, err = fetchBundle(context.Background(), tc.url, tc.digest, transport); err == nil {
			t.Fatal("invalid artifact accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = fetchBundle(ctx, server.URL+"/bundle.yaml", SHA256(data), transport); err == nil {
		t.Fatal("cancelled fetch completed")
	}
}
