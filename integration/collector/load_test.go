package collector_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/prometheus"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

type workload struct {
	Name            string `json:"name"`
	Tenants         int    `json:"tenants"`
	SLOsPerTenant   int    `json:"slosPerTenant"`
	StepSeconds     int64  `json:"stepSeconds"`
	WindowSeconds   int64  `json:"windowSeconds"`
	SourceLatencyMS int    `json:"sourceLatencyMS"`
	ThrottleEvery   int64  `json:"throttleEvery"`
	MissingEvery    int64  `json:"missingEvery"`
	LagIntervals    int64  `json:"lagIntervals"`
}
type result struct {
	Workload            workload       `json:"workload"`
	DurationSeconds     float64        `json:"durationSeconds"`
	Jobs                int64          `json:"jobs"`
	UniqueJobs          int            `json:"uniqueJobs"`
	FailedJobs          int64          `json:"failedJobs"`
	Requests            int64          `json:"requests"`
	Throttled           int64          `json:"throttled"`
	MaxQueries          int64          `json:"maxConcurrentQueries"`
	MaxQueryIntervals   int64          `json:"maxQueryIntervals"`
	TenantJobs          map[string]int `json:"tenantJobs"`
	JobP95MS            float64        `json:"jobP95MS"`
	LagP95MS            float64        `json:"lagP95MS"`
	APIP95MS            float64        `json:"apiP95MS"`
	APIMaxMS            float64        `json:"apiMaxMS"`
	APIRequests         int            `json:"apiRequests"`
	APIErrors           int            `json:"apiErrors"`
	DatabaseBytesBefore int64          `json:"databaseBytesBefore"`
	DatabaseBytesAfter  int64          `json:"databaseBytesAfter"`
	SampleRows          int64          `json:"sampleRows"`
	HeapPeakBytes       uint64         `json:"heapPeakBytes"`
	ProcessMaxRSSBytes  uint64         `json:"processMaxRSSBytes"`
	ProcessCPUSeconds   float64        `json:"processCPUSeconds"`
	MinimumCoverage     float64        `json:"minimumCoverage"`
}

type syntheticSource struct {
	work                                               workload
	requests, throttled, active, maximum, maxIntervals atomic.Int64
}

func maximum(counter *atomic.Int64, n int64) {
	for before := counter.Load(); n > before; before = counter.Load() {
		if counter.CompareAndSwap(before, n) {
			return
		}
	}
}
func (s *syntheticSource) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	active := s.active.Add(1)
	defer s.active.Add(-1)
	maximum(&s.maximum, active)
	request := s.requests.Add(1)
	select {
	case <-r.Context().Done():
		return
	case <-time.After(time.Duration(s.work.SourceLatencyMS) * time.Millisecond):
	}
	if s.work.ThrottleEvery > 0 && request%s.work.ThrottleEvery == 0 {
		s.throttled.Add(1)
		w.WriteHeader(429)
		return
	}
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(400)
		return
	}
	start, e1 := strconv.ParseInt(r.Form.Get("start"), 10, 64)
	end, e2 := strconv.ParseInt(r.Form.Get("end"), 10, 64)
	step, e3 := strconv.ParseInt(r.Form.Get("step"), 10, 64)
	if e1 != nil || e2 != nil || e3 != nil || step < 1 {
		w.WriteHeader(400)
		return
	}
	maximum(&s.maxIntervals, (end-start)/step+1)
	value := "100"
	if r.Form.Get("query") == "good" {
		value = "99"
	}
	points := [][]any{}
	for at := start; at <= end; at += step {
		if s.work.MissingEvery > 0 && (at/step)%s.work.MissingEvery == 0 {
			continue
		}
		points = append(points, []any{at, value})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "matrix", "result": []any{map[string]any{"metric": map[string]string{}, "values": points}}}})
}
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	return values[int(float64(len(values)-1)*p)]
}
func cpuUsage() syscall.Rusage {
	var usage syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &usage)
	return usage
}
func cpuSeconds(u syscall.Rusage) float64 {
	return float64(u.Utime.Sec+u.Stime.Sec) + float64(u.Utime.Usec+u.Stime.Usec)/1e6
}

func TestRealCollectorScalingWorkload(t *testing.T) {
	if os.Getenv("NINES_COLLECTOR_ACCEPTANCE") != "1" {
		t.Skip("run make test-collector for disposable workload acceptance")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("disposable database required")
	}
	profiles := []workload{
		{Name: "baseline", Tenants: 2, SLOsPerTenant: 10, StepSeconds: 60, WindowSeconds: 3600, LagIntervals: 4},
		{Name: "long-window-throttled", Tenants: 8, SLOsPerTenant: 25, StepSeconds: 300, WindowSeconds: 2419200, SourceLatencyMS: 10, ThrottleEvery: 10, MissingEvery: 5, LagIntervals: 370},
		{Name: "slow-source", Tenants: 4, SLOsPerTenant: 5, StepSeconds: 15, WindowSeconds: 86400, SourceLatencyMS: 150, LagIntervals: 4},
	}
	results := []result{}
	for _, profile := range profiles {
		t.Run(profile.Name, func(t *testing.T) { results = append(results, runWorkload(t, dsn, profile)) })
	}
	commit, _ := exec.Command("git", "rev-parse", "HEAD").Output()
	status, _ := exec.Command("git", "status", "--porcelain").Output()
	tree := "clean"
	if len(status) > 0 {
		tree = "modified"
	}
	hashes := map[string]string{}
	for _, file := range []string{"integration/collector/load_test.go", "internal/store/collection_jobs.go", "internal/store/schema.sql", "internal/store/postgres.go", "internal/store/memory.go", "internal/service/collection_scheduler.go", "internal/service/workers.go", "internal/service/service.go", "cmd/server/main.go", "cmd/server/tenants.go"} {
		data, err := os.ReadFile("../../" + file)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		hashes[file] = hex.EncodeToString(digest[:])
	}
	report := struct {
		APIVersion        string            `json:"apiVersion"`
		Kind              string            `json:"kind"`
		GeneratedAt       time.Time         `json:"generatedAt"`
		OS                string            `json:"os"`
		Architecture      string            `json:"architecture"`
		LogicalCPUs       int               `json:"logicalCPUs"`
		CollectorReplicas int               `json:"collectorReplicas"`
		WorkersPerReplica int               `json:"workersPerReplica"`
		Database          string            `json:"database"`
		RaceInstrumented  bool              `json:"raceInstrumented"`
		SourceCommit      string            `json:"sourceCommit"`
		SourceTree        string            `json:"sourceTree"`
		SourceFilesSHA256 map[string]string `json:"sourceFilesSHA256"`
		Results           []result          `json:"results"`
	}{"nines.dev/v1alpha1", "CollectorAcceptanceReport", time.Now().UTC(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), 2, 4, "PostgreSQL 17; loopback; synthetic provider and HTTP API", false, strings.TrimSpace(string(commit)), tree, hashes, results}

	if output := os.Getenv("NINES_COLLECTOR_REPORT"); output != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(output, append(data, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
func runWorkload(t *testing.T, dsn string, profile workload) result {
	t.Helper()
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	source := &syntheticSource{work: profile}
	provider := httptest.NewServer(source)
	defer provider.Close()
	client, err := prometheus.New(provider.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	services := []*service.Service{}
	peers := []*service.Service{}
	pools := []*pgxpool.Pool{}
	now := time.Now().UTC().Truncate(time.Duration(profile.StepSeconds) * time.Second)
	for tenant := 0; tenant < profile.Tenants; tenant++ {
		schema := "collector_" + strings.ReplaceAll(model.NewID(), "-", "")
		if _, err = admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, e := admin.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); e != nil {
				t.Error(e)
			}
		})
		parsed, _ := url.Parse(dsn)
		q := parsed.Query()
		q.Set("search_path", schema)
		q.Set("pool_max_conns", "2")
		parsed.RawQuery = q.Encode()
		st, e := store.NewPostgres(ctx, parsed.String())
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(st.Close)
		peer, e := store.NewPostgres(ctx, parsed.String())
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(peer.Close)
		pool, e := pgxpool.New(ctx, parsed.String())
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(pool.Close)
		pools = append(pools, pool)
		s := service.New(st, client)
		s.TenantID = fmt.Sprintf("tenant-%02d", tenant)
		s.Now = time.Now
		other := service.New(peer, client)
		other.TenantID = s.TenantID
		other.Now = s.Now
		services = append(services, s)
		peers = append(peers, other)
		for slo := 0; slo < profile.SLOsPerTenant; slo++ {
			c := model.SLO{ID: fmt.Sprintf("slo-%03d", slo), Project: "synthetic", Service: "api", Name: "Synthetic availability", Source: "primary", Target: .99, WindowSeconds: profile.WindowSeconds, StepSeconds: profile.StepSeconds, GoodQuery: "good", TotalQuery: "total"}
			if e = s.Create(ctx, c); e != nil {
				t.Fatal(e)
			}
			seed := min(int64(600), 2*profile.WindowSeconds/profile.StepSeconds-profile.LagIntervals-2)
			if e = st.Update(ctx, c.ID, func(r *model.Record) error {
				for i := int64(0); i < seed; i++ {
					at := now.Add(-time.Duration(i+profile.LagIntervals+2) * time.Duration(profile.StepSeconds) * time.Second)
					r.Points[at.Unix()] = model.Point{Time: at, Good: 99, Total: 100}
				}
				r.Collection.SLORevision = 1
				r.Collection.SourceDigest = s.SourceFingerprint("primary")
				r.Collection.SourceTo = now.Add(-time.Duration(profile.LagIntervals+2) * time.Duration(profile.StepSeconds) * time.Second)
				return nil
			}); e != nil {
				t.Fatal(e)
			}
		}
	}
	out := result{Workload: profile, TenantJobs: map[string]int{}, MinimumCoverage: 1}
	databaseSize := func() int64 {
		var total int64
		for _, p := range pools {
			var size int64
			if e := p.QueryRow(ctx, `SELECT COALESCE(sum(pg_total_relation_size(c.oid)),0)::bigint FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=current_schema() AND c.relkind='r'`).Scan(&size); e != nil {
				t.Fatal(e)
			}
			total += size
		}
		return total
	}
	out.DatabaseBytesBefore = databaseSize()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	api := httptest.NewServer(httpapi.New(services[0], "", logger))
	defer api.Close()
	var lock sync.Mutex
	durations, lags := []float64{}, []float64{}
	unique := map[string]bool{}
	callback := func(r service.CollectionResult) {
		lock.Lock()
		defer lock.Unlock()
		out.Jobs++
		if r.Failed {
			out.FailedJobs++
		}
		out.TenantJobs[r.Tenant]++
		unique[r.Tenant+"/"+r.ID] = true
		durations = append(durations, float64(r.Duration)/float64(time.Millisecond))
		lags = append(lags, float64(r.StartedAt.Sub(r.DueAt))/float64(time.Millisecond))
	}
	first, e := service.NewCollectionScheduler(services, 4)
	if e != nil {
		t.Fatal(e)
	}
	second, e := service.NewCollectionScheduler(peers, 4)
	if e != nil {
		t.Fatal(e)
	}
	first.OnResult = callback
	second.OnResult = callback
	run, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var group sync.WaitGroup
	startCPU := cpuUsage()
	started := time.Now()
	group.Go(func() { first.Run(run, logger) })
	group.Go(func() { second.Run(run, logger) })
	apiDurations := []float64{}
	httpClient := &http.Client{Timeout: 2 * time.Second}
	for run.Err() == nil {
		request, err := http.NewRequestWithContext(run, http.MethodGet, api.URL+"/api/slos/slo-000/status", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-Nines-Tenant", services[0].TenantID)
		start := time.Now()
		response, e := httpClient.Do(request)
		if e == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			out.APIRequests++
			if response.StatusCode != 200 {
				out.APIErrors++
			}
			apiDurations = append(apiDurations, float64(time.Since(start))/float64(time.Millisecond))
		} else if run.Err() == nil {
			out.APIErrors++
		}
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		out.HeapPeakBytes = max(out.HeapPeakBytes, stats.HeapAlloc)
		select {
		case <-run.Done():
		case <-time.After(25 * time.Millisecond):
		}
	}
	cancel()
	group.Wait()
	out.DurationSeconds = time.Since(started).Seconds()
	out.UniqueJobs = len(unique)
	endCPU := cpuUsage()
	out.ProcessCPUSeconds = cpuSeconds(endCPU) - cpuSeconds(startCPU)
	rss := uint64(endCPU.Maxrss)
	if runtime.GOOS == "linux" {
		rss *= 1024
	}
	out.ProcessMaxRSSBytes = rss
	out.Requests = source.requests.Load()
	out.Throttled = source.throttled.Load()
	out.MaxQueries = source.maximum.Load()
	out.MaxQueryIntervals = source.maxIntervals.Load()
	out.JobP95MS = percentile(durations, .95)
	out.LagP95MS = percentile(lags, .95)
	out.APIP95MS = percentile(apiDurations, .95)
	out.APIMaxMS = percentile(apiDurations, 1)
	out.DatabaseBytesAfter = databaseSize()
	for index, s := range services {
		if out.TenantJobs[s.TenantID] == 0 {
			t.Error("tenant made no progress", s.TenantID)
		}
		var rows int64
		if e := pools[index].QueryRow(ctx, "SELECT count(*) FROM samples").Scan(&rows); e != nil {
			t.Fatal(e)
		}
		out.SampleRows += rows
		configs, e := s.Store.List(ctx)
		if e != nil {
			t.Fatal(e)
		}
		for _, c := range configs {
			snapshot, e := s.Status(ctx, c.ID, time.Time{})
			if e != nil {
				t.Fatal(e)
			}
			out.MinimumCoverage = min(out.MinimumCoverage, snapshot.Coverage)
		}
	}
	if out.MaxQueries > 8 || out.MaxQueryIntervals > 300 || out.APIRequests == 0 || out.APIErrors > 0 {
		t.Errorf("workload bounds/API failed: %+v", out)
	}
	if profile.MissingEvery > 0 && out.MinimumCoverage >= 1 {
		t.Error("missing observations became full coverage")
	}
	return out
}
