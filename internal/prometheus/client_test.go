package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

func TestCollectContractAndChunkBoundaries(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "POST" || r.URL.Path != "/prefix/api/v1/query_range" || r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Errorf("incorrect request %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		start, _ := strconv.ParseInt(r.Form.Get("start"), 10, 64)
		end, _ := strconv.ParseInt(r.Form.Get("end"), 10, 64)
		step, _ := strconv.ParseInt(r.Form.Get("step"), 10, 64)
		values := [][2]any{}
		value := "100"
		if r.Form.Get("query") == "good" {
			value = "99"
		}
		for k := start; k <= end; k += step {
			values = append(values, [2]any{k, value})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "matrix", "result": []any{map[string]any{"metric": map[string]string{}, "values": values}}}})
	}))
	defer server.Close()
	c, _ := New(server.URL+"/prefix", "test-secret")
	from := time.Unix(1800000000, 0)
	to := from.Add(10001 * time.Minute)
	points, err := c.Collect(context.Background(), model.SLO{GoodQuery: "good", TotalQuery: "total", StepSeconds: 60}, from, to)
	if err != nil || len(points) != 10001 || calls.Load() != 4 {
		t.Fatalf("points=%d calls=%d err=%v", len(points), calls.Load(), err)
	}
	seen := map[int64]bool{}
	for _, p := range points {
		if !p.Time.After(from) || p.Time.After(to) || seen[p.Time.Unix()] || p.Good != 99 || p.Total != 100 {
			t.Fatal(p)
		}
		seen[p.Time.Unix()] = true
	}
}
func TestRejectPartialAmbiguousAndMalformedData(t *testing.T) {
	for name, body := range map[string]string{
		"partial backend": `{"status":"success","isPartial":true,"data":{"resultType":"matrix","result":[]}}`,
		"warnings":        `{"status":"success","warnings":["partial"],"data":{"resultType":"matrix","result":[]}}`,
		"multiple":        `{"status":"success","data":{"resultType":"matrix","result":[{"values":[]},{"values":[]}]}}`,
		"nan":             `{"status":"success","data":{"resultType":"matrix","result":[{"values":[[1800000060,"NaN"]]}]}}`,
		"fractional time": `{"status":"success","data":{"resultType":"matrix","result":[{"values":[[1800000060.5,"1"]]}]}}`,
		"duplicate":       `{"status":"success","data":{"resultType":"matrix","result":[{"values":[[1800000060,"1"],[1800000060,"1"]]}]}}`,
		"error envelope":  `{"status":"error","error":"failed"}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, body) }))
			defer server.Close()
			c, _ := New(server.URL, "")
			_, err := c.Collect(context.Background(), model.SLO{GoodQuery: "g", TotalQuery: "t", StepSeconds: 60}, time.Unix(1800000000, 0), time.Unix(1800000060, 0))
			if err == nil {
				t.Fatal("accepted unsafe source response")
			}
		})
	}
}
func TestUnmatchedQueriesDoNotBecomeZeroes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		if r.Form.Get("query") == "good" {
			_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"matrix","result":[]}}`)
		} else {
			_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"matrix","result":[{"values":[[1800000060,"10"]]}]}}`)
		}
	}))
	defer server.Close()
	c, _ := New(server.URL, "")
	_, err := c.Collect(context.Background(), model.SLO{GoodQuery: "good", TotalQuery: "total", StepSeconds: 60}, time.Unix(1800000000, 0), time.Unix(1800000060, 0))
	if err == nil || !strings.Contains(err.Error(), "unmatched") {
		t.Fatal(err)
	}
}
