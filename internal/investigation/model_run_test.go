package investigation

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	rm "github.com/ishantanu/nines/internal/reviewmodel"
)

type fixtureModel struct {
	route string
	calls atomic.Int32
	turn  func(context.Context, rm.Request, func(rm.Progress) error) (rm.Response, error)
}

func (m *fixtureModel) Descriptor() rm.Descriptor {
	return rm.Descriptor{Provider: "custom", Model: "fixture-model", Protocol: "chat-completions", RouteDigest: m.route, Local: true}
}
func (m *fixtureModel) Turn(ctx context.Context, in rm.Request, progress func(rm.Progress) error) (rm.Response, error) {
	m.calls.Add(1)
	return m.turn(ctx, in, progress)
}
func fixtureResponse(id, name string, args any) rm.Response {
	b, _ := json.Marshal(args)
	return rm.Response{Calls: []rm.Call{{ID: id, ProviderID: id, Name: name, Arguments: b}}, Native: json.RawMessage(`{"role":"assistant","content":"opaque-provider-continuation"}`), UsageKnown: true, InputTokens: 12, OutputTokens: 20}
}
func modelFixture(t *testing.T, mode bool, m *fixtureModel) (*Service, context.Context, string) {
	s, ctx, dsn := fixture(t, mode)
	s.AuthorizeRun = func(context.Context, v1.InvestigationActor) error { return nil }
	s.Model = m
	if _, err := s.Create(ctx, input()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRun(ctx, input().ID, v1.CreateInvestigationRun{ID: "model-run", ExpectedVersion: 1, Mode: "model", Question: "Why is the SLI unknown?"}); err != nil {
		t.Fatal(err)
	}
	return s, ctx, dsn
}
func runSteps(t *testing.T, s *Service, ctx context.Context, n int) {
	t.Helper()
	worker := Worker{Service: s}
	for range n {
		found, err := worker.WorkOne(ctx)
		if err != nil || !found {
			t.Fatal(found, err)
		}
	}
}
func TestModelReviewToolLoopSurvivesPrivateStateReopen(t *testing.T) {
	runModes(t, func(t *testing.T, mode bool) {
		m := &fixtureModel{route: "fixture-route"}
		m.turn = func(ctx context.Context, in rm.Request, emit func(rm.Progress) error) (rm.Response, error) {
			if emit != nil {
				if err := emit(rm.Progress{Bytes: 100}); err != nil {
					return rm.Response{}, err
				}
				if err := emit(rm.Progress{Bytes: 200, Final: true}); err != nil {
					return rm.Response{}, err
				}
			}
			var intro struct {
				EvidenceDigest string `json:"evidenceDigest"`
			}
			if json.Unmarshal([]byte(in.Messages[0].Text), &intro) != nil || intro.EvidenceDigest == "" {
				t.Fatal("missing evidence provenance")
			}
			if len(in.Messages) == 1 {
				return fixtureResponse("first", "read_objective", map[string]string{"resourceId": "checkout"}), nil
			}
			if len(in.Messages) != 3 || !strings.Contains(string(in.Messages[1].Native), "opaque-provider-continuation") || !strings.Contains(string(in.Messages[2].Result), `"tenant":"alpha"`) {
				t.Fatal("lost private model/tool history", in.Messages)
			}
			return fixtureResponse("finish", "finish_review", v1.ModelReview{Summary: "The stored evidence does not establish a successful window.", EvidenceDigests: []string{intro.EvidenceDigest}, Limitations: []string{"No causal diagnosis was verified."}}), nil
		}
		s, ctx, dsn := modelFixture(t, mode, m)
		runSteps(t, s, ctx, 4)
		if mode {
			s.Repository.Close()
			repo, err := NewPostgres(ctx, dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer repo.Close()
			s = New(s.Platform, repo)
			s.Model = m
			s.AuthorizeRun = func(context.Context, v1.InvestigationActor) error { return nil }
		}
		runSteps(t, s, ctx, 2)
		r, err := s.ReadRun(ctx, input().ID, "model-run")
		if err != nil || r.Status != "completed" || r.ModelReview == nil || r.ModelCalls != 2 || r.ToolCalls != 2 || r.OutputTokensReserved != 2048 || r.ReceivedBytes != 400 || r.Steps[0].Attempts != 1 || r.Steps[1].Assessment.State != "insufficient_evidence" {
			t.Fatal(r, err)
		}
		public, _ := json.Marshal(r)
		if strings.Contains(string(public), "opaque-provider-continuation") || strings.Contains(string(public), `"messages"`) {
			t.Fatal("private transcript leaked")
		}
	})
}
func TestModelUnsafeToolsAndCitationsFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name, tool, code string
		args             any
	}{{"write", "apply_yaml", "unsafe_tool_call", map[string]string{"yaml": "untrusted"}}, {"scope", "read_objective", "unsafe_tool_call", map[string]string{"resourceId": "another-tenant-resource"}}, {"citation", "finish_review", "invalid_model_citation", v1.ModelReview{Summary: "Invented conclusion", EvidenceDigests: []string{"not-the-captured-digest"}, Limitations: []string{}}}, {"unknown_parameter", "read_context", "unsafe_tool_call", map[string]string{"tenant": "beta"}}} {
		t.Run(tc.name, func(t *testing.T) {
			m := &fixtureModel{route: "fixture"}
			m.turn = func(context.Context, rm.Request, func(rm.Progress) error) (rm.Response, error) {
				return fixtureResponse("bad", tc.tool, tc.args), nil
			}
			s, ctx, _ := modelFixture(t, false, m)
			runSteps(t, s, ctx, 3)
			r, _ := s.ReadRun(ctx, input().ID, "model-run")
			if r.Status != "failed" || r.ErrorCode != tc.code || r.ModelReview != nil || len(r.Steps) != 3 || r.ModelCalls != 1 {
				t.Fatal(r)
			}
		})
	}
}
func TestModelCancellationInterruptsInflightCall(t *testing.T) {
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	m := &fixtureModel{route: "fixture"}
	m.turn = func(ctx context.Context, _ rm.Request, _ func(rm.Progress) error) (rm.Response, error) {
		close(entered)
		<-ctx.Done()
		close(cancelled)
		return rm.Response{}, rm.Failure("model_unavailable")
	}
	s, ctx, _ := modelFixture(t, false, m)
	runSteps(t, s, ctx, 2)
	done := make(chan error, 1)
	go func() { _, err := (&Worker{Service: s}).WorkOne(ctx); done <- err }()
	<-entered
	r, _ := s.ReadRun(ctx, input().ID, "model-run")
	if _, err := s.CancelRun(ctx, input().ID, "model-run", v1.CancelInvestigationRun{ExpectedVersion: r.Version}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight model call was not cancelled")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	r, _ = s.ReadRun(ctx, input().ID, "model-run")
	if r.Status != "cancelled" || r.ModelReview != nil {
		t.Fatal(r)
	}
}
func TestModelBudgetAndRoutingRemainPinned(t *testing.T) {
	m := &fixtureModel{route: "first"}
	m.turn = func(context.Context, rm.Request, func(rm.Progress) error) (rm.Response, error) {
		t.Fatal("budget allowed another model call")
		return rm.Response{}, nil
	}
	s, ctx, _ := modelFixture(t, false, m)
	runSteps(t, s, ctx, 2)
	_, err := s.Repository.updateRun(ctx, input().ID, "model-run", func(r *runRecord) error { r.Run.ModelCalls = 4; r.Run.OutputTokensReserved = 4096; return nil })
	if err != nil {
		t.Fatal(err)
	}
	runSteps(t, s, ctx, 1)
	r, _ := s.ReadRun(ctx, input().ID, "model-run")
	if r.Status != "failed" || r.ErrorCode != "model_budget_exceeded" || m.calls.Load() != 0 {
		t.Fatal(r)
	}
	s2, ctx2, _ := modelFixture(t, false, m)
	m.route = "changed"
	runSteps(t, s2, ctx2, 1)
	r, _ = s2.ReadRun(ctx2, input().ID, "model-run")
	if r.ErrorCode != "model_configuration_changed" {
		t.Fatal(r)
	}
	s2.Model = nil
	if _, err = s2.CreateRun(ctx2, input().ID, v1.CreateInvestigationRun{ID: "model-run", ExpectedVersion: 1, Mode: "model", Question: "Why is the SLI unknown?"}); err != nil {
		t.Fatal("committed retry depended on new provider configuration", err)
	}
	if _, err = s2.CreateRun(ctx2, input().ID, v1.CreateInvestigationRun{ID: "new-model", ExpectedVersion: 1, Mode: "model", Question: "Explain evidence"}); err == nil {
		t.Fatal("missing provider admitted new model work")
	}
}
