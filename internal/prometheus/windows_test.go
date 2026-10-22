package prometheus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func TestWindowCollectorPinsMimirTenantAndAllCutoffs(t *testing.T) {
	from := time.Unix(1800000000, 0).UTC()
	missing := false
	invalid := false
	seen := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Scope-OrgID") != "tenant-a" {
			t.Error("untrusted tenant")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		q := r.Form.Get("query")
		seen[q] = true
		at, _ := strconv.ParseInt(r.Form.Get("end"), 10, 64)
		value := "100"
		if strings.HasPrefix(q, "sum(increase(good[") {
			value = "82"
		}
		if strings.Contains(q, "count_over_time") {
			value = "1"
			if invalid {
				value = "1.1"
			}
		}
		result := []any{}
		if !missing || !strings.HasPrefix(q, "sum(increase(good[") {
			result = append(result, map[string]any{"values": [][2]any{{at, value}}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "matrix", "result": result}})
	}))
	defer server.Close()
	c, _ := NewMimir(server.URL, "tenant-a", "")
	s := model.SLO{StepSeconds: 10, WindowSeconds: 120, ProviderWindow: &v1.ProviderWindow{GoodSelector: "good", TotalSelector: "total", ScrapeIntervalSeconds: 2}, Policy: &model.Policy{ShortWindowSeconds: 20, LongWindowSeconds: 40}}
	obs, err := c.CollectWindows(context.Background(), s, from, from.Add(10*time.Second))
	if err != nil || len(obs) != 4 {
		t.Fatal(obs, err)
	}
	if !seen["sum(increase(good[120s]))"] || !seen["sum(increase(total[20s]))"] {
		t.Fatal("full windows missing", seen)
	}
	for _, o := range obs {
		if o.Good != 82 || o.Total != 100 || o.Coverage != 1 || o.Revision != 1 {
			t.Fatal(o)
		}
	}
	missing = true
	obs, err = c.CollectWindows(context.Background(), s, from, from.Add(10*time.Second))
	if err != nil || len(obs) != 0 {
		t.Fatal("missing numerator filled", obs, err)
	}
	missing = false
	invalid = true
	if _, err = c.CollectWindows(context.Background(), s, from, from.Add(10*time.Second)); err == nil {
		t.Fatal("invalid coverage accepted")
	}
}

func TestBacktestResolutionDoesNotChangeFullWindowSemantics(t *testing.T) {
	// A valid 60-second measurement grid can start off the epoch-aligned 300-second sampling grid.
	from := time.Unix(1800000060, 0).UTC()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		calls++
		if r.Header.Get("X-Scope-OrgID") != "tenant-a" || r.Form.Get("step") != "300" {
			t.Error("sampling or tenant changed", r.Form)
		}
		start, _ := strconv.ParseInt(r.Form.Get("start"), 10, 64)
		end, _ := strconv.ParseInt(r.Form.Get("end"), 10, 64)
		if start != from.Unix()+300 || end != from.Unix()+600 {
			t.Error("wrong historical range", r.Form)
		}
		value := "100"
		if strings.Contains(r.Form.Get("query"), "count_over_time") {
			value = "1"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "matrix", "result": []any{map[string]any{"values": [][2]any{{start, value}, {end, value}}}}}})
	}))
	defer server.Close()
	c, _ := NewMimir(server.URL, "tenant-a", "")
	s := model.SLO{StepSeconds: 60, WindowSeconds: 3600, ProviderWindow: &v1.ProviderWindow{GoodSelector: "good", TotalSelector: "total", ScrapeIntervalSeconds: 10}}
	out, err := c.CollectWindowRange(context.Background(), s, from, from.Add(600*time.Second), 300)
	if err != nil || len(out) != 4 || calls != 6 {
		t.Fatal(out, err, calls)
	}
	for _, o := range out {
		if o.WindowSeconds != 60 && o.WindowSeconds != 3600 {
			t.Fatal("sampling became measurement window", o)
		}
	}
}
