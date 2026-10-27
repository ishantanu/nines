package kubediscovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ishantanu/nines/internal/model"
)

func TestClusterReadsOnlyAllowedKindsAndRotatedTokenWithPagination(t *testing.T) {
	_, body := fixture(t)
	decoder := yaml.NewDecoder(strings.NewReader(string(body)))
	byKind := map[string][]map[string]any{}
	for {
		var raw map[string]any
		err := decoder.Decode(&raw)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		kind := text(raw["kind"])
		delete(raw, "kind")
		byKind[kind] = append(byKind[kind], raw)
	}
	token := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(token, []byte("initial-fixture-token"), 0600); err != nil {
		t.Fatal(err)
	}
	var reads atomic.Int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := reads.Add(1)
		want := "rotated-fixture-token"
		if count == 1 {
			want = "initial-fixture-token"
		}
		if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer "+want || r.URL.Query().Get("limit") != "100" {
			t.Error("unsafe or unauthenticated inventory read")
			http.Error(w, "bad request", 400)
			return
		}
		resources := map[string]string{"services": "Service", "deployments": "Deployment", "statefulsets": "StatefulSet", "daemonsets": "DaemonSet", "ingresses": "Ingress"}
		path := strings.Split(r.URL.Path, "/")
		kind := resources[path[len(path)-1]]
		if kind == "" || !strings.Contains(r.URL.Path, "/namespaces/commerce/") {
			t.Error("read outside declared inventory")
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		items := byKind[kind]
		next := ""
		if kind == "Service" {
			if r.URL.Query().Get("continue") == "" {
				items = items[:1]
				next = "next-page"
			} else {
				items = items[1:]
			}
		}
		if count == 1 {
			if err := os.WriteFile(token, []byte("rotated-fixture-token"), 0600); err != nil {
				t.Error(err)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "metadata": map[string]string{"continue": next}})
	}))
	defer upstream.Close()
	c := &Cluster{Config: Config{Namespaces: []string{"commerce"}}, Base: upstream.URL, TokenFile: token, HTTP: upstream.Client()}
	inventory, err := c.Read(context.Background(), "commerce")
	if err != nil || len(inventory.Resources) != 4 || reads.Load() != 6 {
		t.Fatalf("inventory: %+v calls:%d err:%v", inventory, reads.Load(), err)
	}
	if _, err = c.Read(context.Background(), "other"); err == nil || reads.Load() != 6 {
		t.Fatal("unconfigured namespace reached provider")
	}
	names := c.Namespaces()
	names[0] = "other"
	if c.Namespaces()[0] != "commerce" {
		t.Fatal("namespace allowlist mutable through capabilities")
	}
}
func TestClusterRejectsTLSRedirectsAndDoesNotReturnProviderBodies(t *testing.T) {
	token := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(token, []byte("fixture-only-token"), 0600); err != nil {
		t.Fatal(err)
	}
	var sinkCalls atomic.Int32
	sink := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { sinkCalls.Add(1) }))
	defer sink.Close()
	for _, status := range []int{302, 403} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", sink.URL)
				w.WriteHeader(status)
				_, _ = w.Write([]byte("private-provider-body"))
			}))
			defer upstream.Close()
			c := &Cluster{Config: Config{Namespaces: []string{"commerce"}}, Base: upstream.URL, TokenFile: token, HTTP: upstream.Client()}
			_, err := c.Read(context.Background(), "commerce")
			if !errors.Is(err, model.ErrUnavailable) || strings.Contains(err.Error(), "private-provider-body") || sinkCalls.Load() != 0 {
				t.Fatal("unsafe redirect/error handling", err)
			}
			c.HTTP = http.DefaultClient
			if _, err = c.Read(context.Background(), "commerce"); !errors.Is(err, model.ErrUnavailable) {
				t.Fatal("untrusted TLS accepted", err)
			}
		})
	}
}
