package prometheus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

func TestMimirQueriesPinOrgAndAuthentication(t *testing.T) {
	at := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	seen := map[string]int{}
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prometheus/api/v1/query_range" {
			t.Error(r.URL.Path)
		}
		org := r.Header.Get("X-Scope-OrgID")
		mu.Lock()
		seen[org]++
		mu.Unlock()
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		value := "100"
		if r.Form.Get("query") == "good" {
			if org == "org-alpha" {
				value = "99"
			} else {
				value = "90"
			}
		}
		if org == "org-alpha" && r.Header.Get("Authorization") != "Bearer alpha-secret" {
			t.Error("wrong bearer authentication")
		}
		if org == "org-beta" {
			u, p, ok := r.BasicAuth()
			if !ok || u != "beta-user" || p != "beta-secret" {
				t.Error("wrong basic authentication")
			}
		}
		start, _ := strconv.ParseInt(r.Form.Get("start"), 10, 64)
		end, _ := strconv.ParseInt(r.Form.Get("end"), 10, 64)
		values := [][2]any{}
		for ts := start; ts <= end; ts += 60 {
			values = append(values, [2]any{ts, value})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "matrix", "result": []any{map[string]any{"values": values}}}})
	}))
	defer source.Close()
	a, err := NewMimir(source.URL+"/prometheus", "org-alpha", "alpha-secret")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewMimir(source.URL+"/prometheus", "org-beta", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = b.SetBasicAuth("beta-user", "beta-secret"); err != nil {
		t.Fatal(err)
	}
	s := model.SLO{StepSeconds: 60, GoodQuery: "good", TotalQuery: "total"}
	var wg sync.WaitGroup
	for _, tc := range []struct {
		client *Client
		want   float64
	}{{a, 99}, {b, 90}} {
		wg.Go(func() {
			points, err := tc.client.Collect(context.Background(), s, at.Add(-time.Minute), at)
			if err != nil || len(points) != 1 || points[0].Good != tc.want {
				t.Error(points, err)
			}
		})
	}
	wg.Wait()
	if seen["org-alpha"] != 2 || seen["org-beta"] != 2 {
		t.Fatal(seen)
	}
	for _, org := range []string{"", "alpha|beta", "bad\nheader", ".", ".."} {
		if _, err := NewMimir(source.URL, org, ""); err == nil {
			t.Fatal("invalid upstream org accepted", org)
		}
	}
}
