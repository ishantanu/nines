// A loopback-only synthetic browser fixture, never used by cmd/server.
package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/intentflow"
	rm "github.com/ishantanu/nines/internal/reviewmodel"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

type fixtureModel struct{}

func (fixtureModel) Descriptor() rm.Descriptor {
	return rm.Descriptor{Provider: "synthetic-fixture", Model: "scripted-intent-fixture", Protocol: "fixture", RouteDigest: "synthetic-local-fixture", Local: true}
}
func (fixtureModel) Turn(ctx context.Context, in rm.Request, progress func(rm.Progress) error) (rm.Response, error) {
	if e := ctx.Err(); e != nil {
		return rm.Response{}, e
	}
	// Keep synthetic transport progress visible long enough for browser polling.
	if progress != nil {
		if err := progress(rm.Progress{Bytes: 128}); err != nil {
			return rm.Response{}, err
		}
		timer := time.NewTimer(1500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return rm.Response{}, ctx.Err()
		case <-timer.C:
		}
		if err := progress(rm.Progress{Bytes: 256, Final: true}); err != nil {
			return rm.Response{}, err
		}
	}
	name := "ask_user"
	args := any(map[string]string{"question": "Confirm which observation point, eligible attempts and success labels match the user outcome."})
	last := in.Messages[len(in.Messages)-1]
	has := func(name string) bool {
		for _, t := range in.Tools {
			if t.Name == name {
				return true
			}
		}
		return false
	}
	switch {
	case has("draft_goal"):
		name = "draft_goal"
		args = map[string]any{}
	case has("find_measurements") && last.Name != "find_measurements":
		name = "find_measurements"
		args = map[string]any{}
	case has("compile_intent") && last.Name != "compile_intent" && last.Name != "backtest_intent":
		name = "compile_intent"
		args = map[string]any{}
	case last.Name == "compile_intent" && strings.Contains(in.Messages[0].Text, `"backtest":{`):
		name = "backtest_intent"
		args = map[string]any{}
	case has("finish_proposal"):
		name = "finish_proposal"
		args = map[string]string{"summary": "Synthetic verification: review the generated attempt-success SLO. Nothing has been applied."}
	}
	b, _ := json.Marshal(args)
	return rm.Response{Native: json.RawMessage(`{"role":"assistant","content":"Synthetic verification fixture"}`), Calls: []rm.Call{{ID: "fixture-call", Name: name, Arguments: b}}, OutputTokens: 50}, nil
}
func (fixtureModel) ModelID() string { return "synthetic-drafting-fixture" }
func (fixtureModel) Draft(_ context.Context, text string) (v1.ModelGoal, error) {
	out := v1.ModelGoal{Outcome: "availability", Clarifications: []string{}}
	if strings.Contains(text, "99.9%") && strings.Contains(text, "1 hour") {
		target, window := "99.9%", "1 hour"
		out.TargetText = &target
		out.WindowText = &window
	} else {
		out.Clarifications = []string{"State the target percentage and rolling duration. This fixture recognizes 99.9% over 1 hour."}
	}
	return out, nil
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	workspaces := []httpapi.Workspace{}
	services := []*intentflow.Service{}
	for _, id := range []string{"default", "other"} {
		s, _, e := testtelemetry.New()
		if e != nil {
			panic(e)
		}
		s.TenantID = id
		s.TenantName = "Intent workflow verification (synthetic)"
		s.Demo = true
		s.IntentDrafter = fixtureModel{}
		flow := intentflow.New(s, intentflow.NewMemory())
		flow.Model = fixtureModel{}
		services = append(services, flow)
		workspaces = append(workspaces, httpapi.Workspace{ID: id, Name: s.TenantName, Service: s, IntentConversations: flow, Anonymous: true})
	}
	h, e := httpapi.NewMulti(workspaces, log)
	if e != nil {
		panic(e)
	}
	var wg sync.WaitGroup
	for _, s := range services {
		wg.Go(func() { s.RunWorker(ctx, log) })
	}
	server := http.Server{Addr: "127.0.0.1:8208", Handler: h, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if e := server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			log.Error("fixture failed")
			cancel()
		}
	}()
	<-ctx.Done()
	stop, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	_ = server.Shutdown(stop)
	wg.Wait()
	for _, s := range services {
		s.Repository.Close()
		s.Platform.Store.Close()
	}
}
