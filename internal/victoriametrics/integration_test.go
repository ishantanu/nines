package victoriametrics

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

// TestRealVictoriaMetrics writes only to the disposable loopback fixture started
// by make test-victoriametrics. Never configure this against an existing server.
func TestRealVictoriaMetrics(t *testing.T) {
	base := os.Getenv("VICTORIAMETRICS_TEST_URL")
	if base == "" {
		t.Skip("use make test-victoriametrics for a disposable backend")
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		t.Fatal("requires disposable loopback fixture")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		t.Fatal("refusing non-loopback writes")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	httpClient := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request := func(path, body string) error {
		req, e := http.NewRequestWithContext(ctx, http.MethodPost, base+path, strings.NewReader(body))
		if e != nil {
			return e
		}
		req.Header.Set("Content-Type", "text/plain")
		resp, e := httpClient.Do(req)
		if e != nil {
			return e
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("fixture returned HTTP %d", resp.StatusCode)
		}
		return nil
	}
	for {
		if err = request("/health", ""); err == nil {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("fixture not ready", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	from := time.Now().UTC().Truncate(time.Minute).Add(-10 * time.Minute)
	to := from.Add(2 * time.Minute)
	metric := "nines_vm_accept_" + strings.ReplaceAll(model.NewID(), "-", "") + "_total"
	var body strings.Builder
	good, bad := 0.0, 0.0
	resetGood, resetBad := 0.0, 0.0
	for at := from.Add(-5 * time.Minute); !at.After(to.Add(2 * time.Minute)); at = at.Add(15 * time.Second) {
		if at.Equal(from.Add(30 * time.Second)) {
			resetGood, resetBad = 0, 0
		}
		resetGood += 22.5
		resetBad += 2.5
		fmt.Fprintf(&body, "%s_reset{code=\"200\"} %g %d\n%s_reset{code=\"500\"} %g %d\n", metric, resetGood, at.UnixMilli(), metric, resetBad, at.UnixMilli())
		if !at.After(to) {
			good += 22.5
			bad += 2.5
		} else {
			good += 25
		}
		fmt.Fprintf(&body, "%s_zero{code=\"200\"} 0 %d\n%s_zero{code=\"500\"} 0 %d\n", metric, at.UnixMilli(), metric, at.UnixMilli())
		fmt.Fprintf(&body, "%s{code=\"200\"} %g %d\n%s{code=\"500\"} %g %d\n", metric, good, at.UnixMilli(), metric, bad, at.UnixMilli())
	}
	if err = request("/api/v1/import/prometheus", body.String()); err != nil {
		t.Fatal(err)
	}
	if err = request("/internal/force_flush", ""); err != nil {
		t.Fatal(err)
	}
	c, err := New(base, "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Probe(ctx); err != nil {
		t.Fatal(err)
	}
	s := slo()
	s.GoodQuery = fmt.Sprintf(`sum(increase_pure(%s{code="200"}[1m]))`, metric)
	s.TotalQuery = fmt.Sprintf(`sum(increase_pure(%s[1m]))`, metric)
	st := store.NewMemory()
	svc := service.New(st, c)
	svc.SourceKind = "victoriametrics"
	svc.Now = func() time.Time { return to.Add(time.Minute) }
	d := model.DefinitionFromSLO(s)
	if _, err = svc.ApplyDefinition(ctx, model.Definition{SLO: &d}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Collect(ctx, s.ID, from, to); err != nil {
		t.Fatal(err)
	}
	record, err := st.Read(ctx, s.ID)
	if err != nil || len(record.Points) != 2 || record.Alert.Phase != "firing" {
		t.Fatal("burn alert must fire", record, err)
	}
	for _, p := range record.Points {
		if math.Abs(p.Good-90) > 1e-6 || math.Abs(p.Total-100) > 1e-6 {
			t.Fatalf("unexpected interval counts: %+v", p)
		}
	}
	snapshot, err := svc.Status(ctx, s.ID, to)
	if err != nil || snapshot.Reliability == nil || math.Abs(*snapshot.Reliability-.9) > 1e-9 || snapshot.Coverage != 1 {
		t.Fatal("SLI must be 90% with full coverage", snapshot, err)
	}
	t.Log("live backend: 90/100 per interval, SLI 90%, coverage 100%, burn alert firing")
	for _, function := range []string{"increase", "increase_prometheus"} {
		alternate := s
		alternate.GoodQuery = strings.ReplaceAll(s.GoodQuery, "increase_pure", function)
		alternate.TotalQuery = strings.ReplaceAll(s.TotalQuery, "increase_pure", function)
		if function == "increase" {
			alternate.QueryLanguage = "promql"
		}
		points, collectErr := c.Collect(ctx, alternate, from, to)
		if collectErr != nil || len(points) != 2 {
			t.Fatal(function, points, collectErr)
		}
		expectedGood, expectedTotal := 90.0, 100.0
		if function == "increase_prometheus" {
			expectedGood, expectedTotal = 67.5, 75
		}
		for _, point := range points {
			if math.Abs(point.Good-expectedGood) > 1e-6 || math.Abs(point.Total-expectedTotal) > 1e-6 {
				t.Fatal(function, point)
			}
		}
	}
	reset := s
	reset.GoodQuery = strings.ReplaceAll(s.GoodQuery, metric, metric+"_reset")
	reset.TotalQuery = strings.ReplaceAll(s.TotalQuery, metric, metric+"_reset")
	resetPoints, err := c.Collect(ctx, reset, from, to)
	if err != nil || len(resetPoints) != 2 {
		t.Fatal("counter reset", resetPoints, err)
	}
	for _, point := range resetPoints {
		if math.Abs(point.Good-90) > 1e-6 || math.Abs(point.Total-100) > 1e-6 {
			t.Fatal("counter reset lost counts", point)
		}
	}
	t.Log("live backend: increase_pure preserves interval counts across a counter reset")
	zero := s
	zero.GoodQuery = strings.ReplaceAll(s.GoodQuery, metric, metric+"_zero")
	zero.TotalQuery = strings.ReplaceAll(s.TotalQuery, metric, metric+"_zero")
	zeroPoints, err := c.Collect(ctx, zero, from, to)
	if err != nil || len(zeroPoints) != 2 {
		t.Fatal("zero traffic", zeroPoints, err)
	}
	for _, point := range zeroPoints {
		if point.Good != 0 || point.Total != 0 {
			t.Fatal("invented traffic", point)
		}
	}
	t.Log("live backend: increase uses the preceding boundary sample (90/100); increase_prometheus uses in-window samples (67.5/75); zero traffic stays zero")

	svc.Now = func() time.Time { return to.Add(3 * time.Minute) }
	if _, err = svc.Collect(ctx, s.ID, to, to.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	record, err = st.Read(ctx, s.ID)
	if err != nil || record.Alert.Phase != "inactive" {
		t.Fatal("burn alert must recover", record, err)
	}
	t.Log("live backend: healthy intervals recover the burn alert")
	integration, err := svc.IntegrationStatus(ctx, s.ID)
	if err != nil || integration.Deployment.State != "unsupported" {
		t.Fatal("must not claim external rules deployed", integration, err)
	}
	missing := s
	missing.GoodQuery = strings.ReplaceAll(s.GoodQuery, metric, metric+"_missing")
	if _, err = c.Collect(ctx, missing, from, to); err == nil {
		t.Fatal("missing numerator must fail rather than imply zero")
	}
	missing.TotalQuery = strings.ReplaceAll(s.TotalQuery, metric, metric+"_missing")
	points, err := c.Collect(ctx, missing, from, to)
	if err != nil || len(points) != 0 {
		t.Fatal("absent telemetry must stay absent", points, err)
	}
	t.Log("live backend: missing numerator rejected; absent telemetry remains absent; rule deployment unsupported")
}
