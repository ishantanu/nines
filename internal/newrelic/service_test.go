package newrelic_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/newrelic"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func slo() model.SLO {
	return model.SLO{ID: "nr-availability", Name: "API availability", Service: "api", Project: "production", Target: .99, WindowSeconds: 120, StepSeconds: 60, QueryLanguage: "nrql", GoodQuery: "SELECT count(*) FROM NinesRequest WHERE service = 'api' AND success = true", TotalQuery: "SELECT count(*) FROM NinesRequest WHERE service = 'api'", Policy: &model.Policy{ShortWindowSeconds: 60, LongWindowSeconds: 120, BurnRate: 2, MinCoverage: 1}}
}
func bounds() (time.Time, time.Time) {
	from := time.Unix(1800000000, 0).UTC()
	return from, from.Add(120 * time.Second)
}

var window = regexp.MustCompile(` SINCE ([0-9]+) UNTIL ([0-9]+) TIMESERIES ([0-9]+) SECONDS$`)

func fixture(t *testing.T, change func(map[string]any, map[string]any) int) *newrelic.Client {
	t.Helper()
	c, err := newrelic.New("https://api.eu.newrelic.com/graphql", "123456", "synthetic-user-key")
	if err != nil {
		t.Fatal(err)
	}
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	http.DefaultTransport = transport(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.String() != "https://api.eu.newrelic.com/graphql" || r.Header.Get("API-Key") != "synthetic-user-key" {
			t.Fatal("wrong account route or credential")
		}
		var request struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Fatal("invalid request JSON")
		}
		if request.Variables["account"] != float64(123456) {
			t.Fatal("request did not pin account")
		}
		if strings.Contains(strings.ToLower(request.Query), "mutation") {
			t.Fatal("mutation attempted")
		}
		account := map[string]any{"id": 123456}
		if strings.Contains(request.Query, "query NinesCounts(") {
			for _, alias := range []string{"good", "total"} {
				q, ok := request.Variables[alias].(string)
				if !ok {
					t.Fatal("query missing")
				}
				m := window.FindStringSubmatch(q)
				if m == nil || !strings.Contains(q, " AS nines_count FROM ") {
					t.Fatal(q)
				}
				from, _ := strconv.ParseInt(m[1], 10, 64)
				to, _ := strconv.ParseInt(m[2], 10, 64)
				step, _ := strconv.ParseInt(m[3], 10, 64)
				if (to-from)/1000/step > 300 {
					t.Fatal("bucket bound exceeded")
				}
				rows := []any{}
				count := 100.
				if alias == "good" {
					count = 90.
				}
				for at := from / 1000; at < to/1000; at += step {
					rows = append(rows, map[string]any{"beginTimeSeconds": at, "endTimeSeconds": at + step, "nines_count": count})
				}
				account[alias] = map[string]any{"results": rows, "metadata": map[string]any{"messages": []any{}, "timeWindow": map[string]any{"begin": from, "end": to}}}
			}
		} else if !strings.Contains(request.Query, "query NinesConnection(") {
			t.Fatal("unexpected GraphQL document")
		}
		response := map[string]any{"data": map[string]any{"actor": map[string]any{"account": account}}}
		code := 200
		if change != nil {
			code = change(account, response)
		}
		body, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}, Request: r}, nil
	})
	return c
}

func TestNRQLServiceBurnEvaluation(t *testing.T) {
	from, to := bounds()
	c := fixture(t, nil)
	st := store.NewMemory()
	svc := service.New(st, c)
	svc.SourceKind = "newrelic"
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
	if err != nil || len(record.Points) != 2 {
		t.Fatal(record, err)
	}
	// Collection advances the normal deterministic burn-policy lifecycle.
	if record.Alert.Phase != "firing" {
		t.Fatal("count data did not fire the burn policy", record.Alert)
	}
	quality, err := svc.MeasurementQuality(context.Background(), slo().ID, 1, false)
	if err != nil || !strings.Contains(strings.Join(quality.Limitations, " "), "stored events") {
		t.Fatal("provider measurement limitations missing", quality, err)
	}
	status, err := svc.IntegrationStatus(context.Background(), slo().ID)
	if err != nil || status.Deployment.State != "unsupported" || status.Evaluation.State != "unsupported" {
		t.Fatal(status, err)
	}
}
func TestNRQLFailedCollectionPreservesExistingSamples(t *testing.T) {
	from, to := bounds()
	c := fixture(t, nil)
	st := store.NewMemory()
	svc := service.New(st, c)
	svc.SourceKind = "newrelic"
	svc.Now = func() time.Time { return to.Add(time.Minute) }
	if err := svc.Create(context.Background(), slo()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Collect(context.Background(), slo().ID, from, to); err != nil {
		t.Fatal(err)
	}
	failed := fixture(t, func(a, r map[string]any) int { r["errors"] = []any{"synthetic provider failure"}; return 200 })
	svc.Collector = failed
	if _, err := svc.Collect(context.Background(), slo().ID, from, to); err == nil {
		t.Fatal("partial query succeeded")
	}
	record, err := st.Read(context.Background(), slo().ID)
	if err != nil || len(record.Points) != 2 || record.Collection.State != "failed" {
		t.Fatal(record, err)
	}
}
