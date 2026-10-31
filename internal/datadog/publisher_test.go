package datadog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/model"
)

const testNativeID = "0123456789abcdef0123456789abcdef"

func nativeFixture(t *testing.T) (*Client, *[]NativeResource, *int) {
	t.Helper()
	c, err := New("https://api.uk1.datadoghq.com", "synthetic-api", "synthetic-app")
	if err != nil {
		t.Fatal(err)
	}
	rows := []NativeResource{}
	writes := 0
	c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.uk1.datadoghq.com" || r.Header.Get("DD-API-KEY") != "synthetic-api" || r.Header.Get("DD-APPLICATION-KEY") != "synthetic-app" {
			t.Fatal("route/credential fence")
		}
		var data any
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/metrics/"):
			data = map[string]any{"type": "count"}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/slo":
			data = map[string]any{"data": rows}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/slo/"+testNativeID:
			data = map[string]any{"data": rows[0]}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/slo":
			var d NativeDocument
			if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
				t.Fatal(err)
			}
			row := NativeResource{NativeDocument: d, ID: testNativeID}
			rows = append(rows, row)
			writes++
			data = map[string]any{"data": []NativeResource{row}}
		default:
			t.Fatal("unexpected write or route", r.Method, r.URL.Path)
		}
		body, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}, Request: r}, nil
	})
	return c, &rows, &writes
}
func nativeSLO() model.SLO { s := slo(); s.WindowSeconds = 7 * 86400; return s }
func TestNativePublicationMappingAndReadback(t *testing.T) {
	c, _, writes := nativeFixture(t)
	ctx := context.Background()
	s := nativeSLO()
	p, err := c.Prepare(ctx, s, "team-a", "dd", "production")
	if err != nil {
		t.Fatal(err)
	}
	if p.Document.Thresholds[0].Target != 99 || p.Document.Thresholds[0].Timeframe != "7d" || p.Document.Query.Numerator != "sum:api.good{env:prod}.as_count()" || p.Matches {
		t.Fatal(p)
	}
	saved, state, err := c.Publish(ctx, p)
	if err != nil || state != "stored" || saved.ID != testNativeID || *writes != 1 {
		t.Fatal(saved, state, err)
	}
	if _, _, err = c.Publish(ctx, p); !errors.Is(err, model.ErrRevisionConflict) || *writes != 1 {
		t.Fatal("stale creation retried", err)
	}
	current, err := c.Prepare(ctx, s, "team-a", "dd", "production")
	if err != nil || !current.Matches || current.NativeID != testNativeID || !strings.HasPrefix(current.URL, "https://app.uk1.datadoghq.com/") {
		t.Fatal(current, err)
	}
	_, state, err = c.Publish(ctx, current)
	if err != nil || state != "unchanged" || *writes != 1 {
		t.Fatal(state, err)
	}
}
func TestNativeWindowAndQueryRejection(t *testing.T) {
	c, _, _ := nativeFixture(t)
	for _, window := range []int64{86400, 28 * 86400, 90 * 86400} {
		s := nativeSLO()
		s.WindowSeconds = window
		if _, err := c.Prepare(context.Background(), s, "team", "dd", "prod"); err == nil {
			t.Fatal("incompatible window", window)
		}
	}
	s := nativeSLO()
	s.WindowSeconds = 30 * 86400
	if _, err := c.Prepare(context.Background(), s, "team", "dd", "prod"); err != nil {
		t.Fatal(err)
	}
	s.GoodQuery = "avg:api.good{*}"
	if _, err := c.Prepare(context.Background(), s, "team", "dd", "prod"); err == nil {
		t.Fatal("unsafe query")
	}
}
func TestNativeOwnershipDriftDuplicatesAndRevisionIsolation(t *testing.T) {
	for _, kind := range []string{"drift", "duplicate", "foreign"} {
		t.Run(kind, func(t *testing.T) {
			c, rows, _ := nativeFixture(t)
			ctx := context.Background()
			p, err := c.Prepare(ctx, nativeSLO(), "team", "dd", "prod")
			if err != nil {
				t.Fatal(err)
			}
			row := NativeResource{NativeDocument: p.Document, ID: testNativeID}
			switch kind {
			case "drift":
				row.Query.Numerator = "sum:other{*}.as_count()"
			case "duplicate":
				*rows = append(*rows, NativeResource{NativeDocument: p.Document, ID: "abcdefabcdefabcdefabcdefabcdefab"})
			case "foreign":
				row.Tags = []string{"nines_owner:foreign"}
			}
			*rows = append(*rows, row)
			if _, err = c.Prepare(ctx, nativeSLO(), "team", "dd", "prod"); err == nil {
				t.Fatal("conflict accepted")
			}
		})
	}
	c, _, _ := nativeFixture(t)
	s := nativeSLO()
	a, err := c.Prepare(context.Background(), s, "team", "dd", "prod")
	if err != nil {
		t.Fatal(err)
	}
	s.Revision = 2
	b, err := c.Prepare(context.Background(), s, "team", "dd", "prod")
	if err != nil || a.Origin == b.Origin || a.Owner != b.Owner {
		t.Fatal("revision ownership changed", err)
	}
}
func TestNativeAmbiguousPostNeverRetriedAndRedacts(t *testing.T) {
	c, _, _ := nativeFixture(t)
	p, err := c.Prepare(context.Background(), nativeSLO(), "team", "dd", "prod")
	if err != nil {
		t.Fatal(err)
	}
	base := c.http.Transport
	posts := 0
	c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost {
			posts++
			return nil, fmt.Errorf("synthetic-api private response")
		}
		return base.RoundTrip(r)
	})
	_, state, err := c.Publish(context.Background(), p)
	if err == nil || state != "unverified" || posts != 1 || strings.Contains(err.Error(), "synthetic-api") || strings.Contains(err.Error(), "private response") {
		t.Fatal(state, err, posts)
	}
}

func TestNativePublicationRejectsGaugeMetadata(t *testing.T) {
	c, _, writes := nativeFixture(t)
	base := c.http.Transport
	c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/metrics/") {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"type":"gauge"}`)), Header: http.Header{}, Request: r}, nil
		}
		return base.RoundTrip(r)
	})
	if _, err := c.Prepare(context.Background(), nativeSLO(), "team", "dd", "prod"); err == nil || *writes != 0 {
		t.Fatal("gauge published", err)
	}
}

func TestNativeLostCreateResponseRecoversWithoutSecondWrite(t *testing.T) {
	c, _, writes := nativeFixture(t)
	ctx := context.Background()
	s := nativeSLO()
	p, err := c.Prepare(ctx, s, "team", "dd", "prod")
	if err != nil {
		t.Fatal(err)
	}
	base := c.http.Transport
	c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		res, err := base.RoundTrip(r)
		if r.Method == http.MethodPost {
			if res != nil {
				_ = res.Body.Close()
			}
			return nil, fmt.Errorf("response lost after commit")
		}
		return res, err
	})
	if _, _, err = c.Publish(ctx, p); err == nil || *writes != 1 {
		t.Fatal("ambiguous create mishandled", err)
	}
	c.http.Transport = base
	current, err := c.Prepare(ctx, s, "team", "dd", "prod")
	if err != nil || !current.Matches {
		t.Fatal("lost response mapping not recovered", err)
	}
	_, state, err := c.Publish(ctx, current)
	if err != nil || state != "unchanged" || *writes != 1 {
		t.Fatal("duplicate create after recovery", state, err)
	}
}
