package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/prometheus"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestRecentCollectionReadsPinnedSourceAndPreservesDefinition(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 23, 12, 0, 32, 0, time.UTC)
	calls := 0
	mode := "data"
	good := `sum(increase(requests_total{code="200"}[1m]))`
	total := `sum(increase(requests_total[1m]))`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Scope-OrgID") != "trusted-org" || r.Header.Get("Authorization") != "Bearer server-source-secret" {
			t.Error("lost pinned credentials")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		start, _ := strconv.ParseInt(r.Form.Get("start"), 10, 64)
		end, _ := strconv.ParseInt(r.Form.Get("end"), 10, 64)
		expectedEnd := now.Add(-time.Minute).Truncate(time.Minute)
		if end != expectedEnd.Unix() || start != expectedEnd.Add(-14*time.Minute).Unix() || r.Form.Get("step") != "60" {
			t.Error("unbounded or unaligned range", r.Form)
		}
		query := r.Form.Get("query")
		if query != good && query != total {
			t.Error("query did not come from saved definition")
		}
		if mode == "failed" {
			http.Error(w, "opaque upstream credential", 500)
			return
		}
		value := "100"
		if query == good {
			value = "99"
		}
		values := [][2]any{}
		if mode != "empty" {
			for at := start; at <= end; at += 60 {
				values = append(values, [2]any{at, value})
			}
		}
		result := []any{}
		if len(values) > 0 {
			result = append(result, map[string]any{"values": values})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "matrix", "result": result}})
	}))
	defer upstream.Close()
	source, err := prometheus.NewMimir(upstream.URL, "trusted-org", "server-source-secret")
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(store.NewMemory(), nil)
	svc.Now = func() time.Time { return now }
	svc.Sources = map[string]service.MetricSource{"requests": {Kind: "mimir", Collector: source}}
	slo := model.SLO{ID: "availability", Name: "API availability", Project: "production", Service: "api", Source: "requests", Target: .999, WindowSeconds: 86400, StepSeconds: 60, GoodQuery: good, TotalQuery: total}
	if err := svc.Create(ctx, slo); err != nil {
		t.Fatal(err)
	}
	before, _ := svc.Store.Read(ctx, slo.ID)
	handler, err := NewMulti([]Workspace{{ID: "alpha", Name: "Alpha", Service: svc, Grants: []tenancy.Grant{{Token: "editor", Role: "editor"}, {Token: "viewer", Role: "viewer"}, {Token: "agent", Role: "agent"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	request := func(token, tenant, body string, want int) []byte {
		t.Helper()
		req := httptest.NewRequest("POST", "/api/slos/availability/collect", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Nines-Tenant", tenant)
		req.Header.Set("X-Scope-OrgID", "forged")
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		if out.Code != want {
			t.Fatalf("%s: %d %s", body, out.Code, out.Body)
		}
		return out.Body.Bytes()
	}
	request("viewer", "alpha", `{"recent":true}`, 403)
	request("agent", "alpha", `{"recent":true}`, 403)
	request("editor", "other", `{"recent":true}`, 403)
	request("editor", "alpha", `{"recent":true,"from":"2026-10-23T10:00:00Z"}`, 400)
	request("editor", "alpha", `{"recent":true,"url":"http://evil.example"}`, 400)
	if calls != 0 {
		t.Fatal("denied requests reached source")
	}
	out := request("editor", "alpha", `{"recent":true}`, 200)
	var count struct {
		Collected int `json:"collected"`
	}
	if json.Unmarshal(out, &count) != nil || count.Collected != 15 {
		t.Fatal(string(out))
	}
	after, _ := svc.Store.Read(ctx, slo.ID)
	if model.DefinitionDigest(before.SLO) != model.DefinitionDigest(after.SLO) || len(after.Points) != 15 {
		t.Fatal("collection changed definition or lost measurements")
	}
	view, err := svc.ReadSLO(ctx, slo.ID)
	if err != nil || view.Status.Reliability == nil || *view.Status.Reliability != .99 {
		t.Fatal("source values did not reach SLI", view, err)
	}
	mode = "empty"
	request("editor", "alpha", `{"recent":true}`, 200)
	after, _ = svc.Store.Read(ctx, slo.ID)
	if after.Collection.Points != 0 {
		t.Fatal("empty query invented data")
	}
	mode = "failed"
	out = request("editor", "alpha", `{"recent":true}`, 502)
	if bytes.Contains(out, []byte("opaque upstream credential")) || bytes.Contains(out, []byte("server-source-secret")) {
		t.Fatal("upstream error leaked")
	}
}
