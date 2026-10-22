package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
)

func TestAPIToPrometheusToStateAndAlertLifecycle(t *testing.T) {
	at := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	good := "90"
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		start, _ := strconv.ParseInt(r.Form.Get("start"), 10, 64)
		end, _ := strconv.ParseInt(r.Form.Get("end"), 10, 64)
		value := "100"
		if r.Form.Get("query") == "good" {
			value = good
		}
		values := [][2]any{}
		for k := start; k <= end; k += 60 {
			values = append(values, [2]any{k, value})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "matrix", "result": []any{map[string]any{"values": values}}}})
	}))
	defer source.Close()
	collector, _ := prometheus.New(source.URL, "")
	st := store.NewMemory()
	svc := service.New(st, collector)
	svc.Now = func() time.Time { return at.Add(time.Minute) }
	h := New(svc, "secret", slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := func(method, path string, body any, want int) []byte {
		t.Helper()
		var b bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&b).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		r := httptest.NewRequest(method, path, &b)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer secret")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: status=%d body=%s", method, path, w.Code, w.Body)
		}
		return w.Body.Bytes()
	}
	c := model.SLO{ID: "checkout", Project: "production", Service: "payments", Name: "Checkout", Target: .99, WindowSeconds: 180, StepSeconds: 60, GoodQuery: "good", TotalQuery: "total", Policy: &model.Policy{ShortWindowSeconds: 60, LongWindowSeconds: 120, BurnRate: 2, ForSeconds: 0, MinCoverage: 1}}
	request("POST", "/api/slos", c, 201)
	request("POST", "/api/slos/checkout/collect", map[string]time.Time{"from": at.Add(-180 * time.Second), "to": at}, 200)
	var status model.Snapshot
	if err := json.Unmarshal(request("GET", "/api/slos/checkout/status", nil, 200), &status); err != nil {
		t.Fatal(err)
	}
	if status.Total != 300 || status.Good != 270 || status.Alert.Phase != "firing" || status.DataStatus != "complete" {
		t.Fatal(status)
	}
	request("GET", "/api/slos/checkout/status?at="+at.Add(-time.Minute).Format(time.RFC3339), nil, 200)
	events, _ := st.Deliveries(context.Background())
	if len(events) != 1 {
		t.Fatal("historical reads created events")
	}
	good = "100"
	request("POST", "/api/slos/checkout/collect", map[string]time.Time{"from": at.Add(-time.Minute), "to": at}, 200)
	events, _ = st.Deliveries(context.Background())
	if len(events) != 2 {
		t.Fatal("expected firing and resolution", events)
	}
	request("GET", "/api/events", nil, 200)
	request("GET", "/api/slos", nil, 200)
	request("GET", "/api/slos/checkout/samples", nil, 200)
	request("POST", "/api/slos", c, 409)
	request("GET", "/api/slos/missing/status", nil, 404)
}
func TestAuthAndStrictInput(t *testing.T) {
	svc := service.New(store.NewMemory(), nil)
	h := New(svc, "secret", slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, tc := range []struct {
		method, path, body, token, origin string
		want                              int
	}{
		{"GET", "/api/slos", "", "", "", 401},
		{"GET", "/healthz", "", "", "", 200},
		{"POST", "/api/slos", `{"id":"x","unexpected":true}`, "secret", "", 400},
		{"POST", "/api/slos", `{} {}`, "secret", "", 400},
		{"GET", "/api/slos", "", "secret", "https://other.example", 403},
	} {
		t.Run(fmt.Sprintf("%s-%d-%s", tc.path, tc.want, tc.body), func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			r.Header.Set("Content-Type", "application/json")
			if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d: %s", w.Code, w.Body)
			}
		})
	}
}
