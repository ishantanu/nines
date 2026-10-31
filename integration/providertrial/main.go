// Command providertrial is an opt-in, bounded synthetic hosted-provider fixture.
// It uses production adapters, resource APIs, stores and HTTP UI, without running
// background collection or notification dispatchers.
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/datadog"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/newrelic"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

type state struct {
	Run       string    `json:"run"`
	From      time.Time `json:"from"`
	BadTo     time.Time `json:"badTo"`
	To        time.Time `json:"to"`
	Providers []string  `json:"providers"`
}

func secret(name string) string {
	b, err := os.ReadFile("/run/secrets/" + name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
func stateDir() string {
	if value := os.Getenv("NINES_TRIAL_STATE"); value != "" {
		return value
	}
	return "/state"
}
func readState() (state, error) {
	var s state
	b, e := os.ReadFile(filepath.Join(stateDir(), "run.json"))
	if e != nil {
		return s, errors.New("run state unavailable; seed once first")
	}
	e = json.Unmarshal(b, &s)
	return s, e
}
func newState(now time.Time) state {
	end := now.UTC().Truncate(time.Minute).Add(-2 * time.Minute)
	return state{Run: model.NewID(), From: end.Add(-6 * time.Minute), BadTo: end.Add(-3 * time.Minute), To: end}
}
func datadogPayload(s state) any {
	series := []any{}
	for _, which := range []string{"good", "total"} {
		points := []any{}
		for i := 0; i < 6; i++ {
			value := 100
			if which == "good" && i < 3 {
				value = 90
			}
			points = append(points, map[string]any{"timestamp": s.From.Add(time.Duration(i)*time.Minute + 30*time.Second).Unix(), "value": value})
		}
		series = append(series, map[string]any{"metric": "nines.trial.requests." + which, "type": 1, "interval": 60, "points": points, "tags": []string{"service:nines-trial", "run:" + s.Run}})
	}
	return map[string]any{"series": series}
}
func newrelicPayload(s state) []any {
	events := make([]any, 0, 600)
	for i := 0; i < 6; i++ {
		for j := 0; j < 100; j++ {
			events = append(events, map[string]any{"eventType": "NinesTrialRequest", "timestamp": s.From.Add(time.Duration(i)*time.Minute + 30*time.Second).UnixMilli(), "service": "nines-trial", "run": s.Run, "success": i >= 3 || j < 90})
		}
	}
	return events
}
func submit(url, key string, payload any, compressed bool) error {
	raw, e := json.Marshal(payload)
	if e != nil {
		return e
	}
	if len(raw) > 150000 {
		return errors.New("synthetic payload exceeds limit")
	}
	var body bytes.Buffer
	if compressed {
		w := gzip.NewWriter(&body)
		if _, e = w.Write(raw); e != nil {
			return e
		}
		if e = w.Close(); e != nil {
			return e
		}
	} else {
		body.Write(raw)
	}
	req, e := http.NewRequestWithContext(context.Background(), http.MethodPost, url, &body)
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.Contains(url, "datadoghq") {
		req.Header.Set("DD-API-KEY", key)
	} else {
		req.Header.Set("Api-Key", key)
	}
	if compressed {
		req.Header.Set("Content-Encoding", "gzip")
	}
	client := http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, e := client.Do(req)
	if e != nil {
		return errors.New("ingestion transport failed; not retried")
	}
	defer func() { _ = response.Body.Close() }()
	b, e := io.ReadAll(io.LimitReader(response.Body, 65537))
	if e != nil || len(b) > 65536 {
		return errors.New("ingestion response invalid")
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("ingestion HTTP %d; not retried", response.StatusCode)
	}
	var result struct {
		Errors  []json.RawMessage `json:"errors"`
		Success *bool             `json:"success"`
	}
	if json.Unmarshal(b, &result) != nil || len(result.Errors) > 0 || (result.Success != nil && !*result.Success) {
		return errors.New("ingestion returned errors; not retried")
	}
	return nil
}
func slo(s state, provider string) v1.ServiceLevelObjective {
	good, total, language := "", "", ""
	if provider == "datadog" {
		language = "datadog"
		filter := "{service:nines-trial,run:" + s.Run + "}.as_count().rollup(sum,60)"
		good = "sum:nines.trial.requests.good" + filter
		total = "sum:nines.trial.requests.total" + filter
	} else {
		language = "nrql"
		total = "SELECT count(*) FROM NinesTrialRequest WHERE service = 'nines-trial' AND run = '" + s.Run + "'"
		good = total + " AND success = true"
	}
	return v1.NewSLO(provider+"-trial-availability", "provider-trial", v1.SLOSpec{DisplayName: provider + " synthetic availability", Service: "nines-trial", Source: provider, QueryLanguage: language, Target: .99, WindowSeconds: 180, StepSeconds: 60, GoodQuery: good, TotalQuery: total, Policies: []v1.BurnRatePolicy{{Name: "fast-burn", Severity: "critical", ShortWindowSeconds: 60, LongWindowSeconds: 180, BurnRate: 5, MinCoverage: 1}, {Name: "sustained-burn", Severity: "warning", ShortWindowSeconds: 60, LongWindowSeconds: 180, BurnRate: 2, MinCoverage: 1}}})
}
func journey(s state) v1.CriticalUserJourney {
	j := v1.CriticalUserJourney{APIVersion: v1.APIVersion, Kind: "CriticalUserJourney", Metadata: v1.Metadata{Name: "complete-synthetic-request", Project: "provider-trial"}}
	j.Spec.DisplayName = "Complete synthetic request"
	j.Spec.Description = "Synthetic provider adapter acceptance; linked SLOs are component evidence, not an end-to-end journey measurement."
	j.Spec.Owner = "trial"
	j.Spec.Criticality = "critical"
	for _, provider := range s.Providers {
		id := provider + "-trial-availability"
		j.Spec.SLOs = append(j.Spec.SLOs, v1.JourneySLO{ID: id, Dimension: "availability", Required: true})
		j.Spec.Steps = append(j.Spec.Steps, v1.JourneyStep{ID: provider, Name: "Observe through " + provider, Actor: "service", SLORefs: []string{id}})
	}
	return j
}
func seed() error {
	if _, err := os.Stat(filepath.Join(stateDir(), "seed.started")); err == nil {
		return errors.New("seed already attempted; use a new isolated Compose project, never resend automatically")
	}
	if secret("dd_api_key") == "" {
		return errors.New("datadog ingest credential missing")
	}
	if os.Getenv("NINES_TRIAL_NEWRELIC") == "1" && (secret("newrelic_ingest_key") == "" || secret("newrelic_ingest_key") == "not-configured") {
		return errors.New("set NEWRELIC_INGEST_KEY before enabling New Relic ingestion")
	}
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stateDir(), "seed.started"), []byte("single submission attempt; inspect logs before any new run\n"), 0o644); err != nil {
		return err
	}
	s := newState(time.Now())
	s.Providers = []string{"datadog"}
	if err := submit("https://api.uk1.datadoghq.com/api/v2/series", secret("dd_api_key"), datadogPayload(s), false); err != nil {
		return fmt.Errorf("datadog: %w", err)
	}
	fmt.Println("Datadog accepted: two COUNT series, twelve points, one submission")
	if os.Getenv("NINES_TRIAL_NEWRELIC") == "1" {
		account := os.Getenv("NEWRELIC_ACCOUNT_ID")
		if _, e := strconv.ParseUint(account, 10, 31); e != nil {
			return errors.New("invalid New Relic account")
		}
		if err := submit("https://insights-collector.eu01.nr-data.net/v1/accounts/"+account+"/events", secret("newrelic_ingest_key"), newrelicPayload(s), true); err != nil {
			return fmt.Errorf("new relic: %w", err)
		}
		s.Providers = append(s.Providers, "newrelic")
		fmt.Println("New Relic accepted: 600 small synthetic events, one compressed submission")
	}
	for _, provider := range s.Providers {
		b, e := yaml.Marshal(slo(s, provider))
		if e != nil {
			return e
		}
		if e = os.WriteFile(filepath.Join(stateDir(), provider+".slo.yaml"), b, 0o644); e != nil {
			return e
		}
	}
	b, e := yaml.Marshal(journey(s))
	if e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(stateDir(), "request.journey.yaml"), b, 0o644); e != nil {
		return e
	}
	b, e = json.Marshal(s)
	if e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(stateDir(), "run.json"), b, 0o644)
}

// seedNewRelic adds the optional provider once without resending Datadog.
func seedNewRelic() error {
	s, err := readState()
	if err != nil {
		return err
	}
	for _, provider := range s.Providers {
		if provider == "newrelic" {
			return errors.New("new relic already seeded; not resending")
		}
	}
	key := secret("newrelic_ingest_key")
	if key == "" || key == "not-configured" {
		return errors.New("set NEWRELIC_INGEST_KEY first")
	}
	account := os.Getenv("NEWRELIC_ACCOUNT_ID")
	if id, err := strconv.ParseUint(account, 10, 31); err != nil || id == 0 {
		return errors.New("invalid New Relic account")
	}
	f, err := os.OpenFile(filepath.Join(stateDir(), "newrelic.started"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return errors.New("new relic submission already attempted; not resending")
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = submit("https://insights-collector.eu01.nr-data.net/v1/accounts/"+account+"/events", key, newrelicPayload(s), true); err != nil {
		return err
	}
	s.Providers = append(s.Providers, "newrelic")
	for name, manifest := range map[string]any{"newrelic.slo.yaml": slo(s, "newrelic"), "request.journey.yaml": journey(s)} {
		b, err := yaml.Marshal(manifest)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(stateDir(), name), b, 0o644); err != nil {
			return err
		}
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(stateDir(), "run.json"), b, 0o644); err != nil {
		return err
	}
	fmt.Println("New Relic accepted: 600 small synthetic events, one compressed submission; Datadog not resent")
	return nil
}

type budgetTransport struct {
	base  http.RoundTripper
	mu    sync.Mutex
	calls map[string]int
}

func (t *budgetTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	limit := 0
	if r.URL.Scheme == "https" && r.Method == http.MethodGet && r.URL.Host == "api.uk1.datadoghq.com" {
		limit = 24
	}
	if r.URL.Scheme == "https" && r.Method == http.MethodPost && r.URL.Host == "api.eu.newrelic.com" {
		limit = 8
	}
	t.mu.Lock()
	if limit == 0 || t.calls[r.URL.Host] >= limit {
		t.mu.Unlock()
		return nil, errors.New("trial vendor request limit or route refused")
	}
	t.calls[r.URL.Host]++
	t.mu.Unlock()
	return t.base.RoundTrip(r)
}
func serve() error {
	s, e := readState()
	if e != nil {
		return e
	}
	token := secret("nines_token")
	if len(token) < 24 {
		return errors.New("trial access token must have at least 24 characters")
	}
	http.DefaultTransport = &budgetTransport{base: http.DefaultTransport, calls: map[string]int{}}
	svc := service.New(store.NewMemory(), nil)
	svc.TenantID = "provider-trial"
	svc.TenantName = "Synthetic provider trial (fixed acceptance clock)"
	svc.Sources = map[string]service.MetricSource{}
	for _, p := range s.Providers {
		switch p {
		case "datadog":
			c, e := datadog.New("https://api.uk1.datadoghq.com", secret("dd_api_key"), secret("dd_app_key"))
			if e != nil {
				return e
			}
			svc.Sources[p] = service.MetricSource{Kind: p, Collector: c}
		case "newrelic":
			c, e := newrelic.New("https://api.eu.newrelic.com/graphql", os.Getenv("NEWRELIC_ACCOUNT_ID"), secret("newrelic_user_key"))
			if e != nil {
				return e
			}
			svc.Sources[p] = service.MetricSource{Kind: p, Collector: c}
		}
	}
	var cutoff atomic.Int64
	cutoff.Store(s.BadTo.Add(time.Minute).Unix())
	svc.Now = func() time.Time { return time.Unix(cutoff.Load(), 0).UTC() }
	handler := httpapi.New(svc, token, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	mux := http.NewServeMux()
	mux.Handle("/", handler)
	mux.HandleFunc("POST /fixture/cutoff", func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			w.WriteHeader(401)
			return
		}
		var body struct {
			Phase string `json:"phase"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 128)).Decode(&body) != nil {
			w.WriteHeader(400)
			return
		}
		at := s.BadTo
		if body.Phase == "healthy" {
			at = s.To
		} else if body.Phase != "bad" {
			w.WriteHeader(400)
			return
		}
		cutoff.Store(at.Add(time.Minute).Unix())
		w.WriteHeader(204)
	})
	server := http.Server{Addr: "0.0.0.0:8080", Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 2 * time.Minute, IdleTimeout: 30 * time.Second}
	fmt.Println("Trial UI/API ready; no background collection; DD query cap 24, NR cap 8; memory-only resources")
	return server.ListenAndServe()
}

func localAPI(path string, body any, out any, manifest bool) error {
	var raw []byte
	var e error
	if body != nil {
		if manifest {
			raw, e = yaml.Marshal(body)
		} else {
			raw, e = json.Marshal(body)
		}
		if e != nil {
			return e
		}
	}
	origin := os.Getenv("NINES_TRIAL_URL")
	if origin == "" {
		origin = "http://nines:8080"
	}
	method := http.MethodGet
	if body != nil {
		method = http.MethodPost
	}
	req, e := http.NewRequestWithContext(context.Background(), method, origin+path, bytes.NewReader(raw))
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+secret("nines_token"))
	req.Header.Set("X-Nines-Tenant", "provider-trial")
	if manifest {
		req.Header.Set("Content-Type", "application/yaml")
		req.Header.Set("If-Match", "\"0\"")
	} else {
		req.Header.Set("Content-Type", "application/json")
	}
	client := http.Client{Timeout: 90 * time.Second}
	response, e := client.Do(req)
	if e != nil {
		return errors.New("local trial API unavailable")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("trial API %s returned HTTP %d", path, response.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(out)
	}
	return nil
}
func verify() error {
	s, e := readState()
	if e != nil {
		return e
	}
	for i := 0; i < 30; i++ {
		if localAPI("/api/workspace", nil, nil, false) == nil {
			break
		}
		if i == 29 {
			return errors.New("trial server did not become ready")
		}
		time.Sleep(time.Second)
	}
	for _, p := range s.Providers {
		if e = localAPI("/api/resources/apply", slo(s, p), nil, true); e != nil {
			return e
		}
	}
	if e = localAPI("/api/resources/apply", journey(s), nil, true); e != nil {
		return e
	}
	fmt.Println("Created Nines SLO definitions and linked critical user journey through production HTTP resource API")
	// Explicit bounded attempts tolerate hosted ingestion visibility without ever
	// resending telemetry or starting an unbounded collector.
	for _, phase := range []string{"bad", "healthy"} {
		if e = localAPI("/fixture/cutoff", map[string]string{"phase": phase}, nil, false); e != nil {
			return e
		}
		from, to, want := s.From, s.BadTo, .9
		if phase == "healthy" {
			from, to, want = s.BadTo, s.To, 1
		}
		for _, p := range s.Providers {
			passed := false
			for attempt := 0; attempt < 3; attempt++ {
				if attempt > 0 {
					fmt.Printf("%s %s: waiting 30s for hosted ingestion visibility (%d/3)\n", p, phase, attempt+1)
					time.Sleep(30 * time.Second)
				}
				e = localAPI("/api/slos/"+p+"-trial-availability/collect", map[string]any{"from": from, "to": to}, nil, false)
				if e != nil {
					fmt.Printf("%s %s: collection not yet verified\n", p, phase)
					continue
				}
				var view service.SLOView
				if e = localAPI("/api/slos/"+p+"-trial-availability", nil, &view, false); e != nil {
					return e
				}
				if view.Status.Reliability == nil || math.Abs(*view.Status.Reliability-want) > 1e-9 || view.Status.Coverage != 1 || math.Abs(view.Status.Total-300) > 1e-9 {
					fmt.Printf("%s %s: exact 3-bucket evidence not yet verified\n", p, phase)
					continue
				}
				// The status endpoint includes the current persisted alert state.
				var status map[string]json.RawMessage
				if e = localAPI("/api/slos/"+p+"-trial-availability/status", nil, &status, false); e != nil {
					return e
				}
				var alert model.AlertState
				if e = json.Unmarshal(status["alert"], &alert); e != nil {
					return errors.New("missing persisted alert evidence")
				}
				expected := "firing"
				if phase == "healthy" {
					expected = "inactive"
				}
				if alert.Phase != expected {
					return fmt.Errorf("%s %s alert transition not verified", p, phase)
				}
				fmt.Printf("%s %s PASS: good=%.0f total=%.0f SLI=%.1f%% coverage=100%% alert=%s\n", p, phase, view.Status.Good, view.Status.Total, *view.Status.Reliability*100, alert.Phase)
				passed = true
				break
			}
			if !passed {
				return fmt.Errorf("%s %s acceptance incomplete within bounded query budget", p, phase)
			}
		}
	}
	if e = localAPI("/api/journeys/complete-synthetic-request", nil, nil, false); e != nil {
		return e
	}
	fmt.Println("CUJ readback PASS; linked component evidence only, no end-to-end success claim. No vendor-native SLOs or alerts created.")
	return nil
}

// playgroundManifests reuses a completed run in a real-time playground. The
// wider window retains historical evidence; experimental lifecycle prevents
// synthetic trial traffic from paging a user's normal notification channels.
func playgroundManifests() error {
	s, err := readState()
	if err != nil {
		return err
	}
	output := os.Getenv("NINES_TRIAL_MANIFEST_DIR")
	if output == "" {
		return errors.New("set output manifest directory")
	}
	if err = os.MkdirAll(output, 0o700); err != nil {
		return err
	}
	for _, provider := range s.Providers {
		manifest := slo(s, provider)
		manifest.Spec.Source = provider + "-trial"
		manifest.Spec.WindowSeconds = 86400
		manifest.Spec.Lifecycle = "experimental"
		b, err := yaml.Marshal(manifest)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(output, provider+".slo.yaml"), b, 0o600); err != nil {
			return err
		}
	}
	b, err := yaml.Marshal(journey(s))
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(output, "request.journey.yaml"), b, 0o600); err != nil {
		return err
	}
	fmt.Println("Generated playground SLOs and CUJ for the existing run; 24-hour partial window, experimental lifecycle, no ingestion")
	return nil
}

// playgroundConfig preserves the selected existing tenant configuration and
// appends two manual-collection connections to one explicitly selected tenant.
func playgroundConfig() error {
	input := os.Getenv("NINES_TRIAL_BASE_TENANTS")
	output := os.Getenv("NINES_PROVIDER_TENANTS_FILE")
	tenant := os.Getenv("NINES_TRIAL_TENANT")
	if input == "" || output == "" || tenant == "" {
		return errors.New("set base tenant file, output file and target tenant explicitly")
	}
	account := os.Getenv("NEWRELIC_ACCOUNT_ID")
	if id, err := strconv.ParseUint(account, 10, 31); err != nil || id == 0 {
		return errors.New("invalid New Relic account")
	}
	b, err := os.ReadFile(input)
	if err != nil {
		return errors.New("base tenant file unavailable")
	}
	var cfg tenancy.Config
	decoder := yaml.NewDecoder(bytes.NewReader(b))
	decoder.KnownFields(true)
	if err = decoder.Decode(&cfg); err != nil {
		return errors.New("invalid base tenant configuration")
	}
	found := false
	for i := range cfg.Tenants {
		t := &cfg.Tenants[i]
		if t.ID != tenant {
			continue
		}
		found = true
		for _, c := range t.Connections {
			if c.Name == "datadog-trial" || c.Name == "newrelic-trial" {
				return errors.New("trial connections already exist; review instead of overwriting")
			}
		}
		if t.ManagedProviderRoutes == nil {
			t.ManagedProviderRoutes = map[string][]string{}
		}
		t.ManagedProviderRoutes["newrelic"] = append(t.ManagedProviderRoutes["newrelic"], account)
		if len(t.ManagedProviderRoutes["dash0"]) == 0 {
			t.ManagedProviderRoutes["dash0"] = []string{"default"}
		}
		var datadogRules *tenancy.RuleDestination
		if os.Getenv("NINES_DD_NATIVE_PUBLISH") == "1" {
			datadogRules = &tenancy.RuleDestination{Mode: "native", Namespace: "provider-trial"}
		}
		var newrelicRules *tenancy.RuleDestination
		if os.Getenv("NINES_NR_NATIVE_PUBLISH") == "1" {
			guid := os.Getenv("NINES_NR_ENTITY_GUID")
			if !newrelic.ValidEntityGUID(guid) {
				return errors.New("new relic native publication requires NINES_NR_ENTITY_GUID")
			}
			newrelicRules = &tenancy.RuleDestination{Mode: "native", Namespace: "provider-trial", EntityGUID: guid}
			t.ManagedProviderRoutes["newrelic-entity"] = append(t.ManagedProviderRoutes["newrelic-entity"], guid)
		}
		t.Connections = append(t.Connections,
			tenancy.Connection{Name: "datadog-trial", Source: tenancy.Source{Kind: "datadog", Rules: datadogRules, URL: "https://api.uk1.datadoghq.com", CollectionMode: "manual", CredentialsEnv: map[string]string{"apiKey": "DD_API_KEY", "applicationKey": "DD_APP_KEY"}}},
			tenancy.Connection{Name: "newrelic-trial", Source: tenancy.Source{Kind: "newrelic", Rules: newrelicRules, URL: "https://api.eu.newrelic.com/graphql", CollectionMode: "manual", Options: map[string]string{"accountID": account}, CredentialsEnv: map[string]string{"userKey": "NEWRELIC_API_KEY"}}})
	}
	if !found {
		return errors.New("target tenant does not exist in base configuration")
	}
	b, err = yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(output), 0o700); err != nil {
		return err
	}
	// This file contains environment references, never credential values. It must
	// be readable by the unprivileged server UID inside the read-only bind mount.
	if err = os.WriteFile(output, b, 0o644); err != nil {
		return err
	}
	fmt.Println("Generated trial connections; original tenant, Jev, identity and rule settings preserved; scheduled trial collection disabled")
	return nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "use seed, seed-newrelic, serve, verify, playground-config or playground-manifests")
		os.Exit(1)
	}
	var e error
	switch os.Args[1] {
	case "playground-manifests":
		e = playgroundManifests()
	case "playground-config":
		e = playgroundConfig()
	case "seed":
		e = seed()
	case "seed-newrelic":
		e = seedNewRelic()
	case "serve":
		e = serve()
	case "verify":
		e = verify()
	default:
		e = errors.New("unknown fixture mode")
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
