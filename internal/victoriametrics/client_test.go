package victoriametrics

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

func slo() model.SLO {
	return model.SLO{ID: "vm-api", Name: "API availability", Project: "production", Service: "api", Target: .99, WindowSeconds: 120, StepSeconds: 60, QueryLanguage: "metricsql", GoodQuery: `sum(increase_pure(requests_total{code="200"}[1m]))`, TotalQuery: `sum(increase_pure(requests_total[1m]))`, Policy: &model.Policy{ShortWindowSeconds: 60, LongWindowSeconds: 120, BurnRate: 2, MinCoverage: 1}}
}
func bounds() (time.Time, time.Time) {
	from := time.Unix(1800000000, 0).UTC()
	return from, from.Add(2 * time.Minute)
}
func fixture(t *testing.T, cluster bool, change func(*http.Request, map[string]any) int) *Client {
	t.Helper()
	account, project, path := "", "", "/api/v1/query_range"
	if cluster {
		account, project, path = "42", "7", "/select/42:7/prometheus/api/v1/query_range"
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != path || r.Header.Get("Authorization") != "Bearer synthetic-vm-token" || r.Header.Get("X-Scope-OrgID") != "" || r.Header.Get("AccountID") != "" {
			t.Error("route or authentication changed", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		if r.Form.Get("deny_partial_response") != "1" {
			t.Error("partial response denial missing")
		}
		start, _ := strconv.ParseInt(r.Form.Get("start"), 10, 64)
		end, _ := strconv.ParseInt(r.Form.Get("end"), 10, 64)
		step, _ := strconv.ParseInt(r.Form.Get("step"), 10, 64)
		value := "100"
		if strings.Contains(r.Form.Get("query"), `code="200"`) {
			value = "90"
		}
		if r.Form.Get("query") == "vector(1)" {
			value = "1"
		}
		values := []any{}
		for at := start; at <= end; at += step {
			values = append(values, []any{at, value})
		}
		body := map[string]any{"status": "success", "isPartial": false, "data": map[string]any{"resultType": "matrix", "result": []any{map[string]any{"metric": map[string]string{}, "values": values}}}}
		code := 200
		if change != nil {
			code = change(r, body)
		}
		w.WriteHeader(code)
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	c, err := New(server.URL, account, project, "synthetic-vm-token", "", "")
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestSingleAndClusterCollection(t *testing.T) {
	for _, cluster := range []bool{false, true} {
		c := fixture(t, cluster, nil)
		from, to := bounds()
		points, err := c.Collect(context.Background(), slo(), from, to)
		if err != nil || len(points) != 2 || points[0].Good != 90 || points[0].Total != 100 || !points[1].Time.Equal(to) {
			t.Fatal(points, err)
		}
		if err = c.Probe(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}
func TestVMServiceBurnLifecycle(t *testing.T) {
	c := fixture(t, true, nil)
	from, to := bounds()
	st := store.NewMemory()
	svc := service.New(st, c)
	svc.SourceKind = "victoriametrics"
	svc.Now = func() time.Time { return to.Add(time.Minute) }
	d := model.DefinitionFromSLO(slo())
	def := model.Definition{SLO: &d}
	if _, err := svc.PlanDefinition(context.Background(), def); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApplyDefinition(context.Background(), def, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Collect(context.Background(), slo().ID, from, to); err != nil {
		t.Fatal(err)
	}
	record, err := st.Read(context.Background(), slo().ID)
	if err != nil || record.Alert.Phase != "firing" || len(record.Points) != 2 {
		t.Fatal(record, err)
	}
	integration, err := svc.IntegrationStatus(context.Background(), slo().ID)
	if err != nil || integration.Deployment.State != "unsupported" {
		t.Fatal(integration, err)
	}
	q, err := svc.MeasurementQuality(context.Background(), slo().ID, 1, false)
	if err != nil || !strings.Contains(strings.Join(q.Limitations, " "), "MetricsQL") {
		t.Fatal(q, err)
	}
}
func TestVMRejectsPartialAndAmbiguousEvidence(t *testing.T) {
	for name, change := range map[string]func(*http.Request, map[string]any) int{
		"partial": func(_ *http.Request, b map[string]any) int { b["isPartial"] = true; return 200 },
		"warning": func(_ *http.Request, b map[string]any) int {
			b["warnings"] = []string{"synthetic-vm-token private"}
			return 200
		},
		"error": func(_ *http.Request, b map[string]any) int {
			b["error"] = "synthetic-vm-token private query"
			return 500
		},
		"missing numerator": func(r *http.Request, b map[string]any) int {
			if strings.Contains(r.Form.Get("query"), `code="200"`) {
				b["data"].(map[string]any)["result"] = []any{}
			}
			return 200
		},
		"multiple series": func(_ *http.Request, b map[string]any) int {
			d := b["data"].(map[string]any)
			d["result"] = append(d["result"].([]any), d["result"].([]any)[0])
			return 200
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := fixture(t, true, change)
			from, to := bounds()
			_, err := c.Collect(context.Background(), slo(), from, to)
			if err == nil || strings.Contains(err.Error(), "synthetic-vm-token") || strings.Contains(err.Error(), "private") {
				t.Fatal("unsafe evidence or leaked error", err)
			}
		})
	}
}
func TestVMPinsTenantAndCredentialIdentity(t *testing.T) {
	for _, ids := range [][2]string{{"multitenant", ""}, {"42:7", ""}, {"01", ""}, {"", "7"}, {"42", "7/../../other"}, {"4294967296", ""}} {
		if _, err := New("https://vm.example", ids[0], ids[1], "", "", ""); err == nil {
			t.Fatal(ids)
		}
	}
	for _, url := range []string{"https://vm.example/select/multitenant/prometheus", "https://vm.example?AccountID=multitenant", "https://user@vm.example", "https://vm.example/%2fselect"} {
		if _, err := New(url, "42", "", "", "", ""); err == nil {
			t.Fatal(url)
		}
	}
	a, err := New("https://vm.example", "42", "7", "secret", "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := New("https://vm.example", "42", "8", "secret", "", "")
	if err != nil {
		t.Fatal(err)
	}
	d, err := New("https://vm.example", "42", "7", "rotated", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if a.SourceFingerprint() == b.SourceFingerprint() || a.SourceFingerprint() == d.SourceFingerprint() {
		t.Fatal("tenant or credential identity lost")
	}
	if _, err = New("https://vm.example", "42", "", "bearer", "user", "password"); err == nil {
		t.Fatal("conflicting auth accepted")
	}
}
func TestVMBasicAuthAndPromQL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "reader" || password != "synthetic-password" {
			t.Error("wrong basic authentication")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		end, _ := strconv.ParseInt(r.Form.Get("end"), 10, 64)
		_, _ = io.WriteString(w, `{"status":"success","data":{"resultType":"matrix","result":[{"values":[[`+strconv.FormatInt(end, 10)+`,"1"]]}]}}`)
	}))
	defer server.Close()
	c, err := New(server.URL, "", "", "", "reader", "synthetic-password")
	if err != nil {
		t.Fatal(err)
	}
	from, to := bounds()
	s := slo()
	s.QueryLanguage = "promql"
	s.GoodQuery = "vector(1)"
	s.TotalQuery = "vector(1)"
	if _, err = c.Collect(context.Background(), s, from, to); err != nil {
		t.Fatal(err)
	}
}
