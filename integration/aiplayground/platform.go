package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
)

const token = "nines-local-playground-owner-token"

type settings struct {
	nines, sample, prometheus, mimir string
	tenants                          []string
}

func fixtureURL(raw, host, path string) error {
	u, e := url.Parse(raw)
	if e != nil {
		return errors.New("invalid fixture endpoint")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "http" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != path || (u.Hostname() != host && u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback())) {
		return errors.New("use only a disposable loopback endpoint or the documented Compose service")
	}
	return nil
}
func (s settings) validate() error {
	for _, v := range []struct{ url, host, path string }{{s.nines, "localhost", ""}, {s.sample, "ai-sample", ""}, {s.prometheus, "prometheus", ""}, {s.mimir, "mimir", "/prometheus"}} {
		if e := fixtureURL(v.url, v.host, v.path); e != nil {
			return e
		}
	}
	return nil
}

var fixtureHTTP = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func fixtureRequest(ctx context.Context, origin, path, method, tenant string, out any) error {
	req, e := http.NewRequestWithContext(ctx, method, origin+path, nil)
	if e != nil {
		return e
	}
	if tenant == "mimir" {
		req.Header.Set("X-Scope-OrgID", "playground")
	}
	res, e := fixtureHTTP.Do(req)
	if e != nil {
		return errors.New("fixture request unavailable")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != 200 {
		return fmt.Errorf("fixture request returned HTTP %d", res.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
}
func waitFor(ctx context.Context, out io.Writer, label string, limit time.Duration, check func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	var last error
	for {
		if e := check(ctx); e == nil {
			_, e = fmt.Fprintln(out, label+": passed")
			return e
		} else {
			last = e
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: timed out or cancelled (%v)", label, last)
		case <-time.After(3 * time.Second):
		}
	}
}
func (s settings) workspace(tenant string) (*client.Client, error) {
	return client.New(client.Config{URL: s.nines, Tenant: tenant, Token: token, HTTPClient: &http.Client{Timeout: 12 * time.Second}})
}
func (s *settings) setup(ctx context.Context, out io.Writer) error {
	if len(s.tenants) == 0 {
		if err := waitFor(ctx, out, "Nines playground workspaces", 150*time.Second, func(ctx context.Context) error {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.nines+"/api/tenants", nil)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("X-Nines-Tenant", "prometheus")
			res, err := fixtureHTTP.Do(req)
			if err != nil {
				return errors.New("nines unavailable")
			}
			defer func() { _ = res.Body.Close() }()
			if res.StatusCode != 200 {
				return fmt.Errorf("tenant discovery returned HTTP %d", res.StatusCode)
			}
			var tenants []struct {
				ID string `json:"id"`
			}
			if err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&tenants); err != nil {
				return err
			}
			found := map[string]bool{}
			for _, tenant := range tenants {
				if tenant.ID != "prometheus" && tenant.ID != "mimir" || found[tenant.ID] {
					return errors.New("not a synthetic playground tenant set")
				}
				found[tenant.ID] = true
			}
			if !found["prometheus"] || len(found) > 2 {
				return errors.New("playground Prometheus workspace missing")
			}
			s.tenants = []string{"prometheus"}
			if found["mimir"] {
				s.tenants = append(s.tenants, "mimir")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	for _, tenant := range s.tenants {
		c, e := s.workspace(tenant)
		if e != nil {
			return e
		}
		for _, d := range definitions() {
			if e = waitFor(ctx, out, tenant+" "+d.SLO.Metadata.Name+" definition", 150*time.Second, func(ctx context.Context) error {
				_, e := c.SLOContext(ctx, d.SLO.Metadata.Name, v1.ContextOptions{})
				if e == nil {
					return nil
				}
				var apiError *client.APIError
				if !errors.As(e, &apiError) || apiError.StatusCode != 404 {
					return e
				}
				data, e := yaml.Marshal(d.SLO)
				if e != nil {
					return e
				}
				compiled, e := c.CompileMeasurement(ctx, data)
				if e != nil {
					return e
				}
				_, e = c.CreateSLO(ctx, compiled.Manifest)
				if errors.As(e, &apiError) && apiError.Conflict() {
					return nil
				}
				return e
			}); e != nil {
				return e
			}
		}
		j := journey()
		_, e = c.JourneyContext(ctx, j.Metadata.Name, v1.ContextOptions{Limit: 10})
		if e != nil {
			var apiError *client.APIError
			if !errors.As(e, &apiError) || apiError.StatusCode != 404 {
				return e
			}
			_, e = c.Apply(ctx, j, 0)
			if e != nil && (!errors.As(e, &apiError) || !apiError.Conflict()) {
				return e
			}
		}
	}
	_, e := fmt.Fprintln(out, "AI definitions ready. Existing definitions were preserved. Automatic rule reconciliation and collection run independently.")
	return e
}
func (s settings) scenario(ctx context.Context, name string) error {
	return fixtureRequest(ctx, s.sample, "/scenario?"+url.Values{"name": {name}}.Encode(), http.MethodPost, "", nil)
}
func (s settings) ratios(ctx context.Context, tenant string, revisions map[string]int64) (map[string]float64, error) {
	origin := s.prometheus
	if tenant == "mimir" {
		origin = s.mimir
	}
	expression := fmt.Sprintf(`nines:sli:ratio{nines_tenant=%q,nines_slo=~"ai-.*",window="60s"}`, tenant)
	var response struct {
		Status   string   `json:"status"`
		Warnings []string `json:"warnings"`
		Data     struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []json.RawMessage `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	e := fixtureRequest(ctx, origin, "/api/v1/query?"+url.Values{"query": {expression}}.Encode(), http.MethodGet, tenant, &response)
	if e != nil {
		return nil, e
	}
	if response.Status != "success" || response.Data.ResultType != "vector" || len(response.Warnings) != 0 {
		return nil, errors.New("provider query did not return an unambiguous vector")
	}
	out := map[string]float64{}
	for _, r := range response.Data.Result {
		id := r.Metric["nines_slo"]
		expected, exists := revisions[id]
		if !exists || r.Metric["nines_revision"] != strconv.FormatInt(expected, 10) {
			continue
		}
		if r.Metric["nines_tenant"] != tenant || r.Metric["nines_source"] != "primary" || r.Metric["window"] != "60s" {
			return nil, errors.New("provider sample identity changed")
		}
		if len(r.Value) != 2 {
			return nil, errors.New("invalid provider sample")
		}
		var text string
		if json.Unmarshal(r.Value[1], &text) != nil {
			return nil, errors.New("invalid provider value")
		}
		value, e := strconv.ParseFloat(text, 64)
		if e != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
			return nil, errors.New("invalid provider ratio")
		}
		if _, exists := out[id]; exists {
			return nil, errors.New("multiple revisions returned; use a fresh AI playground for acceptance")
		}
		out[id] = value
	}
	return out, nil
}
func (s *settings) verify(ctx context.Context, out io.Writer) (err error) {
	if e := s.setup(ctx, out); e != nil {
		return e
	}
	// Restore the prior named scenario on every normal/error/cancellation exit.
	var previous struct {
		Scenario string `json:"scenario"`
	}
	if e := fixtureRequest(ctx, s.sample, "/scenario", http.MethodGet, "", &previous); e != nil {
		return e
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		restore := s.scenario(cleanup, previous.Scenario)
		if restore != nil {
			err = errors.Join(err, errors.New("could not restore prior AI scenario; reset it manually"))
		}
	}()
	if e := s.scenario(ctx, "healthy"); e != nil {
		return e
	}
	revisions := map[string]map[string]int64{}
	for _, tenant := range s.tenants {
		c, e := s.workspace(tenant)
		if e != nil {
			return e
		}
		revisions[tenant] = map[string]int64{}
		if e = waitFor(ctx, out, tenant+" AI collection, automatic rule activation and current output", 4*time.Minute, func(ctx context.Context) error {
			for _, d := range definitions() {
				state, e := c.Integration(ctx, d.SLO.Metadata.Name)
				if e != nil {
					return e
				}
				if state.Automation == nil || !state.Automation.Enabled || state.Automation.State != "configured" || state.Collection.State != "collected" || state.Evaluation.State != "evaluating" || state.RuleOutput == nil || state.RuleOutput.State != "verified" {
					return fmt.Errorf("%s not ready (collection=%s, evaluation=%s, output=%v)", d.SLO.Metadata.Name, state.Collection.State, state.Evaluation.State, state.RuleOutput)
				}
				revisions[tenant][d.SLO.Metadata.Name] = state.Revision
			}
			return nil
		}); e != nil {
			return e
		}
	}
	for _, phase := range []string{"healthy", "degraded", "healthy"} {
		if e := s.scenario(ctx, phase); e != nil {
			return e
		}
		if e := waitFor(ctx, out, "AI "+phase+" recorded ratios and Nines SLI values", 4*time.Minute, func(ctx context.Context) error {
			for _, tenant := range s.tenants {
				ratios, e := s.ratios(ctx, tenant, revisions[tenant])
				if e != nil {
					return e
				}
				c, e := s.workspace(tenant)
				if e != nil {
					return e
				}
				for _, d := range definitions() {
					expected := 1.0
					if phase == "degraded" {
						expected = d.Degraded
					}
					observed, exists := ratios[d.SLO.Metadata.Name]
					if !exists || math.Abs(observed-expected) > .025 {
						return fmt.Errorf("%s/%s recorded ratio not at expected phase", tenant, d.SLO.Metadata.Name)
					}
					measured, e := c.SLOContext(ctx, d.SLO.Metadata.Name, v1.ContextOptions{ExpectedRevision: revisions[tenant][d.SLO.Metadata.Name]})
					if e != nil {
						return e
					}
					m := measured.Measurement
					if m.Reliability == nil || m.Total <= 0 || math.Abs(*m.Reliability-expected) > .025 || measured.Freshness.State != "fresh" || measured.Definition.Lifecycle != v1.LifecycleExperimental {
						return fmt.Errorf("%s/%s Nines SLI not at expected phase", tenant, d.SLO.Metadata.Name)
					}
				}
			}
			return nil
		}); e != nil {
			return e
		}
	}
	for _, tenant := range s.tenants {
		c, e := s.workspace(tenant)
		if e != nil {
			return e
		}
		j, e := c.JourneyContext(ctx, "ai-support-resolution", v1.ContextOptions{Limit: 10})
		if e != nil || len(j.Links) != len(definitions()) {
			return errors.New("journey links not available")
		}
	}
	_, err = fmt.Fprintln(out, `{"result":"passed","checks":["template rendering","telemetry verification","automatic rules","query dataset output","Nines SLI","unknown verdict denominator","evaluation coverage","task versus tool outcomes","conditional TTFT","degradation and recovery","journey links"]}`)
	return err
}
