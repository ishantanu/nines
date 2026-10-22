package mimir_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/prometheus"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

// This opt-in test writes synthetic fixtures. It accepts only a loopback test
// endpoint so setting MIMIR_TEST_URL cannot accidentally target a remote source.
func TestRealMimirTenantCollection(t *testing.T) {
	base := strings.TrimRight(os.Getenv("MIMIR_TEST_URL"), "/")
	if base == "" {
		t.Skip("set MIMIR_TEST_URL to a disposable loopback Mimir origin")
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		t.Fatal("MIMIR_TEST_URL must be a plain loopback HTTP origin")
	}
	if ip := net.ParseIP(u.Hostname()); u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Fatal("refusing synthetic writes to a non-loopback endpoint")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	httpClient := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	waitReady(t, ctx, httpClient, base)
	// A real multi-tenant Mimir must reject a query without an org header.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/prometheus/api/v1/query?query=vector(1)", nil)
	response, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing tenant query: got HTTP %d, want 401", response.StatusCode)
	}
	end := time.Now().UTC().Truncate(time.Minute).Add(-time.Minute)
	from := end.Add(-3 * time.Minute)
	run := fmt.Sprintf("run-%d", time.Now().UnixNano())
	for _, fixture := range []struct {
		org  string
		good float64
	}{{"nines-integration-commerce", 99}, {"nines-integration-operations", 80}} {
		body := remoteWriteFixture(run, from, end, fixture.good, 100)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/push", bytes.NewReader(body))
		req.Header.Set("X-Scope-OrgID", fixture.org)
		req.Header.Set("Content-Encoding", "snappy")
		req.Header.Set("Content-Type", "application/x-protobuf")
		req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")
		req.Header.Set("User-Agent", "nines-mimir-integration/1")
		response, err := httpClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		_ = response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			t.Fatalf("remote write %s: HTTP %d %s", fixture.org, response.StatusCode, b)
		}
	}
	slo := model.SLO{ID: "same-slo", Name: "Same objective", Project: "integration", Service: "checkout", Target: 0.95, WindowSeconds: 180, StepSeconds: 60,
		GoodQuery:  fmt.Sprintf(`sum(nines_integration_good_events{run=%q})`, run),
		TotalQuery: fmt.Sprintf(`sum(nines_integration_total_events{run=%q})`, run)}
	for _, fixture := range []struct {
		org               string
		good, reliability float64
	}{{"nines-integration-commerce", 99, .99}, {"nines-integration-operations", 80, .80}} {
		t.Run(fixture.org, func(t *testing.T) {
			collector, err := prometheus.NewMimir(base+"/prometheus", fixture.org, "")
			if err != nil {
				t.Fatal(err)
			}
			platform := service.New(store.NewMemory(), collector)
			platform.Now = func() time.Time { return end.Add(time.Minute) }
			if err := platform.Create(ctx, slo); err != nil {
				t.Fatal(err)
			}
			var points []model.Point
			for {
				points, err = collector.Collect(ctx, slo, from, end)
				if err == nil && len(points) == 3 {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatalf("real query-range failed or stayed empty: %v (%d points)", err, len(points))
				case <-time.After(200 * time.Millisecond):
				}
			}
			for _, p := range points {
				if p.Good != fixture.good || p.Total != 100 {
					t.Fatalf("tenant %s received another tenant's counts: %+v", fixture.org, p)
				}
			}
			if _, err := platform.Collect(ctx, slo.ID, from, end); err != nil {
				t.Fatal(err)
			}
			status, err := platform.Status(ctx, slo.ID, end)
			if err != nil {
				t.Fatal(err)
			}
			if status.Reliability == nil || math.Abs(*status.Reliability-fixture.reliability) > 1e-12 || status.Coverage != 1 {
				t.Fatalf("wrong real-source evaluation: %+v", status)
			}
			t.Logf("org=%s same query returned 3 intervals: good=%.0f total=100 reliability=%.2f coverage=1", fixture.org, fixture.good, *status.Reliability)
		})
	}
	t.Run("empty-third-tenant", func(t *testing.T) {
		collector, err := prometheus.NewMimir(base+"/prometheus", "nines-integration-empty", "")
		if err != nil {
			t.Fatal(err)
		}
		points, err := collector.Collect(ctx, slo, from, end)
		if err != nil || len(points) != 0 {
			t.Fatalf("unwritten tenant saw data: %+v %v", points, err)
		}
	})
}

func waitReady(t *testing.T, ctx context.Context, c *http.Client, base string) {
	t.Helper()
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/ready", nil)
		response, err := c.Do(req)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == 200 {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("Mimir did not become ready")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// Only the published Remote-Write 1.0 sample/label schema is encoded here. This
// fixture generator is not a general sender and is never linked into the app.
// https://prometheus.io/docs/specs/prw/remote_write_spec/
func remoteWriteFixture(run string, from, to time.Time, good, total float64) []byte {
	var request []byte
	for _, metric := range []struct {
		name  string
		value float64
	}{{"nines_integration_good_events", good}, {"nines_integration_total_events", total}} {
		var series []byte
		for _, label := range []struct{ name, value string }{{"__name__", metric.name}, {"run", run}} {
			wire := field(nil, 1, []byte(label.name))
			wire = field(wire, 2, []byte(label.value))
			series = field(series, 1, wire)
		}
		for at := from.Add(time.Minute); !at.After(to); at = at.Add(time.Minute) {
			sample := []byte{0x09}
			sample = binary.LittleEndian.AppendUint64(sample, math.Float64bits(metric.value))
			sample = append(sample, 0x10)
			sample = binary.AppendUvarint(sample, uint64(at.UnixMilli()))
			series = field(series, 2, sample)
		}
		request = field(request, 1, series)
	}
	// Valid raw Snappy block containing one literal; no framed format or custom
	// compressor dependency. https://github.com/google/snappy/blob/main/format_description.txt
	encoded := binary.AppendUvarint(nil, uint64(len(request)))
	n := len(request) - 1
	if n < 60 {
		encoded = append(encoded, byte(n<<2))
	} else {
		lengthBytes := 0
		for v := n; v > 0; v >>= 8 {
			lengthBytes++
		}
		encoded = append(encoded, byte((59+lengthBytes)<<2))
		for i := 0; i < lengthBytes; i++ {
			encoded = append(encoded, byte(n>>(8*i)))
		}
	}
	return append(encoded, request...)
}

func field(dst []byte, number byte, payload []byte) []byte {
	dst = append(dst, number<<3|2)
	dst = binary.AppendUvarint(dst, uint64(len(payload)))
	return append(dst, payload...)
}
